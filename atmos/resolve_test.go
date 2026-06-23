package atmos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func normalizeJSON(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var out any
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

func loadJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

// TestDescribe_MatchesAtmosGolden guards parity with real Atmos for the config copy of
// the resolver, using the same goldens as the parser copy (captured from real
// describe.ProcessComponentInStack with templates and YAML functions disabled).
func TestDescribe_MatchesAtmosGolden(t *testing.T) {
	for _, component := range []string{"app", "app2"} {
		t.Run(component, func(t *testing.T) {
			cfg, err := Describe("testdata/basic", "dev", component)
			require.NoError(t, err)
			golden := loadJSON(t, "testdata/basic/golden/"+component+"-dev.json")
			require.Equal(t, golden["component"], cfg.FinalComponent)
			require.Equal(t, golden["vars"], normalizeJSON(t, cfg.Vars))
			require.Equal(t, golden["providers"], normalizeJSON(t, cfg.Providers))
		})
	}
}

func TestEnumerate_ListsDeployableStackComponents(t *testing.T) {
	got, err := Enumerate("testdata/basic")
	require.NoError(t, err)
	require.Equal(t, []StackComponent{
		{Stack: "dev", Component: "app", FinalComponent: "app"},
		{Stack: "dev", Component: "app2", FinalComponent: "app"},
		{Stack: "dev", Component: "withmodule", FinalComponent: "withmodule"},
	}, got)
}

func TestManifestPath_RejectsTraversal(t *testing.T) {
	cfg, err := LoadConfig("testdata/basic")
	require.NoError(t, err)
	_, err = cfg.manifestPath("../atmos.yaml", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside the stacks directory")
}

// TestManifestPath_FileRelativeImport_RejectsTraversal ensures a "../" import resolved
// against the importing file's directory still cannot escape the stacks tree.
func TestManifestPath_FileRelativeImport_RejectsTraversal(t *testing.T) {
	cfg, err := LoadConfig("testdata/nametmpl")
	require.NoError(t, err)
	fromDir := filepath.Join(cfg.StacksBasePath, "aws")
	_, err = cfg.manifestPath("../../atmos.yaml", fromDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside the stacks directory")
}

// TestDescribe_NameTemplate guards parity with real Atmos for a repo using
// stacks.name_template and file-relative ../_defaults imports. Ground truth captured
// from describe.ExecuteDescribeStacks: stack "acct-dev__app", with account/region
// propagated through the relative imports.
func TestDescribe_NameTemplate(t *testing.T) {
	cfg, err := Describe("testdata/nametmpl", "acct-dev__app", "app")
	require.NoError(t, err)
	golden := loadJSON(t, "testdata/nametmpl/golden/app-acct-dev__app.json")
	require.Equal(t, golden["component"], cfg.FinalComponent)
	require.Equal(t, golden["vars"], normalizeJSON(t, cfg.Vars))
	require.Equal(t, golden["providers"], normalizeJSON(t, cfg.Providers))
}

// TestEnumerate_NameTemplate also covers skip-on-unresolvable: the fixture includes a
// tenant stack that imports a templated `.yaml.tmpl` catalog manifest, which v1 cannot
// resolve. That stack must be skipped (not error the whole repo), leaving only the
// resolvable name_template stack.
func TestEnumerate_NameTemplate(t *testing.T) {
	got, err := Enumerate("testdata/nametmpl")
	require.NoError(t, err)
	require.Equal(t, []StackComponent{
		{Stack: "acct-dev__app", Component: "app", FinalComponent: "app"},
	}, got)
}

// TestDescribe_SurfacesResolveErrors ensures that when the requested stack is not
// found, any per-file resolution failures (e.g. the templated tenant stack) are
// surfaced in the error rather than masked as a bare "not found".
func TestDescribe_SurfacesResolveErrors(t *testing.T) {
	_, err := Describe("testdata/nametmpl", "does-not-exist", "app")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
	require.Contains(t, err.Error(), "could not be resolved")
	require.Contains(t, err.Error(), "multi-tenant")
}

func TestPathIncluded_MatchesFilenameAndExtensionlessGlobs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		included []string
		excluded []string
		want     bool
	}{
		{"extensionless include", []string{"deploy/**/*"}, nil, true},
		{"filename include", []string{"deploy/**/*.yaml"}, nil, true},
		{"non-matching include", []string{"other/**/*"}, nil, false},
		{"excluded by filename glob", []string{"deploy/**/*"}, []string{"**/dev.yaml"}, false},
		{"excluded by extensionless glob", []string{"deploy/**/*"}, []string{"deploy/dev"}, false},
		{"no includes means all", nil, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{IncludedPaths: tc.included, ExcludedPaths: tc.excluded}
			require.Equal(t, tc.want, c.pathIncluded("deploy/dev.yaml", "deploy/dev"))
		})
	}
}
