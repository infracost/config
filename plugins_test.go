package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/infracost/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Generation persists terraform var files under plugins.<type>.var_files (the key the terraform
// plugin reads) and no longer writes the deprecated top-level terraform.var_files field.
func TestPlugins_GenerationSideloadsVarFilesIntoBlob(t *testing.T) {
	root := NewFilesystem(t)
	root.AddTerraformFileWithProviderBlock("main.tf")
	root.AddTFVarsFile("terraform.tfvars")

	cfg, err := config.Generate(t.Context(), root.Path())
	require.NoError(t, err)
	require.Len(t, cfg.Projects, 1)

	p := cfg.Projects[0]
	assert.Equal(t, config.ProjectTypeTerraform, p.Type)
	// the canonical representation is the plugins blob...
	assert.Contains(t, toStringSlice(p.Plugins["terraform"]["var_files"]), "terraform.tfvars")
	// ...and the deprecated terraform.var_files field is populated as a backwards-compat mirror of it.
	assert.Contains(t, p.Terraform.VarFiles, "terraform.tfvars")
}

// Repo-level plugin defaults for every plugin live under a <plugin>.defaults: sub-key (consistent with
// terraform.defaults) and deep-merge into each matching project's blob, with the per-project value
// winning. A repo-level key placed directly (not under defaults:) is ignored.
func TestPlugins_RepoDefaultsUnderDefaultsKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "infracost.yml")
	require.NoError(t, os.WriteFile(path, []byte(`version: "0.3"
kubernetes:
  defaults:
    namespace: default
    replicas: 1
terraform:
  ignored_direct_key: nope
  defaults:
    workspace: prod
    vars:
      env: staging
projects:
  - path: k8s
    type: kubernetes
    kubernetes:
      replicas: 3
  - path: other
    type: kubernetes
  - path: tf
    type: terraform
`), 0o600))

	cfg, err := config.LoadConfigFile(t.Context(), path, dir)
	require.NoError(t, err)

	byPath := map[string]*config.Project{}
	for _, p := range cfg.Projects {
		byPath[p.Path] = p
	}

	// per-project value wins over the repo default; unset default is inherited
	assert.Equal(t, 3, byPath["k8s"].Plugins["kubernetes"]["replicas"], "per-project replicas wins")
	assert.Equal(t, "default", byPath["k8s"].Plugins["kubernetes"]["namespace"], "repo default namespace inherited")

	// a project with no own blob inherits the repo defaults wholesale
	assert.Equal(t, "default", byPath["other"].Plugins["kubernetes"]["namespace"])
	assert.Equal(t, 1, byPath["other"].Plugins["kubernetes"]["replicas"])

	// terraform.defaults splits: workspace is a typed default (inherited on the typed field), while the
	// rest (vars) is a blob default merged into the terraform project's blob.
	assert.Equal(t, "prod", byPath["tf"].Terraform.Workspace, "typed default inherited")
	assert.Equal(t, map[string]any{"env": "staging"}, byPath["tf"].Plugins["terraform"]["vars"], "blob default merged")
	assert.NotContains(t, byPath["tf"].Plugins["terraform"], "workspace", "typed key never leaks into the blob")

	// a repo-level key placed directly (not under defaults:) is not applied as a default
	for _, p := range cfg.Projects {
		assert.NotContains(t, p.Plugins["terraform"], "ignored_direct_key")
	}
}
