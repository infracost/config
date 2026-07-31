package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	// #nosec G505

	"github.com/infracost/config/cdk"
	"github.com/infracost/config/internal/security"
	"github.com/infracost/config/plugin"
	"gopkg.in/yaml.v3"
)

// TerraformRegexSource defines a regex-based source mapping for Terraform modules.
// Match is a regex pattern that is matched against the module source.
// Replace is the replacement pattern, which can reference capture groups using ${1}, ${2}, etc.
type TerraformRegexSource struct {
	Match   string `yaml:"match"`
	Replace string `yaml:"replace"`
}

// validate checks the config is well-formed. baseDir is the directory project
// paths are resolved against (the repo root); when it is non-empty, project
// paths are confined to it using the symlink-aware security check.
func (c *Config) validate(baseDir string) error {
	v := c.Version
	if v == "" {
		return fmt.Errorf("config file version is required")
	}

	if !slices.Contains(legacyVersions, v) && v != CurrentVersion {
		return fmt.Errorf("unsupported config file version: %s", c.Version)
	}

	for _, project := range c.Projects {
		if project.Path == "" {
			if project.Name == "" {
				return fmt.Errorf("all projects must have a path")
			}
			return fmt.Errorf("project with name %q has no path", project.Name)
		}

		if filepath.IsAbs(project.Path) {
			return fmt.Errorf("project path %q must be relative, not an absolute directory", project.Path)
		}

		// Project paths are resolved relative to the config file's directory, so
		// a path that escapes it (e.g. "../foo" or an in-repo symlink pointing
		// outside) points outside the allowed root.
		if baseDir != "" && !security.IsPathAllowed(filepath.Join(baseDir, project.Path), baseDir) {
			return fmt.Errorf("project path %q is outside the allowed directory", project.Path)
		}
	}

	return nil
}

