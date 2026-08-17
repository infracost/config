package config_test

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/infracost/config"
)

const (
	// defaultPluginBaseURL is the CLI's default plugin release host (see the CLI's
	// pkg/plugins config.BaseURL / INFRACOST_CLI_PLUGIN_BASE_URL). The e2e tests download the
	// same "latest" plugin binaries the CLI installs, using the same URL scheme, so they always
	// exercise the plugins that ship to users.
	defaultPluginBaseURL = "https://releases.infracost.io"
	pluginCacheDir       = ".test-plugins"
	pluginBinarySize     = 1 << 30 // 1 GB cap for archive entries
)

var requiredPlugins = []string{
	"infracost-parser-terraform",
	"infracost-parser-terragrunt",
	"infracost-parser-cloudformation",
	"infracost-parser-ciscostacks",
	"infracost-parser-kubernetes",
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

// installTestPlugins downloads the latest release of each required plugin for the current OS/arch
// (skipping any already present), extracts the binary, and returns the directory the binaries live
// in. It downloads exactly the way the CLI does - from {baseURL}/{plugin}/{goos}/{goarch}/latest -
// so the e2e tests always run against the plugins that ship to users.
func installTestPlugins() (string, error) {
	dir, err := pluginInstallDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create plugin dir: %w", err)
	}

	baseURL := os.Getenv("INFRACOST_CLI_PLUGIN_BASE_URL")
	if baseURL == "" {
		baseURL = defaultPluginBaseURL
	}

	var wg sync.WaitGroup
	errs := make([]error, len(requiredPlugins))
	for i, name := range requiredPlugins {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			errs[i] = ensurePlugin(dir, baseURL, name)
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

// pluginArchiveName returns the release archive filename for the current OS, matching the CLI's
// plugin release layout (data.tar.gz everywhere except Windows).
func pluginArchiveName() string {
	if runtime.GOOS == "windows" {
		return "data.zip"
	}
	return "data.tar.gz"
}

// ensurePlugin downloads the latest release of the named plugin the same way the CLI installs
// plugins: the artifact and its checksum live at {baseURL}/{plugin}/{goos}/{goarch}/latest/. It
// caches by the published archive checksum, so it only re-downloads once "latest" moves on.
func ensurePlugin(destDir, baseURL, name string) error {
	archiveName := pluginArchiveName()
	artifactURL := fmt.Sprintf("%s/%s/%s/%s/latest/%s", strings.TrimRight(baseURL, "/"), name, runtime.GOOS, runtime.GOARCH, archiveName)

	expectedSHA, err := fetchChecksum(artifactURL + ".sha256")
	if err != nil {
		return fmt.Errorf("fetch checksum for %s: %w", name, err)
	}

	binaryName := name
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(destDir, binaryName)

	// Cache by storing the installed archive SHA alongside the binary so we can skip
	// re-downloading while "latest" hasn't moved on.
	shaPath := binaryPath + ".sha"
	if existing, err := os.ReadFile(shaPath); err == nil && string(existing) == expectedSHA {
		if _, err := os.Stat(binaryPath); err == nil {
			return nil
		}
	}

	archivePath, err := downloadAndVerify(artifactURL, expectedSHA, name)
	if err != nil {
		return fmt.Errorf("download %s: %w", name, err)
	}
	defer func() { _ = os.Remove(archivePath) }()

	switch {
	case strings.HasSuffix(archiveName, ".tar.gz"):
		if err := extractTarGzBinary(archivePath, binaryPath, name); err != nil {
			return fmt.Errorf("extract %s: %w", name, err)
		}
	case strings.HasSuffix(archiveName, ".zip"):
		if err := extractZipBinary(archivePath, binaryPath, binaryName); err != nil {
			return fmt.Errorf("extract %s: %w", name, err)
		}
	default:
		return fmt.Errorf("unsupported archive format %s", archiveName)
	}

	if err := os.Chmod(binaryPath, 0o755); err != nil { //nolint:gosec // G302: plugin must be executable
		return fmt.Errorf("chmod %s: %w", name, err)
	}

	if err := os.WriteFile(shaPath, []byte(expectedSHA), 0o644); err != nil { //nolint:gosec // G306: trivial cache marker
		return fmt.Errorf("write sha marker for %s: %w", name, err)
	}

	return nil
}

// fetchChecksum fetches a plugin's ".sha256" file and returns the hex digest (the first
// whitespace-separated field, matching the CLI's checksum parsing).
func fetchChecksum(rawURL string) (string, error) {
	resp, err := http.Get(rawURL) //nolint:gosec // G107: URL derived from the plugin base URL
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum fetch from %s returned %s", rawURL, resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}

	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty checksum response from %s", rawURL)
	}
	return fields[0], nil
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
	defer func() { _ = resp.Body.Close() }()

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
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	if expectedSHA != "" {
		actual := hex.EncodeToString(hasher.Sum(nil))
		if actual != expectedSHA {
			_ = os.Remove(tmpPath)
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
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()

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
			_ = out.Close()
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
	defer func() { _ = r.Close() }()

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
			_ = zr.Close()
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(zr, pluginBinarySize))
		_ = zr.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}

	return fmt.Errorf("entry %q not found in zip", expectedName)
}
