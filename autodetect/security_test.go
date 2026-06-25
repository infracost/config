package autodetect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// symlinkRepo builds a tree shaped like the FIX-313 PoC and returns the repo
// root plus the out-of-repo secret file:
//
//	repo/                    -- the allowed root
//	  inside/main.tf         -- a legitimate in-repo terraform file
//	  evil -> outside        -- an intermediate directory symlink escaping repo
//	  finlink -> outside/secret -- a leaf symlink escaping repo
//	outside/secret           -- a file that lives outside the repo root
func symlinkRepo(t *testing.T) (repo string) {
	t.Helper()

	tmp := t.TempDir()
	repo = filepath.Join(tmp, "repo")
	outside := filepath.Join(tmp, "outside")

	require.NoError(t, os.MkdirAll(filepath.Join(repo, "inside"), 0700))
	require.NoError(t, os.MkdirAll(outside, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "inside", "main.tf"), []byte("# tf"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("TOP-SECRET"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(repo, "evil")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(repo, "finlink")))

	return repo
}

// TestReadFileWithSymlinkResolution_Confinement exercises the search.go call
// site directly: an in-repo read works, but reads that escape the allowed dirs
// via a leaf OR an intermediate symlink are rejected.
func TestReadFileWithSymlinkResolution_Confinement(t *testing.T) {
	repo := symlinkRepo(t)
	allowed := []string{repo}

	// Legitimate in-repo read works.
	b, err := readFileWithSymlinkResolution(filepath.Join(repo, "inside", "main.tf"), allowed)
	require.NoError(t, err)
	assert.Equal(t, "# tf", string(b))

	// Leaf symlink escaping the repo is rejected.
	_, err = readFileWithSymlinkResolution(filepath.Join(repo, "finlink"), allowed)
	require.Error(t, err, "leaf symlink escaping the allowed dirs must be rejected")

	// Intermediate symlink escaping the repo is rejected (the FIX-313 gap):
	// evil/secret resolves to outside/secret even though the "secret" leaf is
	// not itself a symlink.
	_, err = readFileWithSymlinkResolution(filepath.Join(repo, "evil", "secret"), allowed)
	require.Error(t, err, "intermediate symlink escaping the allowed dirs must be rejected")
}

// TestBuildSubtree_RejectsIntermediateSymlinkEscape exercises the tree.go call
// site: walking into a path that traverses an intermediate symlink out of the
// allowed dirs must be refused.
func TestBuildSubtree_RejectsIntermediateSymlinkEscape(t *testing.T) {
	repo := symlinkRepo(t)
	b := newTreeBuilder(nil, repo, &Config{MaxSearchDepth: 10}, false, false, false, repo)

	// repo/evil/<dir> resolves under the out-of-repo "outside" directory.
	_, err := b.buildSubtree(context.Background(), filepath.Join(repo, "evil"), 0, nil)
	require.Error(t, err, "buildSubtree must refuse a path escaping the allowed dirs via a symlink")
	assert.Contains(t, err.Error(), "not allowed")
}

// TestBuildSubtree_PreservesRepoRootPath is a regression test for the
// canonicalisation bug: the hardened containment check must NOT rewrite
// legitimate in-repo paths. On macOS t.TempDir() lives under /var, which is
// itself a symlink to /private/var — if buildSubtree canonicalised every
// component it would store /private/var/... and break repo-relative path
// computation (filepath.Rel against the un-canonicalised repoRoot).
func TestBuildSubtree_PreservesRepoRootPath(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(repo, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "main.tf"), []byte("# tf"), 0600))

	b := newTreeBuilder(nil, repo, &Config{MaxSearchDepth: 10}, false, false, false, repo)

	root, err := b.build(context.Background())
	require.NoError(t, err)
	assert.Equal(t, repo, root.AbsolutePath,
		"a legitimate in-repo directory's path must be preserved verbatim, not canonicalised")
}
