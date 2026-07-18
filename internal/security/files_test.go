package security

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsPathAllowed_BlocksIntermediateSymlinkTraversal locks in the security
// posture that intermediate directory symlinks are resolved before the
// allowed-parents check. Before this fix, a legitimately-named path like
// /repo-root/dir-link/passwd where dir-link points outside the repo would
// pass IsPathAllowed because the leaf wasn't a symlink — letting template
// readFile read files outside the sandbox.
func TestIsPathAllowed_BlocksIntermediateSymlinkTraversal(t *testing.T) {
	// repo/    -- the allowed parent
	//   inside/ -- a directory inside repo (touched so it exists)
	//   link  -- symlink pointing OUTSIDE repo to ../outside
	// outside/
	//   secret -- the file we shouldn't be able to reach via repo/link/secret
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	outside := filepath.Join(tmp, "outside")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "inside"), 0700))
	require.NoError(t, os.MkdirAll(outside, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("nope"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(repo, "link")))

	// /repo/inside/file (entirely within repo) — allowed
	assert.True(t, IsPathAllowed(filepath.Join(repo, "inside", "anything"), repo),
		"a path inside the allowed parent should be allowed")

	// /repo/link/secret — link points OUTSIDE repo. The leaf "secret" is
	// not a symlink, but the intermediate "link" is. This is the case the
	// old leaf-only resolver missed.
	assert.False(t, IsPathAllowed(filepath.Join(repo, "link", "secret"), repo),
		"a path traversing an intermediate symlink that escapes the allowed parent must be rejected")

	// /repo/link (the symlink itself, leaf is the symlink) — also points
	// outside, must be rejected.
	assert.False(t, IsPathAllowed(filepath.Join(repo, "link"), repo),
		"a leaf-symlink that targets outside the allowed parent must be rejected")
}

// TestIsPathAllowed_HandlesNonExistentPaths verifies the longest-existing-
// prefix resolver — existence checks walk candidate paths that don't exist
// yet, and the security check must still recognize them as inside the
// allowed parent.
func TestIsPathAllowed_HandlesNonExistentPaths(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	require.NoError(t, os.MkdirAll(repo, 0700))

	// Candidate file doesn't exist yet. Still inside repo.
	assert.True(t, IsPathAllowed(filepath.Join(repo, "doesnotexist.tf"), repo),
		"a non-existent path inside the allowed parent should be allowed")

	// Candidate file with a non-existent intermediate dir, still inside repo.
	assert.True(t, IsPathAllowed(filepath.Join(repo, "missing", "nested", "file.tf"), repo),
		"a deeply nested non-existent path inside the allowed parent should be allowed")

	// Candidate file outside the repo — even if it doesn't exist, must be rejected.
	assert.False(t, IsPathAllowed(filepath.Join(tmp, "elsewhere", "file.tf"), repo),
		"a non-existent path outside the allowed parent must be rejected")
}

// TestSafePath_NeverLeaksAbsolutePath locks in that SafePath returns a
// repo-relative string and never the absolute filesystem layout.
func TestSafePath_NeverLeaksAbsolutePath(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "usr", "src", "app", "infracost", "runner")

	// In-repo path is made relative to base.
	assert.Equal(t, filepath.Join("etc", "passwd"),
		SafePath(base, filepath.Join(base, "etc", "passwd")))

	// An escaping path becomes a "../"-style relative path, not the absolute one.
	got := SafePath(base, filepath.Join(string(filepath.Separator), "etc", "passwd"))
	assert.NotContains(t, got, base, "must not leak the absolute base path")
	assert.Contains(t, got, "passwd")

	// With no base, only the final element is returned — never the full path.
	assert.Equal(t, "passwd",
		SafePath("", filepath.Join(string(filepath.Separator), "etc", "secrets", "passwd")))
}

// TestSafeErr_NeverLeaksAbsolutePath locks in that SafeErr returns only the
// underlying reason, with the embedded absolute path stripped.
func TestSafeErr_NeverLeaksAbsolutePath(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "usr", "src", "app", "infracost", "runner", "etc", "passwd")

	// A *fs.PathError (as returned by os.ReadFile/Open/ReadDir) keeps the reason
	// but drops the path.
	pathErr := &fs.PathError{Op: "open", Path: abs, Err: fs.ErrNotExist}
	got := SafeErr(pathErr)
	assert.NotContains(t, got, abs, "must not leak the absolute path")
	assert.Equal(t, fs.ErrNotExist.Error(), got)

	// A *os.LinkError (as returned by symlink ops) is handled the same way.
	linkErr := &os.LinkError{Op: "symlink", Old: abs, New: abs, Err: fs.ErrPermission}
	got = SafeErr(linkErr)
	assert.NotContains(t, got, abs, "must not leak the absolute path")
	assert.Equal(t, fs.ErrPermission.Error(), got)

	// An opaque error that happens to embed a path yields a generic message,
	// never the path itself.
	got = SafeErr(errors.New("stat " + abs + ": boom"))
	assert.NotContains(t, got, abs, "must not leak a path embedded in an opaque error")
	assert.Equal(t, "operation failed", got)

	assert.Equal(t, "", SafeErr(nil))
}
