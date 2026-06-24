package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGenerate_AtmosProjects exercises the full config.Generate path on an Atmos repo:
// it must emit one project per (stack, component) and filter out the plain-Terraform
// projects that autodetect would otherwise find under components/terraform.
func TestGenerate_AtmosProjects(t *testing.T) {
	cfg, err := Generate(context.Background(), "atmos/testdata/basic")
	require.NoError(t, err)

	atmosNames := map[string]*Project{}
	for _, p := range cfg.Projects {
		if p.Type == ProjectTypeAtmos {
			atmosNames[p.Name] = p
		}
		require.False(t,
			p.Type == ProjectTypeTerraform && strings.HasPrefix(p.Path, "components/terraform"),
			"plain terraform project under components/terraform should be filtered in favour of atmos: %s", p.Path)
	}

	require.Contains(t, atmosNames, "atmos/dev/app")
	require.Contains(t, atmosNames, "atmos/dev/app2")
	require.Contains(t, atmosNames, "atmos/dev/withmodule")

	app := atmosNames["atmos/dev/app"]
	require.Equal(t, "dev", app.Metadata["atmos_stack"])
	require.Equal(t, "app", app.Metadata["atmos_component"])
	require.Equal(t, "", app.Terraform.Workspace, "Atmos projects must not set TF workspace to the stack name")
}

// TestNormalize_AtmosProjectDoesNotInheritWorkspace guards that an Atmos project keeps
// its empty Terraform workspace through normalize() even when a global default workspace
// is set. normalize() runs on both generation and reload of a generated config (which
// serializes type: atmos), so without the exemption the stack would silently inherit a
// TF workspace and diverge from real Atmos.
func TestNormalize_AtmosProjectDoesNotInheritWorkspace(t *testing.T) {
	c := &Config{
		Terraform: Terraform{Defaults: TerraformDefaults{Workspace: "global-ws"}},
		Projects: []*Project{
			{Name: "tf", Type: ProjectTypeTerraform, Path: "a"},
			{Name: "atmos", Type: ProjectTypeAtmos, Path: "b"},
		},
	}
	require.NoError(t, c.normalize())

	byName := map[string]*Project{}
	for _, p := range c.Projects {
		byName[p.Name] = p
	}
	require.Equal(t, "global-ws", byName["tf"].Terraform.Workspace, "non-Atmos projects still inherit the default workspace")
	require.Equal(t, "", byName["atmos"].Terraform.Workspace, "Atmos projects must not inherit a Terraform workspace")
}

// TestGenerate_AtmosNoProjects_NameTemplate ensures a name_template repo whose stacks
// declare no Terraform components produces zero Atmos projects rather than crashing.
// (name_template itself is supported; here there are simply no components to enumerate.)
func TestGenerate_AtmosNoProjects_NameTemplate(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`
base_path: "."
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths:
    - "deploy/**/*"
  name_template: "{{ .vars.tenant }}-{{ .vars.env }}"
`), 0600)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks", "deploy"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "prod.yaml"), []byte("vars:\n  tenant: acme\n  env: prod\n"), 0600))

	cfg, err := Generate(context.Background(), dir)
	require.NoError(t, err)
	for _, p := range cfg.Projects {
		require.NotEqual(t, ProjectTypeAtmos, p.Type, "name_template repos must produce no Atmos projects in v1")
	}
}