func (c *Config) normalize() error {
	if c == nil {
		return nil
	}

	sort.Slice(c.Projects, func(i, j int) bool {
		if c.Projects[i].Name == c.Projects[j].Name {
			return c.Projects[i].Path < c.Projects[j].Path
		}
		return c.Projects[i].Name < c.Projects[j].Name
	})

	for _, project := range c.Projects {
		// set names for those without
		if project.Name == "" {
			if len(c.Projects) == 1 {
				project.Name = "main"
			} else {
				project.Name = strings.ReplaceAll(project.Path, string(filepath.Separator), "-")
			}
		}

		// inherit the following from base config if not set at project level
		if project.Terraform.Workspace == "" && c.Terraform.Defaults.Workspace != "" {
			project.Terraform.Workspace = c.Terraform.Defaults.Workspace
		}
		if project.Terraform.Cloud.Host == "" && c.Terraform.Defaults.Cloud.Host != "" {
			project.Terraform.Cloud.Host = c.Terraform.Defaults.Cloud.Host
		}
		if project.Terraform.Cloud.Org == "" && c.Terraform.Defaults.Cloud.Org != "" {
			project.Terraform.Cloud.Org = c.Terraform.Defaults.Cloud.Org
		}
		if project.Terraform.Cloud.Workspace == "" && c.Terraform.Defaults.Cloud.Workspace != "" {
			project.Terraform.Cloud.Workspace = c.Terraform.Defaults.Cloud.Workspace
		}
		if project.Terraform.Cloud.Token == "" && c.Terraform.Defaults.Cloud.Token != "" {
			project.Terraform.Cloud.Token = c.Terraform.Defaults.Cloud.Token
		}
		if project.Terraform.Spacelift.APIKey.Endpoint == "" && c.Terraform.Defaults.Spacelift.APIKey.Endpoint != "" {
			project.Terraform.Spacelift.APIKey.Endpoint = c.Terraform.Defaults.Spacelift.APIKey.Endpoint
		}
		if project.Terraform.Spacelift.APIKey.ID == "" && c.Terraform.Defaults.Spacelift.APIKey.ID != "" {
			project.Terraform.Spacelift.APIKey.ID = c.Terraform.Defaults.Spacelift.APIKey.ID
		}
		if project.Terraform.Spacelift.APIKey.Secret == "" && c.Terraform.Defaults.Spacelift.APIKey.Secret != "" {
			project.Terraform.Spacelift.APIKey.Secret = c.Terraform.Defaults.Spacelift.APIKey.Secret
		}
	}

	// fold repo-level plugin defaults (incl. the terraform.source_map compat shim) into each project's
	// blob, per-project winning.
	c.foldRepoPluginDefaults()

	// first sort by path + env to ensure duplicate name resolution uses the same path for each iteration
	sort.Slice(c.Projects, func(i, j int) bool {
		if c.Projects[i].Path == c.Projects[j].Path {
			return c.Projects[i].EnvName < c.Projects[j].EnvName
		}
		return c.Projects[i].Path < c.Projects[j].Path
	})

	// fix duplicate names
	projectNames := make(map[string]int, len(c.Projects))
	for _, project := range c.Projects {
		if num, ok := projectNames[project.Name]; ok {
			original := project.Name
			project.Name = fmt.Sprintf("%s-%d", project.Name, num+1)
			if _, ok := projectNames[project.Name]; ok {
				project.Name = project.Path
				if _, ok := projectNames[project.Name]; ok {
					return fmt.Errorf("failed to generate infracost config: duplicate project name could not automatically resolved: %s", project.Name)
				}
			}
			// ...and increment the original name
			projectNames[original]++
		}
		projectNames[project.Name] = 0
	}

	// order projects alphabetically by name as these are now guaranteed unique
	sort.Slice(c.Projects, func(i, j int) bool {
		return c.Projects[i].Name < c.Projects[j].Name
	})

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// LoadOption configures LoadConfigFile.
type LoadOption func(*loadOptions)

type loadOptions struct {
	identifier       *plugin.Identifier
	pluginDir        string
	defaultPluginDir bool
}

// WithLoadIdentifier supplies an existing plugin identifier for type backfill. The caller retains
// ownership of its lifecycle - LoadConfigFile will not close it. Prefer this over WithLoadPluginDir
// when a plugin identifier already exists (e.g. one the runner created for generation), to avoid
// re-spawning plugin processes.
func WithLoadIdentifier(identifier *plugin.Identifier) LoadOption {
	return func(o *loadOptions) { o.identifier = identifier }
}

// WithLoadPluginDir makes LoadConfigFile create (and close) a plugin identifier from the given
// directory so that plugin-provided project types (e.g. kubernetes) can be backfilled. Ignored when
// WithLoadIdentifier is also supplied.
func WithLoadPluginDir(dir string) LoadOption {
	return func(o *loadOptions) { o.pluginDir = dir }
}

// WithLoadDefaultPluginDir is like WithLoadPluginDir but uses the default plugin directory.
func WithLoadDefaultPluginDir(useDefault bool) LoadOption {
	return func(o *loadOptions) { o.defaultPluginDir = useDefault }
}

func LoadConfigFile(
	ctx context.Context,
	path, repoPath string,
	opts ...LoadOption,
) (*Config, error) {
	if !fileExists(path) {
		return nil, fmt.Errorf("config file does not exist at %s", path)
	}

	var loadOpts loadOptions
	for _, opt := range opts {
		opt(&loadOpts)
	}

	// Resolve a plugin identifier so plugin-provided types (e.g. kubernetes) can be backfilled. Prefer
	// a caller-supplied identifier; otherwise create one from the configured plugin dir and close it
	// when we're done.
	identifier := loadOpts.identifier
	if identifier == nil && (loadOpts.defaultPluginDir || loadOpts.pluginDir != "") {
		created, err := plugin.CreateIdentifier(ctx, loadOpts.pluginDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create plugin identifier: %w", err)
		}
		defer created.Close()
		identifier = created
	}

	// #nosec G304
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
	}

	cfg, err := parseConfigFile(content, nil, repoPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
	}

	// A hand-written config whose project entries omit `type:` would otherwise default to terraform
	// downstream, breaking non-terraform projects. There is no autodetect context here, so resolve
	// each typeless project by sniffing its path (using plugins when available). See FIX-495.
	backfillProjectTypes(ctx, repoPath, cfg, nil, identifier, false)

	if len(cfg.CDK.Projects) > 0 || len(cfg.CDK.Defaults.Context) > 0 || len(cfg.CDK.Defaults.Env) > 0 {
		if err := finalizeCDKConfig(repoPath, cfg, false, false); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
		}
	}

	return cfg, nil
}

func isYAMLEmpty(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			return false
		}
	}
	return true
}

func readConfigVersion(raw []byte) (string, error) {
	var base ConfigBase
	if err := yaml.Unmarshal(raw, &base); err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
	}
	if base.Version == "" {
		return "0.2", nil
	}
	return base.Version, nil
}

func parseConfigFile(content []byte, target *Config, baseDir string) (*Config, error) {
	if target == nil {
		target = defaultConfig()
	}

	if !isYAMLEmpty(string(content)) {

		version, err := readConfigVersion(content)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
		}

		switch {
		case slices.Contains(legacyVersions, version):
			if err := parseLegacyVersion(content, target); err != nil {
				return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
			}
		case version == CurrentVersion:
			if err := parseCurrentVersion(content, target); err != nil {
				return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
			}
		default:
			return nil, fmt.Errorf("%w: unsupported config file version: %s", ErrInvalidConfigYAML, version)
		}
	}

	if err := target.normalize(); err != nil {
		return nil, err
	}

	if err := target.validate(baseDir); err != nil {
		return nil, err
	}

	return target, nil
}

