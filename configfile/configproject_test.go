package configfile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func decodeProject(t *testing.T, src string) Project {
	t.Helper()
	var p Project
	require.NoError(t, yaml.Unmarshal([]byte(src), &p))
	return p
}

// Structural fields decode into their fields; every other top-level key is captured as an opaque
// heading, and the structural keys never leak into Headings.
func TestProject_UnmarshalSplitsStructuralFromHeadings(t *testing.T) {
	p := decodeProject(t, `
name: main
type: terragrunt
path: app
env_name: prod
usage_file: usage.yml
exclude_paths: [vendor]
terraform:
  workspace: prod
  cloud:
    host: app.terraform.io
    org: acme
  var_files: [prod.tfvars]
  vars:
    region: us-east-1
aws:
  region: eu-west-1
  stack_id: s-123
kubernetes:
  some_option: true
`)

	assert.Equal(t, "main", p.Name)
	assert.Equal(t, "terragrunt", p.Type)
	assert.Equal(t, "app", p.Path)
	assert.Equal(t, "prod", p.EnvName)
	assert.Equal(t, "usage.yml", p.UsageFile)
	assert.Equal(t, []string{"vendor"}, p.ExcludePaths)

	// headings captured
	assert.Contains(t, p.Headings, "terraform")
	assert.Contains(t, p.Headings, "aws")
	assert.Contains(t, p.Headings, "kubernetes")

	// structural keys must NOT be captured as headings
	for _, k := range []string{"name", "type", "path", "env_name", "usage_file", "exclude_paths"} {
		_, leaked := p.Headings[k]
		assert.Falsef(t, leaked, "structural key %q leaked into Headings", k)
	}

	// heading contents are preserved as raw nodes we can decode
	tf := decodeHeading(t, p, "terraform")
	assert.Equal(t, "prod", tf["workspace"])
	assert.Equal(t, []any{"prod.tfvars"}, tf["var_files"])
	assert.Equal(t, map[string]any{"host": "app.terraform.io", "org": "acme"}, tf["cloud"])
}

// An unknown key with a non-mapping value is a typo of a structural field (plugin blobs are always
// mappings), so it errors rather than being silently captured as a heading. Mapping and empty/null
// headings are accepted.
func TestProject_RejectsNonMappingUnknownKeys(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		wantErr string
	}{
		{"scalar typo", "path: app\npathh: x", `unknown field "pathh"`},
		{"sequence heading", "path: app\nterraform: [foo]", `unknown field "terraform"`},
		{"mapping heading", "path: app\nkubernetes: {ns: x}", ""},
		{"empty heading", "path: app\nkubernetes:", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p Project
			err := yaml.Unmarshal([]byte(tc.src), &p)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

// A project with no headings has a nil/empty Headings map (not spurious captures).
func TestProject_NoHeadings(t *testing.T) {
	p := decodeProject(t, `
name: main
type: terraform
path: .
`)
	assert.Empty(t, p.Headings)
}

// Marshalling emits one heading per plugin (no `plugins:` block) and round-trips: decode -> encode ->
// decode preserves structural fields and heading contents.
func TestProject_RoundTrip(t *testing.T) {
	src := `
name: main
type: terragrunt
path: app
terraform:
    cloud:
        host: app.terraform.io
        org: acme
    workspace: prod
    var_files:
        - prod.tfvars
aws:
    region: eu-west-1
kubernetes:
    some_option: true
`
	p := decodeProject(t, src)

	out, err := yaml.Marshal(p)
	require.NoError(t, err)

	rendered := string(out)
	t.Logf("\n--- MARSHALLED ---\n%s", rendered)
	assert.NotContains(t, rendered, "plugins:", "must not emit a plugins: block")
	// one heading per plugin, at project level
	assert.Contains(t, rendered, "terraform:")
	assert.Contains(t, rendered, "aws:")
	assert.Contains(t, rendered, "kubernetes:")

	p2 := decodeProject(t, rendered)
	assert.Equal(t, p.Name, p2.Name)
	assert.Equal(t, p.Type, p2.Type)
	assert.Equal(t, p.Path, p2.Path)

	assert.Equal(t, decodeHeading(t, p, "terraform"), decodeHeading(t, p2, "terraform"), "terraform heading must survive round-trip")
	assert.Equal(t, decodeHeading(t, p, "aws"), decodeHeading(t, p2, "aws"))
}

// decodeHeading decodes a captured heading node into a generic map (map values aren't addressable, so
// the node must be copied to a local before calling the pointer-receiver Decode).
func decodeHeading(t *testing.T, p Project, name string) map[string]any {
	t.Helper()
	node := p.Headings[name]
	var out map[string]any
	require.NoError(t, node.Decode(&out))
	return out
}

// A project decoded as an element of a list (the real config shape) still splits correctly - i.e. the
// custom (un)marshal works in the position it's actually used.
func TestProject_InProjectsList(t *testing.T) {
	var doc struct {
		Projects []Project `yaml:"projects"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(`
projects:
  - name: a
    type: terraform
    path: a
    terraform:
      var_files: [a.tfvars]
  - name: b
    type: cloudformation
    path: b
    aws:
      region: us-east-1
`), &doc))

	require.Len(t, doc.Projects, 2)
	assert.Contains(t, doc.Projects[0].Headings, "terraform")
	assert.NotContains(t, doc.Projects[0].Headings, "aws")
	assert.Contains(t, doc.Projects[1].Headings, "aws")
}

// structuralProjectKeys must exactly match the yaml tags of the non-Headings fields, so nothing config
// treats as structural is accidentally captured as a heading (or vice versa).
func TestProject_StructuralKeysMatchTags(t *testing.T) {
	tagged := map[string]bool{}
	rt := reflect.TypeFor[Project]()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			continue
		}
		tagged[name] = true
	}
	assert.Equal(t, tagged, structuralProjectKeys, "structuralProjectKeys is out of sync with Project's yaml tags")
}
