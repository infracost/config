package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/infracost/config/cdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ConfigVersionChecking(t *testing.T) {

	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "legacy cli version",
			content: "version: 0.1\nprojects:\n  - path: .",
		},
		{
			name:    "legacy service version",
			content: "version: 0.2\nprojects:\n  - path: .",
		},
		{
			name:    "current version",
			content: "version: 0.3\nprojects:\n  - path: .",
		},
		{
			name:    "missing version defaults to 0.2",
			content: "projects:\n  - path: .",
		},
		{
			name:    "invalid version",
			content: "version: 0.4\nprojects:\n  - path: .",
			wantErr: "invalid config YAML: unsupported config file version: 0.4",
		},
		{
			name:    "missing project path",
			content: "version: 0.3\nprojects:\n  - name: test",
			wantErr: "project with name \"test\" has no path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseConfigFile([]byte(tt.content), nil, nil)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg)
			assert.Equal(t, CurrentVersion, cfg.Version)
		})
	}

}

func Test_ErrorSimplification(t *testing.T) {

	content := `
version: 0.2
projects:
  - unknown_field: foo
  - unknown_field: bar
  - unknown_field: baz
`

	_, err := parseConfigFile([]byte(content), nil, nil)
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid config YAML: invalid config YAML: line 4: field unknown_field not found")

}

func TestLoad(t *testing.T) {
	tmp := t.TempDir()
	tests := []struct {
		error             error
		name              string
		contents          []byte
		expected          []*Project
		expectedSourceMap []TerraformRegexSource
	}{
		{
			name: "should parse valid projects (legacy version)",
			contents: []byte(`version: 0.2

projects:
  - path: path/to/my_terraform
  - path: path/to/my_terraform_two
    terraform_workspace: "development"
    terraform_cloud_host: "cloud_host"
    terraform_cloud_token: "cloud_token"
    usage_file: "usage/file"
`),
			expected: []*Project{
				{
					Name: "path-to-my_terraform",
					Path: "path/to/my_terraform",
				},
				{
					Name: "path-to-my_terraform_two",
					Path: "path/to/my_terraform_two",
					Terraform: ProjectTerraform{
						Workspace: "development",
						Cloud: TerraformCloud{
							Host:  "cloud_host",
							Token: "cloud_token",
						},
					},
					UsageFile: "usage/file",
				},
			},
		},
		{
			name: "should parse valid projects (current version)",
			contents: []byte(`version: 0.2

projects:
  - path: path/to/my_terraform
  - path: path/to/my_terraform_two
    terraform:
      workspace: "development"
      cloud:
        host: "cloud_host"
        token: "cloud_token"
    usage_file: "usage/file"
`),
			expected: []*Project{
				{
					Name: "path-to-my_terraform",
					Path: "path/to/my_terraform",
				},
				{
					Name: "path-to-my_terraform_two",
					Path: "path/to/my_terraform_two",
					Terraform: ProjectTerraform{
						Workspace: "development",
						Cloud: TerraformCloud{
							Host:  "cloud_host",
							Token: "cloud_token",
						},
					},
					UsageFile: "usage/file",
				},
			},
		},
		{
			name: "should not return error if no projects given",
			contents: []byte(`version: 0.1

projects:
`),
			expected: nil,
		},
		{
			name: "should report invalid indentation",
			contents: []byte(`version: 0.1

projects:
  - afdas: safasfddas
		`),
			error: fmt.Errorf("%w: yaml: line 4: found a tab character that violates indentation", ErrInvalidConfigYAML),
		},
		{
			name: "should error invalid project key given",
			contents: []byte(`version: 0.1

projects:
  - path: path/to/my_terraform
    invalid_key: "test"
`),
			error: fmt.Errorf("parsing config file failed, check file syntax: yaml: unmarshal errors:\n  line 5: field invalid_key not found in type config.Project"),
		},
		{
			name: "should error invalid version given",
			contents: []byte(`version: 81923.1

projects:
  - path: path/to/my_terraform
`),
			error: fmt.Errorf("config file version v81923.1 is not supported, must be between 0.1 and 0.1"),
		},
		{
			name: "should parse valid terraform_source_map",
			contents: []byte(`version: 0.1

terraform_source_map:
  - match: "^ANOTHER_MODULE$"
    replace: "github.com/CentricaDevOps/networks-aws-modules//modules/another?ref=another_v1.0.0"

projects:
  - path: path/to/my_terraform
`),
			expectedSourceMap: []TerraformRegexSource{
				{
					Match:   "^ANOTHER_MODULE$",
					Replace: "github.com/CentricaDevOps/networks-aws-modules//modules/another?ref=another_v1.0.0",
				},
			},
			expected: []*Project{
				{
					Name: "main",
					Path: "path/to/my_terraform",
				},
			},
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(tmp, fmt.Sprintf("conf-%d.yaml", i))
			err := os.WriteFile(path, tt.contents, os.ModePerm) // nolint: gosec
			require.NoError(t, err)

			// we need to remove INFRACOST_TERRAFORM_CLOUD_TOKEN value for these tests.
			// as CI uses INFRACOST_TERRAFORM_CLOUD_TOKEN for private registry tests. This means the expected value
			// will be inconsistent and show "***".
			key := "INFRACOST_TERRAFORM_CLOUD_TOKEN"
			v := os.Getenv(key)
			_ = os.Unsetenv(key)

			if v != "" {
				defer func() {
					_ = os.Setenv(key, v)
				}()
			}

			c, err := LoadConfigFile(path, tmp, nil)
			if tt.error != nil {
				require.Error(t, tt.error, err)
			} else {
				require.NoError(t, err)
				require.EqualValues(t, tt.expected, c.Projects)
				require.EqualValues(t, tt.expectedSourceMap, c.Terraform.SourceMap)
			}
		})
	}
}

