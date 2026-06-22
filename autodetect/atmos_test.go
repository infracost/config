package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/infracost/config/types"
	"github.com/stretchr/testify/require"
)

func TestDetectAtmosProjects(t *testing.T) {
	projects, covered, err := DetectAtmosProjects("../atmos/testdata/basic")
	require.NoError(t, err)

	require.True(t, covered["components/terraform"], "the terraform components dir must be marked covered")

	byName := map[string]Project{}
	for _, p := range projects {
		byName[p.Name] = p
	}
	require.Contains(t, byName, "atmos/dev/app")
	require.Contains(t, byName, "atmos/dev/app2")
	require.Contains(t, byName, "atmos/dev/withmodule")

	app := byName["atmos/dev/app"]
	require.Equal(t, types.ProjectTypeAtmos, app.Type)
	require.Equal(t, "atmos/dev/app", app.Path)
	require.Equal(t, "dev", app.Env)
	require.Equal(t, "dev", app.Metadata["atmos_stack"])
	require.Equal(t, "app", app.Metadata["atmos_component"])
	require.Contains(t, app.DependencyPaths, "atmos.yaml")
	require.Contains(t, app.DependencyPaths, "components/terraform/**", "must watch the whole tf components base for sibling local modules")
	require.Contains(t, app.DependencyPaths, "stacks/**", "must watch the configured stacks base")
}

func TestDetectAtmosProjects_NotAtmosRepo(t *testing.T) {
	projects, covered, err := DetectAtmosProjects(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, projects)
	require.Empty(t, covered)
}

func TestDetectAtmosProjects_MalformedAtmosYAML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(":\n  invalid: yaml: :::"), 0600))
	_, _, err := DetectAtmosProjects(dir)
	require.Error(t, err, "a present-but-broken atmos.yaml must surface an error, not silently fall through")
}
