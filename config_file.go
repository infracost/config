package config

import (
	"bytes"
	"errors"
	"slices"

	// #nosec G505

	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/infracost/config/cdk"
	"github.com/infracost/config/internal/security"
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

		// Merge repo-level plugin defaults (c.Plugins) into the project's matching plugin blob.
		// This is a generic, schema-agnostic deep merge over the raw YAML types: nested maps
		// merge recursively, and every other value (scalars and lists alike) is atomic with the
		// per-project value winning over the global default. Config never needs to understand the shape.
		for pluginName, globalOpts := range c.Plugins {
			if len(globalOpts) == 0 {
				continue
			}
			merged := mergePluginBlobs(globalOpts, project.Plugins[pluginName])
			if len(merged) > 0 {
				if project.Plugins == nil {
					project.Plugins = make(map[string]map[string]any, len(c.Plugins))
				}
				project.Plugins[pluginName] = merged
			}
		}

		// Fold the deprecated structured terraform.* / aws.* fields into the plugins blob so that
		// consumers can read only the new plugins.<name> style. Structured values win over the
		// blob (they were either there before or added by the user). See config_plugins_compat.go.
		project.foldDeprecatedFieldsIntoPlugins(c.Terraform.SourceMap)
	}

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

// mergePluginBlobs deep-merges a repo-level plugin defaults map (global) with a per-project plugin
// map (project) and returns the result. It is schema-agnostic: nested maps merge recursively, and
// every other value (scalars and lists alike) is atomic with the per-project value winning.
// Neither input is mutated.
func mergePluginBlobs(global, project map[string]any) map[string]any {
	merged := deepMergeAny(global, project)
	if m, ok := merged.(map[string]any); ok {
		return m
	}
	return nil
}

// deepMergeAny merges project into global following the plugin-merge convention (see
// mergePluginBlobs). Only maps merge recursively; every other value - scalars AND lists - is
// treated atomically, with the per-project value winning when it is set.
func deepMergeAny(global, project any) any {
	gm, gok := global.(map[string]any)
	pm, pok := project.(map[string]any)
	if gok && pok {
		out := make(map[string]any, len(gm)+len(pm))
		for k, gv := range gm {
			// deep-copy global branches. global is the shared repo-level defaults map reused for
			// every project, so aliasing its nested maps/slices into a project's result would let
			// a later mutation of one project's blob corrupt the global and every sibling project.
			out[k] = deepCopyAny(gv)
		}
		for k, pv := range pm {
			if gv, ok := gm[k]; ok {
				out[k] = deepMergeAny(gv, pv)
			} else {
				out[k] = pv
			}
		}
		return out
	}

	// Non-map values (scalars and lists) are atomic: the per-project value wins, unless the
	// project didn't set anything, in which case fall back to a copy of the global default. Lists
	// are deliberately NOT concatenated - a project's list replaces the global one.
	if project == nil {
		return deepCopyAny(global)
	}
	return project
}

// deepCopyAny returns a deep copy of a decoded YAML/JSON value (nested maps, slices, scalars) so
// that a merged result never aliases the shared global-defaults map.
func deepCopyAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = deepCopyAny(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = deepCopyAny(val)
		}
		return out
	default:
		return v
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func LoadConfigFile(
	path, repoPath string,
	envVars map[string]string,
) (*Config, error) {

	if !fileExists(path) {
		return nil, fmt.Errorf("config file does not exist at %s", path)
	}

	// #nosec G304
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
	}

	cfg, err := parseConfigFile(content, envVars, nil, repoPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
	}

	if len(cfg.CDK.Projects) > 0 || len(cfg.CDK.Defaults.Context) > 0 || len(cfg.CDK.Defaults.Env) > 0 {
		if err := finalizeCDKConfig(repoPath, cfg, false, false); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
		}
	}

	return cfg, nil
}

