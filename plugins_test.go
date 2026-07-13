package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/infracost/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadConfigString writes content to a temp config file and loads it (no env vars, so no YAML
// round-trip after the fold - blob values stay Go-native).
func loadConfigString(t *testing.T, content string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "infracost.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	cfg, err := config.LoadConfigFile(path, dir, nil)
	require.NoError(t, err)
	return cfg
}

func projectByPath(t *testing.T, cfg *config.Config, path string) *config.Project {
	t.Helper()
	for _, p := range cfg.Projects {
		if p.Path == path {
			return p
		}
	}
	t.Fatalf("no project with path %q", path)
	return nil
}

// Generation persists terraform var files under plugins.<type>.tfVarsFiles and no longer writes the
// deprecated terraform.var_files field.
func TestPlugins_GenerationSideloadsVarFilesIntoBlob(t *testing.T) {
	root := NewFilesystem(t)
	root.AddTerraformFileWithProviderBlock("main.tf")
	root.AddTFVarsFile("terraform.tfvars")

	cfg, err := config.Generate(t.Context(), root.Path())
	require.NoError(t, err)
	require.Len(t, cfg.Projects, 1)

	p := cfg.Projects[0]
	assert.Empty(t, p.Terraform.VarFiles, "generation must not emit the deprecated terraform.var_files field")
	assert.Equal(t, config.ProjectTypeTerraform, p.Type)
	assert.Contains(t, toStringSlice(p.Plugins["terraform"]["tfVarsFiles"]), "terraform.tfvars")
}

// A hand-written cloudformation project's aws.* fields fold into plugins.cloudformation.awsContext,
// using the JSON keys the cloudformation plugin consumes.
func TestPlugins_FoldsAWSContext(t *testing.T) {
	cfg := loadConfigString(t, `version: 0.3
projects:
  - path: stack
    type: cloudformation
    aws:
      region: us-east-1
      account_id: "123456789012"
      stack_name: my-stack
`)

	p := projectByPath(t, cfg, "stack")
	assert.Equal(t, map[string]any{
		"region":    "us-east-1",
		"accountId": "123456789012",
		"stackName": "my-stack",
	}, p.Plugins["cloudformation"]["awsContext"])
}

// CDK projects are parsed by the cloudformation plugin, so their aws.* fold into
// plugins.cloudformation (not plugins.cdk_*).
func TestPlugins_FoldsCDKAWSContextUnderCloudformation(t *testing.T) {
	cfg := loadConfigString(t, `version: 0.3
projects:
  - path: app
    type: cdk_typescript
    aws:
      region: eu-west-1
`)

	p := projectByPath(t, cfg, "app")
	_, hasCDKKey := p.Plugins["cdk_typescript"]
	assert.False(t, hasCDKKey, "CDK options must not live under a cdk_* key")
	assert.Equal(t, map[string]any{"region": "eu-west-1"}, p.Plugins["cloudformation"]["awsContext"])
}

// Repo-level plugin defaults deep-merge into each project's blob: nested maps merge, but lists are
// atomic with the per-project value winning; a project with no blob inherits the defaults wholesale.
func TestPlugins_TopLevelDefaultsMergeIntoProjects(t *testing.T) {
	cfg := loadConfigString(t, `version: 0.3
plugins:
  terraform:
    env:
      FOO: bar
    tfVarsFiles:
      - global.tfvars
projects:
  - path: a
    type: terraform
    plugins:
      terraform:
        tfVarsFiles:
          - a.tfvars
  - path: b
    type: terraform
`)

	a := projectByPath(t, cfg, "a")
	assert.Equal(t, []any{"a.tfvars"}, a.Plugins["terraform"]["tfVarsFiles"], "per-project list wins atomically over the default")
	assert.Equal(t, map[string]any{"FOO": "bar"}, a.Plugins["terraform"]["env"], "unset keys are inherited from the repo-level default")

	b := projectByPath(t, cfg, "b")
	assert.Equal(t, []any{"global.tfvars"}, b.Plugins["terraform"]["tfVarsFiles"], "a project with no blob inherits the defaults")
	assert.Equal(t, map[string]any{"FOO": "bar"}, b.Plugins["terraform"]["env"])
}

// Precedence is deprecated field > per-project blob > repo-level default. (workspace is no longer a
// folded field, so this uses var files, which still fold from the deprecated terraform.var_files.)
func TestPlugins_PrecedenceDeprecatedOverBlobOverDefault(t *testing.T) {
	cfg := loadConfigString(t, `version: 0.3
plugins:
  terraform:
    tfVarsFiles:
      - from-default.tfvars
projects:
  - path: a
    type: terraform
    terraform:
      var_files:
        - from-deprecated.tfvars
    plugins:
      terraform:
        tfVarsFiles:
          - from-blob.tfvars
`)

	a := projectByPath(t, cfg, "a")
	assert.Equal(t, []string{"from-deprecated.tfvars"}, toStringSlice(a.Plugins["terraform"]["tfVarsFiles"]))
}

// With no deprecated field set, the per-project blob wins over the repo-level default.
func TestPlugins_PrecedenceBlobOverDefault(t *testing.T) {
	cfg := loadConfigString(t, `version: 0.3
plugins:
  terraform:
    tfVarsFiles:
      - from-default.tfvars
projects:
  - path: a
    type: terraform
    plugins:
      terraform:
        tfVarsFiles:
          - from-blob.tfvars
`)

	a := projectByPath(t, cfg, "a")
	assert.Equal(t, []string{"from-blob.tfvars"}, toStringSlice(a.Plugins["terraform"]["tfVarsFiles"]))
}
