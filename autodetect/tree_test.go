package autodetect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSearchForProjects_PrunesExcludedDirs checks that projects under
// default-excluded directories (dependency stores, caches, editor metadata)
// are never detected, while a normal sibling project still is. The exclusion
// happens at walk time, so nothing under these directories is ever offered to
// plugin identification.
func TestSearchForProjects_PrunesExcludedDirs(t *testing.T) {
	repo := t.TempDir()
	tf := []byte("provider \"aws\" {}\n")

	excluded := []string{
		filepath.Join("node_modules", "some-pkg"),
		filepath.Join(".claude", "skills"),
		filepath.Join("services", "api", "node_modules", "nested"),
		"venv",
	}
	for _, dir := range append([]string{"app"}, excluded...) {
		require.NoError(t, os.MkdirAll(filepath.Join(repo, dir), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(repo, dir, "main.tf"), tf, 0600))
	}

	projects, _, err := SearchForProjects(context.Background(), repo)
	require.NoError(t, err)

	var paths []string
	for _, p := range projects {
		paths = append(paths, p.Path)
	}

	require.Len(t, paths, 1, "only the sibling project outside excluded dirs should be detected, got %v", paths)
	assert.Contains(t, paths[0], "app")
}