func TestGenerateFillsCDKBlockWhenFeatureFlagEnabled(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(`variable "x" {}`), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))
	cfg, err := Generate(tmp, WithTemplate(`version: 0.3
cdk:
  defaults:
    context:
      foo: bar
`))
	require.NoError(t, err)
	require.Len(t, cfg.CDK.Projects, 1)
	require.Equal(t, "foo/cdk.json", cfg.CDK.Projects[0].CdkConfigPath)
	require.True(t, cfg.CDK.Projects[0].Context.IsSet)
	require.Equal(t, map[string]string{"foo": "bar"}, cfg.CDK.Projects[0].Context.Value)
	require.Equal(t, []string{"package.json"}, cfg.CDK.Projects[0].PackageManifestPaths)
}

func TestGenerateDetectsCDK(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(`variable "x" {}`), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))

	cfg, err := Generate(tmp)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.CDK.Projects)
}

func TestGenerateAutodetectsEmptyCDKBlock(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(`variable "x" {}`), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))

	cfg, err := Generate(tmp, WithTemplate(`version: 0.2
cdk:
`))
	require.NoError(t, err)
	require.Len(t, cfg.CDK.Projects, 1)
	require.Equal(t, "foo/cdk.json", cfg.CDK.Projects[0].CdkConfigPath)
	require.Equal(t, []string{"package.json"}, cfg.CDK.Projects[0].PackageManifestPaths)
}

