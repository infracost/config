package autodetect

import (
	"io"
	"os"
)

// Shared helpers for content-based project identification, copied from the
// parser repo's internal/scan package (internal, so it can't be imported
// across repos). Identification runs over every candidate file in a repo, so
// these helpers exist to reject files cheaply (a byte scan) before paying for
// a full JSON/YAML decode.

// containsFold reports whether haystack contains needle under ASCII
// case-folding. needle must already be lower-case (it is in practice a
// package-level constant, so this is checked by the callers' tests rather
// than at runtime). Non-ASCII bytes are compared verbatim, which is correct
// for the ASCII marker keys identification scans for.
func containsFold(haystack []byte, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	first := needle[0]
	firstUpper := toUpperASCII(first)
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if c := haystack[i]; c != first && c != firstUpper {
			continue
		}
		if hasPrefixFold(haystack[i:], needle) {
			return true
		}
	}
	return false
}

func hasPrefixFold(haystack []byte, needle string) bool {
	for i := 0; i < len(needle); i++ {
		if toLowerASCII(haystack[i]) != needle[i] {
			return false
		}
	}
	return true
}

func toLowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func toUpperASCII(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

// readCapped reads path, returning nil if it can't be read or holds more than
// maxBytes. Identification sniffs whole documents, and a document truncated at
// maxBytes would fail to decode anyway, so oversized files are skipped
// outright rather than partially read.
func readCapped(path string, maxBytes int64) []byte {
	f, err := os.Open(path) // #nosec G304 — identification confines paths to the scanned directory
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	content, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || int64(len(content)) > maxBytes {
		return nil
	}
	return content
}