func parseWithAutodetectAllowed(content []byte, target *Config, baseDir string) (*Config, error) {
	if target == nil {
		target = defaultConfig()
	}

	if !isYAMLEmpty(string(content)) {

		version, err := readConfigVersion(content)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
		}

		switch {
		case slices.Contains(legacyVersions, version):
			if err := parseLegacyVersion(content, target); err != nil {
				return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
			}
		case version == CurrentVersion:
			// Config.UnmarshalYAML routes through the configfile layer, which also captures the
			// autodetect: node (ignored here; the autodetect config is parsed separately from the raw
			// bytes), so no dedicated wrapper is needed to tolerate it.
			if err := parseCurrentVersion(content, target); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%w: unsupported config file version: %s", ErrInvalidConfigYAML, version)
		}
	}

	if err := target.normalize(); err != nil {
		return nil, err
	}

	if err := target.validate(baseDir); err != nil {
		return nil, err
	}

	return target, nil
}

func parseCurrentVersion(content []byte, config *Config) error {
	// yaml.Unmarshal invokes Config.UnmarshalYAML, which decodes via the configfile layer and applies
	// the typed/blob split. config keeps any defaults it was seeded with for keys the file omits.
	if err := yaml.Unmarshal(content, config); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidConfigYAML, simplifyYAMLError(err))
	}

	return nil
}

// simplifyYAMLError takes a yaml error and simplifies it to be more user-friendly: it only shows the _first_ error,
// and it removes go types from the error
func simplifyYAMLError(err error) string {
	if err == nil {
		return ""
	}

	var yamlTypeErr *yaml.TypeError
	if errors.As(err, &yamlTypeErr) {
		// for type errors, we can simplify each line and remove the "in type" suffix which is not helpful to users
		if len(yamlTypeErr.Errors) == 0 {
			return "unknown YAML type error"
		}
		return simplifyErrorLine(yamlTypeErr.Errors[0])
	}

	return err.Error()
}

func simplifyErrorLine(line string) string {
	line = strings.TrimSpace(line)
	simple, _, _ := strings.Cut(line, " in type")
	return simple
}

func finalizeCDKConfig(repoPath string, cfg *Config, ignorePermissionErrors, ignoreHiddenDirs bool) error {
	// If no CDK entries and no defaults, nothing to do
	if len(cfg.CDK.Projects) == 0 && len(cfg.CDK.Defaults.Context) == 0 && len(cfg.CDK.Defaults.Env) == 0 {
		return nil
	}

	mergedEntries, err := mergeCDKEntriesWithAutodetect(repoPath, cfg.CDK.Projects, cfg.CDK.Defaults, ignorePermissionErrors, ignoreHiddenDirs)
	if err != nil {
		return err
	}
	cfg.CDK.Projects = mergedEntries
	return nil
}

func mergeCDKEntriesWithAutodetect(rootPath string, entries []*cdk.ConfigEntry, defaults cdk.Defaults, ignorePermissionErrors, ignoreHiddenDirs bool) ([]*cdk.ConfigEntry, error) {
	if len(entries) == 0 {
		// If no entries, check if we should autodetect
		detectedEntries, err := cdk.GenerateConfig(rootPath, ignorePermissionErrors, ignoreHiddenDirs)
		if err != nil {
			return nil, err
		}
		result := make([]*cdk.ConfigEntry, 0, len(detectedEntries))
		for _, detected := range detectedEntries {
			merged := applyCDKDefaults(cloneCDKEntry(detected), defaults)
			result = append(result, merged)
		}
		return result, nil
	}

	var needsAutodetect bool
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		// Only trigger autodetect if CdkConfigPath or PackageManifestPaths are missing
		if entry.CdkConfigPath == "" || len(entry.PackageManifestPaths) == 0 {
			needsAutodetect = true
			break
		}
	}
	if !needsAutodetect {
		// Apply defaults even when autodetect is not needed
		result := make([]*cdk.ConfigEntry, 0, len(entries))
		for _, entry := range entries {
			if entry != nil {
				result = append(result, applyCDKDefaults(cloneCDKEntry(entry), defaults))
			}
		}
		return result, nil
	}

	detectedEntries, err := cdk.GenerateConfig(rootPath, ignorePermissionErrors, ignoreHiddenDirs)
	if err != nil {
		return nil, err
	}

	detectedByPath := make(map[string]*cdk.ConfigEntry, len(detectedEntries))
	for _, detected := range detectedEntries {
		detectedByPath[detected.CdkConfigPath] = detected
	}

	var overlays []*cdk.ConfigEntry
	scoped := make([]*cdk.ConfigEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.CdkConfigPath == "" {
			overlays = append(overlays, entry)
			continue
		}
		scoped = append(scoped, entry)
	}

	// Merge user-specified entries with detected entries, then apply defaults
	merged := make([]*cdk.ConfigEntry, 0, len(scoped))
	for _, entry := range scoped {
		detected := detectedByPath[entry.CdkConfigPath]
		mergedEntry := mergeCDKEntry(entry, detected)
		merged = append(merged, applyCDKDefaults(mergedEntry, defaults))
	}

	// If no user-specified entries, use all detected entries with defaults applied
	if len(scoped) == 0 {
		for _, detected := range detectedEntries {
			if detected == nil {
				continue
			}
			merged = append(merged, applyCDKDefaults(cloneCDKEntry(detected), defaults))
		}
	}

	if len(merged) == 0 {
		return merged, nil
	}

	for _, overlay := range overlays {
		applyCDKOverlay(merged, overlay)
	}

	return merged, nil
}