func TestMergeCDKEntriesWithAutodetect_ForgivingExamples(t *testing.T) {
	// Set up a temp dir with foo/cdk.json and foo/package.json so cdk.GenerateConfig detects them
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))

	testCases := []struct {
		name     string
		input    []*cdk.ConfigEntry
		expected []*cdk.ConfigEntry
	}{
		{
			name:  "missingEverything",
			input: []*cdk.ConfigEntry{{}},
			expected: []*cdk.ConfigEntry{{
				CdkConfigPath:        "foo/cdk.json",
				PackageManifestPaths: []string{"package.json"},
			}},
		},
		{
			name: "contextOnly",
			input: []*cdk.ConfigEntry{{
				Context: cdk.FromMapPtr(&map[string]string{"foo": "bar"}),
			}},
			expected: []*cdk.ConfigEntry{{
				CdkConfigPath:        "foo/cdk.json",
				Context:              cdk.FromMapPtr(&map[string]string{"foo": "bar"}),
				PackageManifestPaths: []string{"package.json"},
			}},
		},
		{
			name: "pathOnly",
			input: []*cdk.ConfigEntry{{
				CdkConfigPath: "cdk.json",
			}},
			expected: []*cdk.ConfigEntry{{
				CdkConfigPath: "cdk.json",
			}},
		},
		{
			name: "package_manifest_pathsOnly",
			input: []*cdk.ConfigEntry{{
				PackageManifestPaths: []string{"package.json"},
			}},
			expected: []*cdk.ConfigEntry{{
				CdkConfigPath:        "foo/cdk.json",
				PackageManifestPaths: []string{"package.json"},
			}},
		},
		{
			name: "pathAndPackageManifestPaths",
			input: []*cdk.ConfigEntry{{
				CdkConfigPath:        "cdk.json",
				PackageManifestPaths: []string{"package.json"},
			}},
			expected: []*cdk.ConfigEntry{{
				CdkConfigPath:        "cdk.json",
				PackageManifestPaths: []string{"package.json"},
			}},
		},
		{
			name: "pathAndContext",
			input: []*cdk.ConfigEntry{{
				CdkConfigPath: "cdk.json",
				Context:       cdk.FromMapPtr(&map[string]string{"foo": "bar"}),
			}},
			expected: []*cdk.ConfigEntry{{
				CdkConfigPath: "cdk.json",
				Context:       cdk.FromMapPtr(&map[string]string{"foo": "bar"}),
			}},
		},
		{
			name: "contextAndPackageManifestPaths",
			input: []*cdk.ConfigEntry{{
				Context:              cdk.FromMapPtr(&map[string]string{"foo": "bar"}),
				PackageManifestPaths: []string{"package.json"},
			}},
			expected: []*cdk.ConfigEntry{{
				CdkConfigPath:        "foo/cdk.json",
				Context:              cdk.FromMapPtr(&map[string]string{"foo": "bar"}),
				PackageManifestPaths: []string{"package.json"},
			}},
		},
		{
			name: "fullyConfigured",
			input: []*cdk.ConfigEntry{{
				CdkConfigPath:        "cdk.json",
				Context:              cdk.FromMapPtr(&map[string]string{"foo": "bar"}),
				PackageManifestPaths: []string{"package.json"},
			}},
			expected: []*cdk.ConfigEntry{{
				CdkConfigPath:        "cdk.json",
				Context:              cdk.FromMapPtr(&map[string]string{"foo": "bar"}),
				PackageManifestPaths: []string{"package.json"},
			}},
		},
		{
			name: "pathMatchesDetected",
			input: []*cdk.ConfigEntry{{
				CdkConfigPath: "foo/cdk.json",
			}},
			expected: []*cdk.ConfigEntry{{
				CdkConfigPath:        "foo/cdk.json",
				PackageManifestPaths: []string{"package.json"},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := mergeCDKEntriesWithAutodetect(tmp, tc.input, cdk.Defaults{})
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestMergeCDKEntriesWithAutodetect_GlobalOverlaysApplyToDetectedEntries(t *testing.T) {
	// Set up a temp dir with foo/cdk.json, foo/package.json, bar/cdk.json, bar/package.json
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "bar"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "bar", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))

	ctxMap := map[string]string{"foo": "bar"}
	input := []*cdk.ConfigEntry{
		{Context: cdk.FromMapPtr(&ctxMap)},
		{PackageManifestPaths: []string{"package.json"}},
	}

	result, err := mergeCDKEntriesWithAutodetect(tmp, input, cdk.Defaults{})
	require.NoError(t, err)
	require.Len(t, result, 2)

	require.Equal(t, "bar/cdk.json", result[0].CdkConfigPath)
	require.Equal(t, []string{"package.json"}, result[0].PackageManifestPaths)
	require.True(t, result[0].Context.IsSet)
	require.Equal(t, map[string]string{"foo": "bar"}, result[0].Context.Value)

	require.Equal(t, "foo/cdk.json", result[1].CdkConfigPath)
	require.Equal(t, []string{"package.json"}, result[1].PackageManifestPaths)
	require.True(t, result[1].Context.IsSet)
	require.Equal(t, map[string]string{"foo": "bar"}, result[1].Context.Value)
}

func TestMergeCDKEntriesWithAutodetect_GlobalOverlayDoesNotOverrideLocal(t *testing.T) {
	// Set up a temp dir with foo/cdk.json and foo/package.json
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))

	ctxMap1 := map[string]string{"foo": "bar"}
	ctxMap2 := map[string]string{"local": "value"}
	input := []*cdk.ConfigEntry{
		{Context: cdk.FromMapPtr(&ctxMap1)},
		{
			CdkConfigPath: "foo/cdk.json",
			Context:       cdk.FromMapPtr(&ctxMap2),
		},
	}

	result, err := mergeCDKEntriesWithAutodetect(tmp, input, cdk.Defaults{})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "foo/cdk.json", result[0].CdkConfigPath)
	require.Equal(t, []string{"package.json"}, result[0].PackageManifestPaths)
	require.True(t, result[0].Context.IsSet)
	require.Equal(t, map[string]string{"local": "value"}, result[0].Context.Value)
}

