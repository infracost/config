package config_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/infracost/config"
)

const (
	pluginManifestURL = "https://infracost.github.io/cli/manifest.json"
	pluginCacheDir    = ".test-plugins"
	pluginBinarySize  = 1 << 30 // 1 GB cap for archive entries
)

var requiredPlugins = []string{
	"infracost-plugin-terraform",
	"infracost-plugin-terragrunt",
	"infracost-plugin-cloudformation",
	"infracost-plugin-ciscostacks",
}

// pluginDir is the directory the test plugins are extracted into. It is
// populated by TestMain before any tests run.
var pluginDir string

func TestMain(m *testing.M) {
	dir, err := installTestPlugins()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to install test plugins: %v\n", err)
		os.Exit(1)
	}
	pluginDir = dir
	os.Exit(m.Run())
}

// testConfigGenerationWithPlugins is the same as testConfigGeneration but
// injects WithPluginDir(pluginDir) so config.Generate runs against the
// downloaded plugin binaries.
func testConfigGenerationWithPlugins(t *testing.T, dir string, wantProjects []*config.Project, opts ...config.GenerationOption) {
	opts = append(opts, config.WithPluginDir(pluginDir))
	testConfigGeneration(t, dir, wantProjects, opts...)
}

// testConfigGenerationWithTemplateAndPlugins is the same as
// testConfigGenerationWithTemplate but injects WithPluginDir(pluginDir).
func testConfigGenerationWithTemplateAndPlugins(t *testing.T, dir, template string, wantProjects []*config.Project, opts ...config.GenerationOption) {
	opts = append(opts, config.WithPluginDir(pluginDir))
	testConfigGenerationWithTemplate(t, dir, template, wantProjects, opts...)
}

// installTestPlugins resolves the manifest, downloads the latest version of
// each required plugin for the current OS/arch (skipping any already present),
// extracts the binary, and returns the directory the binaries live in.
func installTestPlugins() (string, error) {
	dir, err := pluginInstallDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create plugin dir: %w", err)
	}

	manifest, err := fetchPluginManifest()
	if err != nil {
		return "", fmt.Errorf("fetch manifest: %w", err)
	}

	platform := runtime.GOOS + "_" + runtime.GOARCH

	var wg sync.WaitGroup
	errs := make([]error, len(requiredPlugins))
	for i, name := range requiredPlugins {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			errs[i] = ensurePlugin(dir, manifest, name, platform)
		}(i, name)
	}
	wg.Wait()

	if err := errors.Join(errs...); err != nil {
		return "", err
	}

	return dir, nil
}

func pluginInstallDir() (string, error) {
	// Anchor on this source file's directory so the cache lives in the repo
	// root regardless of which package the `go test` was invoked from.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("could not determine caller path")
	}
	return filepath.Join(filepath.Dir(file), pluginCacheDir), nil
}

type pluginManifest struct {
	Plugins map[string]struct {
		Latest   string                         `json:"latest"`
		Versions map[string]pluginManifestEntry `json:"versions"`
	} `json:"plugins"`
}

type pluginManifestEntry struct {
	Artifacts map[string]pluginArtifact `json:"artifacts"`
}

type pluginArtifact struct {
	URL  string `json:"url"`
	SHA  string `json:"sha"`
	Name string `json:"name"`
}

func fetchPluginManifest() (*pluginManifest, error) {
	resp, err := http.Get(pluginManifestURL) //nolint:gosec // G107: constant URL
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest fetch returned %s", resp.Status)
	}

	var m pluginManifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func ensurePlugin(destDir string, manifest *pluginManifest, name, platform string) error {
	plugin, ok := manifest.Plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not in manifest", name)
	}
	if plugin.Latest == "" {
		return fmt.Errorf("plugin %q has no latest version", name)
	}
	version, ok := plugin.Versions[plugin.Latest]
	if !ok {
		return fmt.Errorf("plugin %q latest version %s missing from manifest", name, plugin.Latest)
	}
	artifact, ok := version.Artifacts[platform]
	if !ok {
		return fmt.Errorf("plugin %q has no artifact for %s", name, platform)
	}

	binaryName := name
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(destDir, binaryName)

	// Cache by storing the installed SHA alongside the binary so we can skip
	// re-downloading when the manifest hasn't moved on.
	shaPath := binaryPath + ".sha"
	if existing, err := os.ReadFile(shaPath); err == nil && string(existing) == artifact.SHA {
		if _, err := os.Stat(binaryPath); err == nil {
			return nil
		}
	}

	archivePath, err := downloadAndVerify(artifact.URL, artifact.SHA, name)
	if err != nil {
		return fmt.Errorf("download %s: %w", name, err)
	}
	defer os.Remove(archivePath)

	switch {
	case len(artifact.Name) > 7 && artifact.Name[len(artifact.Name)-7:] == ".tar.gz":
		if err := extractTarGzBinary(archivePath, binaryPath, name); err != nil {
			return fmt.Errorf("extract %s: %w", name, err)
		}
	case len(artifact.Name) > 4 && artifact.Name[len(artifact.Name)-4:] == ".zip":
		if err := extractZipBinary(archivePath, binaryPath, binaryName); err != nil {
			return fmt.Errorf("extract %s: %w", name, err)
		}
	default:
		return fmt.Errorf("unsupported archive format %s", artifact.Name)
	}

	if err := os.Chmod(binaryPath, 0o755); err != nil { //nolint:gosec // G302: plugin must be executable
		return fmt.Errorf("chmod %s: %w", name, err)
	}

	if err := os.WriteFile(shaPath, []byte(artifact.SHA), 0o644); err != nil { //nolint:gosec // G306: trivial cache marker
		return fmt.Errorf("write sha marker for %s: %w", name, err)
	}

	return nil
}

func downloadAndVerify(rawURL, expectedSHA, name string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s returned %s", rawURL, resp.Status)
	}

	tmp, err := os.CreateTemp("", name+"-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	if expectedSHA != "" {
		actual := hex.EncodeToString(hasher.Sum(nil))
		if actual != expectedSHA {
			os.Remove(tmpPath)
			return "", fmt.Errorf("sha mismatch for %s: expected %s, got %s", name, expectedSHA, actual)
		}
	}

	return tmpPath, nil
}

func extractTarGzBinary(archivePath, destPath, expectedName string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("entry %q not found", expectedName)
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) != expectedName {
			continue
		}
		out, err := os.OpenFile(destPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, io.LimitReader(tr, pluginBinarySize)); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	}
}

func extractZipBinary(archivePath, destPath, expectedName string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, zf := range r.File {
		if filepath.Base(zf.Name) != expectedName {
			continue
		}
		zr, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(destPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			zr.Close()
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(zr, pluginBinarySize))
		zr.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}

	return fmt.Errorf("entry %q not found in zip", expectedName)
}
