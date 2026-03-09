package autodetect

import (
	"os"
	"path/filepath"
	"strings"
)

// isPathAllowed checks if a path is within any of the supplied parent paths.
// It recursively resolves symlinks and returns false if there is an error resolving symlinks.
func isPathAllowed(path string, allowedParents ...string) bool {

	if path == "" {
		path = "."
	}

	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	pathResolved, err := recursivelyResolveSymlink(pathAbs)
	if err != nil {
		pathResolved = pathAbs
	}

	pathResolved = filepath.Clean(pathResolved)

	for _, parent := range allowedParents {
		if parent == "" {
			continue
		}
		parentAbs, err := filepath.Abs(parent)
		if err != nil {
			continue
		}

		parentResolved, err := recursivelyResolveSymlink(parentAbs)
		if err != nil {
			parentResolved = parentAbs
		}

		parentResolved = filepath.Clean(parentResolved)

		if strings.HasPrefix(pathResolved+string(filepath.Separator), parentResolved+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// recursivelyResolveSymlink resolves symlinks recursively.
func recursivelyResolveSymlink(path string) (string, error) {
	for isSymlink(path) {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		path = target
	}
	return path, nil
}

// isSymlink returns true if the path is a symlink.
func isSymlink(path string) bool {
	fileInfo, err := os.Lstat(path)
	return err == nil && fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink
}