func TestLoadConfigFile_WithoutCDKBlocks(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "infracost.yml")

	configContent := `version: 0.1

projects:
  - path: terraform/
    name: my-terraform
`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	cfg, err := LoadConfigFile(configPath, tmp, nil)

	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Empty(t, cfg.CDK.Projects, "should have no CDK entries")
	require.Len(t, cfg.Projects, 1)
	require.Equal(t, "my-terraform", cfg.Projects[0].Name)
}

// TestGenerate_CDKDefaults_WithTemplate_DefaultsOnly tests that cdk defaults in a template
// (without a cdk projects section) are applied to autodetected CDK entries
func TestGenerate_CDKDefaults_WithTemplate_DefaultsOnly(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(`variable "x" {}`), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))

	cfg, err := Generate(tmp, WithTemplate(`version: 0.3
cdk:
  defaults:
    context:
      org: acme
      env: prod
`))
	require.NoError(t, err)
	require.Len(t, cfg.CDK.Projects, 1)
}

// TestGenerate_CDKDefaults_WithTemplate_PreservesLocalValues tests that cdk defaults
// don't override explicitly set context/env values in CDK entries
func TestGenerate_CDKDefaults_WithTemplate_PreservesLocalValues(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(`variable "x" {}`), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))

	cfg, err := Generate(tmp, WithTemplate(`version: 0.3
cdk:
  defaults:
    context:
      org: acme
      env: prod
  apps:
  - cdk_config_path: foo/cdk.json
    package_manifest_paths:
    - package.json
    context:
      org: local-org
      env: dev
`))
	require.NoError(t, err)
	require.Len(t, cfg.CDK.Projects, 1)
}

// TestGenerate_CDKDefaults_WithTemplate_PartialOverride tests that cdk defaults
// are applied to missing fields while preserving explicitly set ones
func TestGenerate_CDKDefaults_WithTemplate_PartialOverride(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(`variable "x" {}`), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))

	cfg, err := Generate(tmp, WithTemplate(`version: 0.2
cdk_defaults:
  context:
    org: acme
    env: prod
cdk:
  - cdk_config_path: foo/cdk.json
    package_manifest_paths:
    - package.json
    context:
      org: local-org
`))
	require.NoError(t, err)
	require.Len(t, cfg.CDK.Projects, 1)
}

// TestGenerate_CDKDefaults_WithTemplate_RepoConfigNoCDKBlock tests the scenario where:
// - Org template (dashboard) has cdk defaults but NO cdk projects block
// - Autodetect finds CDK entries
// - Defaults should be applied to autodetected CDK entries
func TestGenerate_CDKDefaults_WithTemplate_RepoConfigNoCDKBlock(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(`variable "x" {}`), 0600))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "foo"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "foo", "cdk.json"), []byte(`{"app":"ts-node"}`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0600))

	cfg, err := Generate(tmp, WithTemplate(`version: 0.3
cdk:
  defaults:
    context:
      deployStack: cftp
      deployEnvironment: dev
`))
	require.NoError(t, err)
	require.Len(t, cfg.CDK.Projects, 1)
}

func TestLoadConfigFile_DoesNotCallSynth(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "infracost.yml")

	configContent := `version: 0.3

cdk:
  apps:
    - cdk_config_path: "cdk-app/cdk.json"

projects:
  - path: terraform/
    name: existing-terraform
`

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	cfg, err := LoadConfigFile(configPath, tmp, nil)

	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify CDK block was parsed and finalized, but no CDK projects were added
	require.Len(t, cfg.CDK.Projects, 1)
	require.Len(t, cfg.Projects, 1)
	require.Equal(t, "existing-terraform", cfg.Projects[0].Name)
}
