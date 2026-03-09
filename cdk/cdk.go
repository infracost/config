package cdk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Language represents the programming language used for a CDK project.
type Language string // nolint:revive

const (
	LanguageTypeScript Language = "typescript"
	LanguageJavaScript Language = "javascript"
	LanguagePython     Language = "python"
)

type Config struct {
	Defaults Defaults       `yaml:"defaults,omitempty"`
	Projects []*ConfigEntry `yaml:"apps,omitempty"`
}

type ConfigEntry struct {
	Context              OptionalStringMap `json:"context,omitempty" yaml:"context,omitempty"`
	Env                  OptionalStringMap `json:"env,omitempty" yaml:"env,omitempty"`
	CdkConfigPath        string            `json:"cdk_config_path" yaml:"cdk_config_path"`
	PackageManifestPaths []string          `json:"package_manifest_paths" yaml:"package_manifest_paths"`
}

// OptionalStringMap tracks whether a map was explicitly set in YAML
type OptionalStringMap struct {
	Value map[string]string
	IsSet bool
}

func (m *OptionalStringMap) UnmarshalYAML(value *yaml.Node) error {
	m.IsSet = true
	switch {
	case value.Kind == yaml.MappingNode:
		m.Value = make(map[string]string)
		for i := 0; i < len(value.Content); i += 2 {
			key := value.Content[i].Value
			val := value.Content[i+1].Value
			m.Value[key] = val
		}
	case value.Tag == "!!null":
		// Explicit null
		m.Value = nil
	default:
		// Empty map
		m.Value = make(map[string]string)
	}
	return nil
}

func (m OptionalStringMap) MarshalYAML() (any, error) {
	if !m.IsSet {
		return nil, nil
	}
	return m.Value, nil
}

// MarshalJSON ensures only the Value field is marshaled, matching MarshalYAML behavior.
func (m OptionalStringMap) MarshalJSON() ([]byte, error) {
	if !m.IsSet {
		return []byte("null"), nil
	}
	return json.Marshal(m.Value)
}

// FromMapPtr creates an OptionalStringMap from a pointer to a map.
// Used primarily in tests to create test data.
func FromMapPtr(m *map[string]string) OptionalStringMap {
	if m == nil {
		return OptionalStringMap{IsSet: false}
	}
	return OptionalStringMap{Value: *m, IsSet: true}
}

