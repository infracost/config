// Package security contains security related functions, mainly checking a file is within a given path, and symlink handling
package security

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SafePath returns path expressed relative to base, for safe inclusion in
// user-facing errors. Absolute paths (e.g. a runner's working directory) must
// never be leaked, so if path cannot be made relative to base — or base is
// empty — only the final path element is returned. The result is always a
// repo-relative-style string, never the absolute filesystem layout.
func SafePath(base, path string) string {
	if base != "" {
		if rel, err := filepath.Rel(base, path); err == nil {
			return rel
		}
	}
	return filepath.Base(path)
}

// SafeErr returns the underlying reason for err with any embedded absolute path
// stripped, for safe inclusion in user-facing errors. The os file primitives
// (Open/ReadFile/ReadDir/Stat) wrap their cause in *fs.PathError and the link
// primitives in *os.LinkError, both of which carry the absolute path in a
// separate field from the reason; this returns only the reason. Any other error
// type yields a generic message, so a raw error string can never leak a path.
func SafeErr(err error) string {
	if err == nil {
		return ""
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err.Error()
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Err.Error()
	}
	return "operation failed"
}

// IsPathAllowed checks if a path is within any of the supplied parent paths.
// It is the containment security boundary: it resolves symlinks anywhere in
// the path (leaf AND intermediate directory symlinks) before comparing against
// the resolved parents, and returns false if the resolved path escapes every
// supplied parent. An in-repo symlink (e.g. evil -> /etc) therefore cannot be
// used to read files outside the allowed parents via a path like
// evil/passwd whose leaf isn't itself a symlink.
func IsPathAllowed(path string, allowedParents ...string) bool {
	if path == "" {
		path = "."
	}

	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	pathResolved := resolveFully(pathAbs)

	for _, parent := range allowedParents {
		if parent == "" {
			continue
		}
		parentAbs, err := filepath.Abs(parent)
		if err != nil {
			continue
		}

		parentResolved := resolveFully(parentAbs)

		if strings.HasPrefix(pathResolved+string(filepath.Separator), parentResolved+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// resolveFully canonicalises every symlink in the path (leaf and intermediate)
// via filepath.EvalSymlinks. EvalSymlinks errors when any component of the path
// doesn't exist (e.g. existence checks for candidate paths that aren't there
// yet). For those cases we walk up to the longest existing prefix, resolve
// that, then rejoin the missing tail — so prefix-matching still aligns with how
// parents are themselves resolved. The result is always cleaned and absolute
// (callers pass absolute paths).
func resolveFully(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}

	// path or some ancestor doesn't exist. Walk up until we find an existing
	// ancestor, resolve that, then append the missing tail.
	missing := []string{}
	dir := path
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			// hit root without finding anything that exists; nothing to
			// resolve, return the original cleaned path.
			return filepath.Clean(path)
		}
		missing = append([]string{filepath.Base(dir)}, missing...)
		dir = parent
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Clean(filepath.Join(append([]string{resolved}, missing...)...))
		}
	}
}

// RecursivelyResolveSymlink follows leaf symlinks to their target, returning a
// real path that can be read or stored. It is NOT the containment boundary —
// callers must still pass the result through IsPathAllowed, which independently
// performs the hardened (intermediate-symlink-aware) containment check. This
// deliberately only resolves the leaf, leaving intermediate non-symlink
// components (e.g. /var on macOS, which is itself a symlink to /private/var)
// untouched, so stored/returned paths stay consistent with the repo root the
// caller derived its other paths from.
func RecursivelyResolveSymlink(path string) (string, error) {
	for IsSymlink(path) {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		path = target
	}
	return path, nil
}

// IsSymlink returns true if the path's leaf is a symlink.
func IsSymlink(path string) bool {
	fileInfo, err := os.Lstat(path)
	return err == nil && fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink
}
