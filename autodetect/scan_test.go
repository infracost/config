package autodetect

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContainsFold(t *testing.T) {
	tests := []struct {
		name     string
		haystack string
		needle   string
		want     bool
	}{
		{"exact match", "awstemplateformatversion", "awstemplateformatversion", true},
		{"upper haystack", `{"RESOURCES": {}}`, "resources", true},
		{"mixed case", `{"AwsTemplateFormatVersion": ""}`, "awstemplateformatversion", true},
		{"needle at start", "resources:", "resources", true},
		{"needle at end", "my-resources", "resources", true},
		{"absent", `{"name": "not-a-template"}`, "awstemplateformatversion", false},
		{"partial overlap only", "resource", "resources", false},
		{"empty needle", "anything", "", true},
		{"empty haystack", "", "resources", false},
		{"haystack shorter than needle", "res", "resources", false},
		{"repeated first byte", "rrrrresources", "resources", true},
		{"non-ascii bytes untouched", "\xc3\xa9 Resources: {}", "resources", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsFold([]byte(tt.haystack), tt.needle); got != tt.want {
				t.Fatalf("containsFold(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}

// TestContainsFold_AgreesWithStdlib cross-checks the scanner against the
// stdlib equivalent it replaces (which allocates a lowered copy).
func TestContainsFold_AgreesWithStdlib(t *testing.T) {
	haystacks := []string{
		"", "a", "AbCdEfG", "resources RESOURCES Resources",
		`{"AWSTemplateFormatVersion": "2010-09-09"}`,
		strings.Repeat("x", 1000) + "Format_Version",
	}
	needles := []string{"", "a", "resources", "awstemplateformatversion", "format_version", "zzz"}
	for _, h := range haystacks {
		for _, n := range needles {
			want := bytes.Contains(bytes.ToLower([]byte(h)), []byte(n))
			if got := containsFold([]byte(h), n); got != want {
				t.Fatalf("containsFold(%q, %q) = %v, stdlib says %v", h, n, got, want)
			}
		}
	}
}

// The identification markers must be lower-case: containsFold folds the
// haystack only and assumes an already-lowered needle.
func TestIdentifyMarkersAreLowercase(t *testing.T) {
	for _, m := range []string{markerFormatVersion, markerResources} {
		if m != strings.ToLower(m) {
			t.Fatalf("marker %q must be lower-case for containsFold", m)
		}
	}
}

func TestReadCapped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := readCapped(path, 10); string(got) != "0123456789" {
		t.Fatalf("file at exactly the cap should be read fully, got %q", got)
	}
	if got := readCapped(path, 9); got != nil {
		t.Fatalf("file over the cap should return nil, got %q", got)
	}
	if got := readCapped(filepath.Join(dir, "missing"), 10); got != nil {
		t.Fatalf("missing file should return nil, got %q", got)
	}
}