func mergeCDKEntry(base, detected *cdk.ConfigEntry) *cdk.ConfigEntry {
	if base == nil {
		return cloneCDKEntry(detected)
	}

	result := &cdk.ConfigEntry{
		Context: base.Context,
		Env:     base.Env,
	}

	// If base doesn't have Context/Env set, use detected values (from autodetect)
	if !result.Context.IsSet && detected != nil && detected.Context.IsSet {
		result.Context = detected.Context
	}
	if !result.Env.IsSet && detected != nil && detected.Env.IsSet {
		result.Env = detected.Env
	}

	if base.CdkConfigPath != "" {
		result.CdkConfigPath = base.CdkConfigPath
	} else if detected != nil {
		result.CdkConfigPath = detected.CdkConfigPath
	}

	if len(base.PackageManifestPaths) > 0 {
		result.PackageManifestPaths = copyStringSlice(base.PackageManifestPaths)
	} else if detected != nil {
		result.PackageManifestPaths = copyStringSlice(detected.PackageManifestPaths)
	}

	if result.CdkConfigPath == "" && detected != nil {
		result.CdkConfigPath = detected.CdkConfigPath
	}

	return result
}

// applyCDKDefaults applies defaults to an entry if Context/Env are not explicitly set.
// If Context/Env are explicitly set (even if empty), they are preserved as-is.
// This allows repos to opt out of defaults by specifying an empty map: context: {}
func applyCDKDefaults(entry *cdk.ConfigEntry, defaults cdk.Defaults) *cdk.ConfigEntry {
	if entry == nil {
		return nil
	}

	result := cloneCDKEntry(entry)

	// Apply context defaults only if not explicitly set in the entry
	if !result.Context.IsSet && len(defaults.Context) > 0 {
		result.Context = cdk.OptionalStringMap{
			Value: copyStringMap(defaults.Context),
			IsSet: true,
		}
	}

	// Apply env defaults only if not explicitly set in the entry
	if !result.Env.IsSet && len(defaults.Env) > 0 {
		result.Env = cdk.OptionalStringMap{
			Value: copyStringMap(defaults.Env),
			IsSet: true,
		}
	}

	return result
}

func cloneCDKEntry(entry *cdk.ConfigEntry) *cdk.ConfigEntry {
	if entry == nil {
		return nil
	}
	result := &cdk.ConfigEntry{
		CdkConfigPath:        entry.CdkConfigPath,
		PackageManifestPaths: copyStringSlice(entry.PackageManifestPaths),
	}
	// Deep copy the maps to avoid sharing references
	if entry.Context.IsSet {
		result.Context = cdk.OptionalStringMap{
			Value: copyStringMap(entry.Context.Value),
			IsSet: true,
		}
	}
	if entry.Env.IsSet {
		result.Env = cdk.OptionalStringMap{
			Value: copyStringMap(entry.Env.Value),
			IsSet: true,
		}
	}
	return result
}

func copyStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func copyStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func applyCDKOverlay(entries []*cdk.ConfigEntry, overlay *cdk.ConfigEntry) {
	for _, entry := range entries {
		if entry == nil || overlay == nil {
			continue
		}
		// Apply overlay Context only if entry doesn't have it set
		if !entry.Context.IsSet && overlay.Context.IsSet && len(overlay.Context.Value) > 0 {
			entry.Context = overlay.Context
		}
		if len(entry.PackageManifestPaths) == 0 && len(overlay.PackageManifestPaths) > 0 {
			entry.PackageManifestPaths = copyStringSlice(overlay.PackageManifestPaths)
		}
		// Apply overlay Env only if entry doesn't have it set
		if !entry.Env.IsSet && overlay.Env.IsSet && len(overlay.Env.Value) > 0 {
			entry.Env = overlay.Env
		}
	}
}
