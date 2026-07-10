package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/infracost/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toStringSlice converts a decoded YAML/JSON list (a []any of strings) to []string for comparison.
func toStringSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, fmt.Sprint(it))
	}
	return out
}

func findProject(projects []*config.Project, name string) *config.Project {
	for _, p := range projects {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// When a plugin does not emit a raw_options blob (as here, with no plugins configured), autodetect
// backfills it from the data config derived itself, and it is emitted under plugins.<type>.
func Test_Generate_EmitsBackfilledPluginBlob(t *testing.T) {
	root := NewFilesystem(t)

	infraDir := root.AddDirectory("infra")
	componentsDir := infraDir.AddDirectory("components")
	fooDir := componentsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")

	variablesDir := infraDir.AddDirectory("variables")
	devDir := variablesDir.AddDirectory("dev")
	devDir.AddTFVarsFile("bla.tfvars")
	variablesDir.AddTFVarsFile("defaults.tfvars")

	generated, err := config.Generate(t.Context(), root.Path())
	require.NoError(t, err)

	p := findProject(generated.Projects, "infra-components-foo-dev")
	require.NotNil(t, p, "expected project infra-components-foo-dev to be generated")

	// the deprecated terraform.* fields are no longer emitted...
	assert.Empty(t, p.Terraform.VarFiles)
	assert.Empty(t, p.Terraform.Workspace)

	// ...the data lives only in the plugins.terraform blob now.
	require.Contains(t, p.Plugins, "terraform")
	blob := p.Plugins["terraform"]
	assert.Equal(t, "dev", blob["workspace"])
	assert.Equal(t, []string{
		"../../variables/defaults.tfvars",
		"../../variables/dev/bla.tfvars",
	}, toStringSlice(blob["tfVarsFiles"]))
}

// Repo-level plugin defaults deep-merge into each project's plugin blob during normalization:
// per-project values win (scalars and lists alike), and projects without a blob inherit the defaults.
func Test_LoadConfigFile_MergesGlobalPluginDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "infracost.yml")
	require.NoError(t, os.WriteFile(path, []byte(`version: "0.3"
plugins:
  terraform:
    workspace: default
    tfVarsFiles:
      - global.tfvars
projects:
  - path: prod
    type: terraform
    plugins:
      terraform:
        workspace: prod
        tfVarsFiles:
          - prod.tfvars
  - path: staging
    type: terraform
`), 0600))

	cfg, err := config.LoadConfigFile(path, dir, nil)
	require.NoError(t, err)

	prod := findProject(cfg.Projects, "prod")
	require.NotNil(t, prod)
	// per-project scalar wins over the global default
	assert.Equal(t, "prod", prod.Plugins["terraform"]["workspace"])
	// lists are atomic too: the per-project list replaces the global one (no concatenation)
	assert.Equal(t, []string{"prod.tfvars"}, toStringSlice(prod.Plugins["terraform"]["tfVarsFiles"]))

	staging := findProject(cfg.Projects, "staging")
	require.NotNil(t, staging)
	// a project with no blob inherits the repo-level defaults wholesale
	assert.Equal(t, "default", staging.Plugins["terraform"]["workspace"])
	assert.Equal(t, []string{"global.tfvars"}, toStringSlice(staging.Plugins["terraform"]["tfVarsFiles"]))
}

// An old-style config that only uses the deprecated terraform.* / aws.* fields is folded into the
// plugins blob on read, so consumers can read only the new plugins.<name> style.
func Test_LoadConfigFile_FoldsDeprecatedFieldsIntoPlugins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "infracost.yml")
	require.NoError(t, os.WriteFile(path, []byte(`version: "0.3"
terraform:
  source_map:
    - match: foo
      replace: bar
projects:
  - path: app
    type: terraform
    terraform:
      workspace: prod
      var_files:
        - prod.tfvars
      vars:
        region: us-east-1
      cloud:
        org: my-org
        workspace: app-prod
        host: app.terraform.io
  - path: stack
    type: cloudformation
    aws:
      region: us-east-1
      stack_name: my-stack
`), 0600))

	cfg, err := config.LoadConfigFile(path, dir, nil)
	require.NoError(t, err)

	app := findProject(cfg.Projects, "app")
	require.NotNil(t, app)
	tf := app.Plugins["terraform"]
	require.NotNil(t, tf, "expected terraform.* to be folded into plugins.terraform")
	assert.Equal(t, "prod", tf["workspace"])
	assert.Equal(t, []string{"prod.tfvars"}, toStringSlice(tf["tfVarsFiles"]))
	assert.Equal(t, map[string]any{"region": "us-east-1"}, tf["vars"])
	assert.Equal(t, map[string]any{
		"organization": "my-org",
		"workspace":    "app-prod",
		"hostname":     "app.terraform.io",
	}, tf["terraformCloudConfiguration"])
	// the repo-level source_map is folded into each terraform project's blob
	assert.Equal(t, map[string]any{"foo": "bar"}, tf["regexSourceMap"])

	stack := findProject(cfg.Projects, "stack")
	require.NotNil(t, stack)
	cfn := stack.Plugins["cloudformation"]
	require.NotNil(t, cfn, "expected aws.* to be folded into plugins.cloudformation")
	// unset aws fields are omitted, not written as empty strings.
	assert.Equal(t, map[string]any{
		"region":    "us-east-1",
		"stackName": "my-stack",
	}, cfn["awsContext"])
}
