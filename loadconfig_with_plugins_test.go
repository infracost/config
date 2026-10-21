package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/infracost/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_LoadConfigFile_BackfillsMissingType_WithPlugins checks that LoadConfigFile backfills a
// typeless project using a plugin identifier when one is configured, so plugin-driven types are
// detected rather than everything defaulting to terraform. See FIX-495.
func Test_LoadConfigFile_BackfillsMissingType_WithPlugins(t *testing.T) {
	root := NewFilesystem(t)
	root.AddDirectory("cfn").AddCFNYAML()

	configPath := filepath.Join(root.Path(), "infracost.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(`version: 0.1
projects:
  - path: cfn
    name: my-cfn
`), 0o600))

	cfg, err := config.LoadConfigFile(t.Context(), configPath, root.Path(), config.WithLoadPluginDir(pluginDir))
	require.NoError(t, err)
	require.Len(t, cfg.Projects, 1)
	assert.Equal(t, config.ProjectTypeCloudFormation, cfg.Projects[0].Type)
}

// Test_LoadConfigFile_BackfilledTypeKeepsPluginSettings checks that a typeless project's plugin
// settings end up under the key for the type the backfill gives it.
func Test_LoadConfigFile_BackfilledTypeKeepsPluginSettings(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		config     string
		wantType   config.ProjectType
		wantKey    string
		wantPlugin map[string]any
	}{
		{
			name: "0.3 terragrunt",
			path: "tg/dev",
			config: `version: "0.3"
projects:
  - path: tg/dev
    terraform:
      vars:
        x: 1
      var_files:
        - dev.tfvars
`,
			wantType:   config.ProjectTypeTerragrunt,
			wantKey:    "terragrunt",
			wantPlugin: map[string]any{"vars": map[string]any{"x": 1}, "var_files": []any{"dev.tfvars"}},
		},
		{
			name: "0.1 terragrunt",
			path: "tg/dev",
			config: `version: 0.1
projects:
  - path: tg/dev
    terraform_vars:
      x: 1
    terraform_var_files:
      - dev.tfvars
`,
			wantType:   config.ProjectTypeTerragrunt,
			wantKey:    "terragrunt",
			wantPlugin: map[string]any{"vars": map[string]any{"x": 1}, "var_files": []string{"dev.tfvars"}},
		},
		{
			name: "0.2 terragrunt with repo source map",
			path: "tg/dev",
			config: `version: 0.2
terraform_source_map:
  - match: "^git::foo"
    replace: "../../mod"
projects:
  - path: tg/dev
    terraform_vars:
      x: 1
`,
			wantType: config.ProjectTypeTerragrunt,
			wantKey:  "terragrunt",
			wantPlugin: map[string]any{
				"vars":             map[string]any{"x": 1},
				"regex_source_map": map[string]any{"^git::foo": "../../mod"},
				"regex_source_rules": []any{
					map[string]any{"match": "^git::foo", "replace": "../../mod"},
				},
			},
		},
		{
			name: "0.3 terraform",
			path: "mod",
			config: `version: "0.3"
projects:
  - path: mod
    terraform:
      vars:
        x: 1
`,
			wantType:   config.ProjectTypeTerraform,
			wantKey:    "terraform",
			wantPlugin: map[string]any{"vars": map[string]any{"x": 1}},
		},
		{
			name: "0.3 cloudformation with repo aws defaults",
			path: "cfn",
			config: `version: "0.3"
aws:
  defaults:
    region: eu-west-1
projects:
  - path: cfn
`,
			wantType:   config.ProjectTypeCloudFormation,
			wantKey:    "cloudformation",
			wantPlugin: map[string]any{"region": "eu-west-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := NewFilesystem(t)
			root.AddDirectory("tg").AddDirectory("dev").AddFile("terragrunt.hcl").Content(`terraform {
  source = "../../mod"
}`)
			root.AddDirectory("mod").AddTerraformFileWithProviderBlock("main.tf")
			root.AddDirectory("cfn").AddCFNYAML()

			configPath := filepath.Join(root.Path(), "infracost.yml")
			require.NoError(t, os.WriteFile(configPath, []byte(tt.config), 0o600))

			cfg, err := config.LoadConfigFile(t.Context(), configPath, root.Path(), config.WithLoadPluginDir(pluginDir))
			require.NoError(t, err)
			require.Len(t, cfg.Projects, 1)

			p := cfg.Projects[0]
			assert.Equal(t, tt.wantType, p.Type)
			assert.EqualValues(t, tt.wantPlugin, p.Plugins[tt.wantKey])
			if tt.wantKey != "terraform" {
				assert.Nil(t, p.Plugins["terraform"])
			}
		})
	}
}

// Test_LoadConfigFile_BackfilledTerragruntMirrorsDeprecatedFields checks that a moved terragrunt blob
// is mirrored to the deprecated typed fields.
func Test_LoadConfigFile_BackfilledTerragruntMirrorsDeprecatedFields(t *testing.T) {
	root := NewFilesystem(t)
	root.AddDirectory("tg").AddTerragruntFile()

	configPath := filepath.Join(root.Path(), "infracost.yml")
	require.NoError(t, os.WriteFile(configPath, []byte(`version: "0.3"
projects:
  - path: tg
    terraform:
      vars:
        x: 1
      var_files:
        - dev.tfvars
`), 0o600))

	cfg, err := config.LoadConfigFile(t.Context(), configPath, root.Path(), config.WithLoadPluginDir(pluginDir))
	require.NoError(t, err)
	require.Len(t, cfg.Projects, 1)

	p := cfg.Projects[0]
	assert.Equal(t, config.ProjectTypeTerragrunt, p.Type)
	assert.Equal(t, []string{"dev.tfvars"}, p.Terraform.VarFiles)
	assert.EqualValues(t, map[string]any{"x": 1}, p.Terraform.Vars)
}
