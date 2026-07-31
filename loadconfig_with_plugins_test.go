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
