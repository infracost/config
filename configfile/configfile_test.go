package configfile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func decodeConfig(t *testing.T, src string) Config {
	t.Helper()
	var c Config
	require.NoError(t, yaml.Unmarshal([]byte(src), &c))
	return c
}

// Structural top-level fields decode into their fields; repo-level headings (terraform defaults, cdk,
// unknown plugin defaults) are captured opaquely and don't leak the structural keys.
func TestConfig_UnmarshalSplitsStructuralFromHeadings(t *testing.T) {
	c := decodeConfig(t, `
version: "0.3"
currency: USD
usage_file: usage.yml
terraform:
  source_map:
    - match: a
      replace: b
cdk:
  defaults:
    context:
      key: val
kubernetes:
  some_default: true
projects:
  - name: main
    type: terraform
    path: .
    terraform:
      var_files: [prod.tfvars]
`)

	assert.Equal(t, "0.3", c.Version)
	assert.Equal(t, "USD", c.Currency)
	assert.Equal(t, "usage.yml", c.UsageFile)
	require.Len(t, c.Projects, 1)
	assert.Equal(t, "main", c.Projects[0].Name)
	assert.Contains(t, c.Projects[0].Headings, "terraform")

	// repo-level headings captured
	assert.Contains(t, c.Headings, "terraform")
	assert.Contains(t, c.Headings, "cdk")
	assert.Contains(t, c.Headings, "kubernetes")

	// structural keys must NOT be captured as repo-level headings
	for _, k := range []string{"version", "currency", "usage_file", "projects"} {
		_, leaked := c.Headings[k]
		assert.Falsef(t, leaked, "structural key %q leaked into repo Headings", k)
	}
}

// Round-trip: decode -> encode -> decode preserves structural fields, projects, and repo headings,
// and never emits a `plugins:` block.
func TestConfig_RoundTrip(t *testing.T) {
	src := `
version: "0.3"
currency: USD
terraform:
    source_map:
        - match: a
          replace: b
cdk:
    defaults:
        context:
            key: val
projects:
    - name: main
      type: terraform
      path: .
      terraform:
        var_files:
            - prod.tfvars
      kubernetes:
        opt: true
`
	c := decodeConfig(t, src)

	out, err := yaml.Marshal(c)
	require.NoError(t, err)
	rendered := string(out)
	t.Logf("\n--- MARSHALLED ---\n%s", rendered)
	assert.NotContains(t, rendered, "plugins:", "must not emit a plugins: block")
	assert.Contains(t, rendered, "terraform:")
	assert.Contains(t, rendered, "cdk:")

	c2 := decodeConfig(t, rendered)
	assert.Equal(t, c.Version, c2.Version)
	assert.Equal(t, c.Currency, c2.Currency)
	require.Len(t, c2.Projects, 1)
	assert.Equal(t, "main", c2.Projects[0].Name)
	assert.Contains(t, c2.Projects[0].Headings, "terraform")
	assert.Contains(t, c2.Projects[0].Headings, "kubernetes")

	assert.Equal(t, decodeRepoHeading(t, c, "terraform"), decodeRepoHeading(t, c2, "terraform"))
	assert.Equal(t, decodeRepoHeading(t, c, "cdk"), decodeRepoHeading(t, c2, "cdk"))
}

func decodeRepoHeading(t *testing.T, c Config, name string) map[string]any {
	t.Helper()
	node := c.Headings[name]
	var out map[string]any
	require.NoError(t, node.Decode(&out))
	return out
}

func TestConfig_StructuralKeysMatchTags(t *testing.T) {
	tagged := map[string]bool{}
	rt := reflect.TypeFor[Config]()
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			continue
		}
		tagged[name] = true
	}
	assert.Equal(t, tagged, structuralConfigKeys, "structuralConfigKeys is out of sync with Config's yaml tags")
}