type Defaults struct {
	Context map[string]string `yaml:"context,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
}

var (
	nodeManifests   = []string{"package.json"}
	pythonManifests = []string{"pyproject.toml", "Pipfile", "setup.py", "*requirements*.txt"}

	languageToManifestPatterns = map[Language][]string{
		LanguageTypeScript: nodeManifests,
		LanguageJavaScript: nodeManifests,
		LanguagePython:     pythonManifests,
	}
)

// GenerateConfig finds all CDK configurations in the repository
func GenerateConfig(repoPath string) ([]*ConfigEntry, error) {

	cdkConfigFiles, err := findCDKConfigFiles(repoPath)
	if err != nil {
		return nil, err
	}
	if len(cdkConfigFiles) == 0 {
		return []*ConfigEntry{}, nil
	}
	cdkConfigEntries := []*ConfigEntry{}
	for _, cdkConfigFile := range cdkConfigFiles {
		lang, err := DetermineCDKLanguage(repoPath, cdkConfigFile)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			continue
		}
		manifestPatterns, ok := languageToManifestPatterns[lang]
		if !ok {
			return nil, fmt.Errorf("expected manifest pattern list not found for language %s", lang)
		}
		manifestPaths := []string{}
		for _, pattern := range manifestPatterns {
			foundPaths, err := findManifestPathsForCDKConfigFile(repoPath, cdkConfigFile, pattern)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				continue
			}
			manifestPaths = append(manifestPaths, foundPaths...)
		}
		if len(manifestPaths) == 0 {
			_, _ = fmt.Printf("no package manifest paths found for %s, skipping\n", cdkConfigFile)
			continue
		}
		cdkConfigEntries = append(cdkConfigEntries, &ConfigEntry{
			CdkConfigPath:        cdkConfigFile,
			PackageManifestPaths: manifestPaths,
		})
	}
	return cdkConfigEntries, nil
}

func findManifestPathsForCDKConfigFile(repoPath, cdkConfigFile, filePattern string) ([]string, error) {
	absCDKConfigFile := filepath.Join(repoPath, cdkConfigFile)
	dir := filepath.Dir(absCDKConfigFile)

	var paths []string
	for {
		pattern := filepath.Join(dir, filePattern)
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to glob pattern %s: %w", pattern, err)
		}
		for _, match := range matches {
			relPath, err := filepath.Rel(repoPath, match)
			if err != nil {
				return nil, fmt.Errorf("failed to get relative path: %w", err)
			}
			paths = append(paths, relPath)
		}
		if dir == repoPath || filepath.Dir(dir) == dir {
			break
		}
		dir = filepath.Dir(dir)
	}
	return paths, nil
}

func getAppFromCDKConfigFile(repoPath string, cdkConfigFile string) (string, error) {
	cdkConfigFile = filepath.Join(repoPath, cdkConfigFile)

	// #nosec G304
	cdkConfig, err := os.ReadFile(cdkConfigFile)
	if err != nil {
		return "", fmt.Errorf("failed to read cdk.json file at %q: %w", cdkConfigFile, err)
	}
	cdkConfigMap := make(map[string]any)
	err = json.Unmarshal(cdkConfig, &cdkConfigMap)
	if err != nil {
		return "", fmt.Errorf("failed to deserialize cdk.json file at %q: %w", cdkConfigFile, err)
	}
	app := cdkConfigMap["app"]
	if app == nil {
		return "", fmt.Errorf("%s does not contain an `app` field", cdkConfigFile)
	}
	appStr, ok := app.(string)
	if !ok {
		return "", fmt.Errorf("%s: app field is not a string", cdkConfigFile)
	}
	return appStr, nil
}

// DetermineCDKLanguage infers the CDK project language from the app field in the CDK configuration file.
func DetermineCDKLanguage(repoPath string, cdkConfigFile string) (Language, error) {
	app, err := getAppFromCDKConfigFile(repoPath, cdkConfigFile)
	if err != nil {
		return "", err
	}
	if app == "" {
		return "", fmt.Errorf("%s contains an invalid app", cdkConfigFile)
	}

	// Strip shell preamble (e.g. ". .venv/bin/activate; python app.py" → "python app.py")
	// Handles both ";" and "&&" separators.
	if idx := strings.LastIndex(app, "&&"); idx != -1 {
		app = strings.TrimSpace(app[idx+2:])
	} else if idx := strings.LastIndex(app, ";"); idx != -1 {
		app = strings.TrimSpace(app[idx+1:])
	}
	if app == "" {
		return "", fmt.Errorf("%s contains an invalid app", cdkConfigFile)
	}

	// check for TypeScript indicators, this is a best guess.
	if strings.Contains(app, "ts-node") || strings.Contains(app, ".ts") {
		return LanguageTypeScript, nil
	}

	// check first word for runtime, using filepath.Base to handle paths like ../.venv/bin/python
	appParts := strings.Split(app, " ")
	tool := filepath.Base(appParts[0])
	switch tool {
	case "npx", "npm", "node", "yarn":
		return LanguageJavaScript, nil
	case "python", "python3", "pip", "pip3", "pipenv", "poetry", "uv":
		return LanguagePython, nil
	default:
		return "", fmt.Errorf("%s uses unsupported tool: %s", cdkConfigFile, tool)
	}
}

func findCDKConfigFiles(repoPath string) ([]string, error) {
	var cdkConfigFiles []string
	err := filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == "cdk.json" {
			relPath, err := filepath.Rel(repoPath, path)
			if err != nil {
				return err
			}
			cdkConfigFiles = append(cdkConfigFiles, relPath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cdkConfigFiles, nil
}