func (c *Config) replaceEnvVars(envVars map[string]string) error {
	if c == nil || envVars == nil {
		return nil
	}
	content, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	content = []byte(os.Expand(string(content), func(key string) string {
		if val, ok := envVars[key]; ok {
			return val
		}
		return fmt.Sprintf("${%s}", key)
	}))
	if err := yaml.Unmarshal(content, c); err != nil {
		return fmt.Errorf("failed to unmarshal config after env var replacement: %w", err)
	}
	return nil
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

func parseConfigFile(content []byte, envVars map[string]string, target *Config, baseDir string) (*Config, error) {

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

		if err := target.replaceEnvVars(envVars); err != nil {
			return nil, fmt.Errorf("%w: failed to replace env vars: %s", ErrInvalidConfigYAML, err)
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
			config := ConfigWithAutodetect{
				Config: target,
			}

			decoder := yaml.NewDecoder(bytes.NewReader(content))
			decoder.KnownFields(true)

			if err := decoder.Decode(&config); err != nil {
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

func parseCurrentVersion(content []byte, config *Config) error {

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	if err := decoder.Decode(&config); err != nil {
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

func parseLegacyVersion(content []byte, config *Config) error {

	var intermediary ConfigWithLegacySupport
	if len(config.Projects) > 0 {
		// if the config came with projectsd set (e.g. default main) we need to set it here to see if it gets overridden by legacy projects
		intermediary.Projects = make([]*ProjectWithLegacySupport, 0, len(config.Projects))
		for _, project := range config.Projects {
			intermediary.Projects = append(intermediary.Projects, &ProjectWithLegacySupport{
				Project: *project,
			})
		}
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	if err := decoder.Decode(&intermediary); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidConfigYAML, simplifyYAMLError(err))
	}

	// copy across fields that exist in both
	config.ConfigBase = intermediary.ConfigBase
	config.Version = CurrentVersion // force version to latest after converting
	config.Currency = intermediary.Currency
	config.UsageFilePath = intermediary.UsageFilePath
	config.CDK.Projects = intermediary.CDK
	config.CDK.Defaults = intermediary.CDKDefaults

	// copy legacy fields to their new locations
	config.Terraform.SourceMap = intermediary.TerraformRegexSourceMap
	config.Terraform.Defaults.Cloud.Host = intermediary.TerraformCloudHost
	config.Terraform.Defaults.Cloud.Org = intermediary.TerraformCloudOrg
	config.Terraform.Defaults.Cloud.Workspace = intermediary.TerraformCloudWorkspace
	config.Terraform.Defaults.Cloud.Token = intermediary.TerraformCloudToken
	config.Terraform.Defaults.Spacelift.APIKey.Endpoint = intermediary.SpaceliftAPIKeyEndpoint
	config.Terraform.Defaults.Spacelift.APIKey.ID = intermediary.SpaceliftAPIKeyID
	config.Terraform.Defaults.Spacelift.APIKey.Secret = intermediary.SpaceliftAPIKeySecret
	config.Terraform.Defaults.Workspace = intermediary.TerraformWorkspace

	// remove default projects and take whatever the decode gace us - if the user didn't specify the projects key, we'll get the defaults preserved anyway
	config.Projects = nil

	// convert legacy projects to new ones
	// this is deliberately a nil check rather than checking length, as we want to preserve an empty projects section if it was explicitly set to empty in the legacy config
	if len(intermediary.Projects) > 0 {
		config.Projects = make([]*Project, 0, len(intermediary.Projects))

		for _, legacyProject := range intermediary.Projects {
			project := legacyProject.Project
			if len(legacyProject.TerraformVars) > 0 {
				if project.Terraform.Vars == nil {
					project.Terraform.Vars = make(map[string]any)
				}
				for k, v := range legacyProject.TerraformVars {
					project.Terraform.Vars[k] = v
				}
			}
			if legacyProject.TerraformWorkspace != "" {
				project.Terraform.Workspace = legacyProject.TerraformWorkspace
			}
			if legacyProject.TerraformCloudHost != "" {
				project.Terraform.Cloud.Host = legacyProject.TerraformCloudHost
			}
			if legacyProject.TerraformCloudOrg != "" {
				project.Terraform.Cloud.Org = legacyProject.TerraformCloudOrg
			}
			if legacyProject.TerraformCloudWorkspace != "" {
				project.Terraform.Cloud.Workspace = legacyProject.TerraformCloudWorkspace
			}
			if legacyProject.TerraformCloudToken != "" {
				project.Terraform.Cloud.Token = legacyProject.TerraformCloudToken
			}
			if legacyProject.SpaceliftAPIKeyEndpoint != "" {
				project.Terraform.Spacelift.APIKey.Endpoint = legacyProject.SpaceliftAPIKeyEndpoint
			}
			if legacyProject.SpaceliftAPIKeyID != "" {
				project.Terraform.Spacelift.APIKey.ID = legacyProject.SpaceliftAPIKeyID
			}
			if legacyProject.SpaceliftAPIKeySecret != "" {
				project.Terraform.Spacelift.APIKey.Secret = legacyProject.SpaceliftAPIKeySecret
			}
			if len(legacyProject.TerraformVarFiles) > 0 {
				project.Terraform.VarFiles = legacyProject.TerraformVarFiles
			}
			if legacyProject.ProjectType != "" {
				project.Type = legacyProject.ProjectType
			}
			config.Projects = append(config.Projects, &project)
		}
	}

	return nil
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
