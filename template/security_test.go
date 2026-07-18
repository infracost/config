package template

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSymlinkRepo builds a repo tree that mirrors the FIX-313 PoC:
//
//	repo/                 -- the repository root the parser is confined to
//	  inside/secret.tf    -- a legitimate in-repo file
//	  evil -> outside     -- an intermediate symlink escaping the repo
//	  finlink -> outside/secret  -- a leaf symlink escaping the repo
//	outside/secret        -- a secret that lives outside the repo root
//
// It returns the (absolute, symlink-resolved) repo dir.
func setupSymlinkRepo(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	outside := filepath.Join(tmp, "outside")

	require.NoError(t, os.MkdirAll(filepath.Join(repo, "inside"), 0700))
	require.NoError(t, os.MkdirAll(outside, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "inside", "secret.tf"), []byte("in-repo"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("TOP-SECRET"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(repo, "evil")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(repo, "finlink")))

	return repo
}

func TestParser_readFile_confinement(t *testing.T) {
	repo := setupSymlinkRepo(t)
	p := NewParser(repo, Variables{}, nil)

	// Legitimate in-repo read works.
	assert.Equal(t, "in-repo", p.readFile("inside/secret.tf"))

	// Lexical parent traversal is blocked.
	assert.Panics(t, func() { p.readFile("../outside/secret") },
		"parent-directory traversal must be blocked")

	// Leaf symlink escaping the repo is blocked.
	assert.Panics(t, func() { p.readFile("finlink") },
		"leaf symlink escaping the repo must be blocked")

	// Intermediate symlink escaping the repo is blocked (the FIX-313 gap).
	assert.Panics(t, func() { p.readFile("evil/secret") },
		"intermediate symlink escaping the repo must be blocked")
}

func TestParser_pathExists_confinement(t *testing.T) {
	repo := setupSymlinkRepo(t)
	p := NewParser(repo, Variables{}, nil)

	// Legitimate in-repo path exists.
	assert.True(t, p.pathExists("inside", "secret.tf"))

	// Parent traversal to a real file outside the repo reports false.
	assert.False(t, p.pathExists(".", "../outside/secret"),
		"parent-directory traversal must not be reported as existing")

	// Intermediate symlink escaping the repo reports false even though the
	// underlying file exists.
	assert.False(t, p.pathExists("evil", "secret"),
		"path traversing an intermediate symlink must not be reported as existing")

	// A base that is itself an escaping symlink reports false.
	assert.False(t, p.pathExists("evil", ""),
		"a base that escapes the repo via a symlink must not be reported as existing")
}

func TestParser_isDir_confinement(t *testing.T) {
	repo := setupSymlinkRepo(t)
	p := NewParser(repo, Variables{}, nil)

	// Legitimate in-repo directory.
	assert.True(t, p.isDir("inside"))

	// The intermediate symlink resolves to an out-of-repo directory and must
	// not be reported as an in-repo directory.
	assert.False(t, p.isDir("evil"),
		"a symlinked directory escaping the repo must not be reported as a dir")
}

func TestParser_matchPaths_confinement(t *testing.T) {
	repo := setupSymlinkRepo(t)
	p := NewParser(repo, Variables{}, nil)

	matches := p.matchPaths(":dir/secret")

	// Only the in-repo inside/secret.tf-style entries should ever surface;
	// nothing reached via the escaping `evil` symlink may appear.
	for _, m := range matches {
		assert.NotEqual(t, "evil", m["_dir"],
			"matchPaths must not return entries reached via an escaping symlink")
	}
}

// TestParser_readFile_errorDoesNotLeakAbsolutePath asserts that readFile
// failures report only the relative path the caller passed and never leak the
// resolved absolute path (e.g. the runner's working directory) or the raw os
// error text.
func TestParser_readFile_errorDoesNotLeakAbsolutePath(t *testing.T) {
	repo := setupSymlinkRepo(t)
	p := NewParser(repo, Variables{}, nil)

	// assertNoLeak runs fn (expected to panic) and checks the recovered panic
	// message contains the relative path but not the absolute repo dir.
	assertNoLeak := func(t *testing.T, relPath string, fn func()) {
		t.Helper()
		defer func() {
			r := recover()
			require.NotNil(t, r, "expected a panic")
			msg, ok := r.(string)
			require.True(t, ok, "panic value must be a string, got %T", r)

			assert.NotContains(t, msg, repo,
				"error must not leak the absolute repo path")
			assert.NotContains(t, msg, filepath.Dir(repo),
				"error must not leak the parent of the repo path")
			assert.NotContains(t, msg, string(filepath.Separator)+"private",
				"error must not leak any absolute filesystem path")
			assert.Contains(t, msg, relPath,
				"error should reference the relative path the caller passed")
		}()
		fn()
	}

	// A missing in-repo file: reports "file does not exist", relative path only.
	assertNoLeak(t, "inside/missing.tf", func() { p.readFile("inside/missing.tf") })

	// A directory (not a regular file) read fails with a non-not-exist error and
	// must fall back to a uniform "permission denied" without the absolute path.
	assertNoLeak(t, "inside", func() { p.readFile("inside") })
}

// TestParser_Compile_readFile_errorDoesNotLeakAbsolutePath drives the real
// template entrypoint and asserts the surfaced error stays path-clean.
func TestParser_Compile_readFile_errorDoesNotLeakAbsolutePath(t *testing.T) {
	repo := setupSymlinkRepo(t)
	p := NewParser(repo, Variables{}, nil)

	var buf bytes.Buffer
	err := p.Compile(`{{ readFile "inside/missing.tf" }}`, &buf)

	require.Error(t, err)
	assert.NotContains(t, err.Error(), repo,
		"surfaced template error must not leak the absolute repo path")
	assert.NotContains(t, err.Error(), filepath.Dir(repo),
		"surfaced template error must not leak the parent of the repo path")
	assert.True(t, strings.Contains(err.Error(), "inside/missing.tf"),
		"surfaced template error should reference the relative path")
}

// TestParser_Compile_readFile_blocksIntermediateSymlink drives the real
// template entrypoint (Compile) with the exact PoC payload from FIX-313 and
// asserts the out-of-repo secret is never rendered into the output.
func TestParser_Compile_readFile_blocksIntermediateSymlink(t *testing.T) {
	repo := setupSymlinkRepo(t)
	p := NewParser(repo, Variables{}, nil)

	var buf bytes.Buffer
	err := p.Compile(`{{ readFile "evil/secret" }}`, &buf)

	require.Error(t, err, "rendering a template that escapes the repo must error")
	assert.NotContains(t, buf.String(), "TOP-SECRET",
		"the out-of-repo secret must never be rendered into the output")
}
