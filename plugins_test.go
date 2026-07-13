package config_test

import (
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
	assert.Empty(t, p.Terraform.VarFiles, "generation must not emit the deprecated terraform.var_files field")
	assert.Equal(t, config.ProjectTypeTerraform, p.Type)
	assert.Contains(t, toStringSlice(p.Plugins["terraform"]["var_files"]), "terraform.tfvars")
}
