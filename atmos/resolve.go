// Package atmos resolves Cloud Posse Atmos stack configuration (import merging,
// inheritance, per-component vars/providers) without the cloudposse/atmos dependency
// and without evaluating templates or YAML functions. It is the source for Atmos
// project detection in the config module; the parser plugin keeps a copy for parsing
// (kept in sync, both guarded by the same golden tests against real Atmos).
package atmos

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar"
	"gopkg.in/yaml.v3"
)

// Atmos manifest section keys.
const (
	keyImport     = "import"
	keyVars       = "vars"
	keyProviders  = "providers"
	keyComponents = "components"
	keyTerraform  = "terraform"
	keyComponentS = "component" // metadata.component / base-component reference
	keyMetadata   = "metadata"
)

// ComponentConfig is the resolved Atmos configuration for a single (stack, component).
type ComponentConfig struct {
	Stack     string
	Component string
	// FinalComponent is the Terraform component directory (after metadata.component
	// inheritance); the source lives at <TerraformBasePath>/<FinalComponent>.
	FinalComponent string
	Vars           map[string]any
	Providers      map[string]any
	// SourceDir is the absolute path to the Terraform component directory.
	SourceDir string
	// StackFile is the absolute path to the stack manifest defining this stack,
	// used to map generated files back to their originating source location.
	StackFile string
}

// Config is the subset of atmos.yaml the resolver needs. All paths are absolute.
type Config struct {
	BasePath          string
	StacksBasePath    string
	TerraformBasePath string
	IncludedPaths     []string
	ExcludedPaths     []string
	NamePattern       string
	NameTemplate      string
}

type atmosYAML struct {
	BasePath   string `yaml:"base_path"`
	Components struct {
		Terraform struct {
			BasePath string `yaml:"base_path"`
		} `yaml:"terraform"`
	} `yaml:"components"`
	Stacks struct {
		BasePath      string   `yaml:"base_path"`
		IncludedPaths []string `yaml:"included_paths"`
		ExcludedPaths []string `yaml:"excluded_paths"`
		NamePattern   string   `yaml:"name_pattern"`
		NameTemplate  string   `yaml:"name_template"`
	} `yaml:"stacks"`
}

// LoadConfig reads and resolves atmos.yaml under repoBasePath.
func LoadConfig(repoBasePath string) (*Config, error) {
	raw, err := os.ReadFile(filepath.Join(repoBasePath, "atmos.yaml")) // #nosec G304 - path is a trusted repo root
	if err != nil {
		return nil, fmt.Errorf("read atmos.yaml: %w", err)
	}
	var y atmosYAML
	if err := yaml.Unmarshal(raw, &y); err != nil {
		return nil, fmt.Errorf("parse atmos.yaml: %w", err)
	}

	base := repoBasePath
	if y.BasePath != "" && y.BasePath != "." {
		base = filepath.Join(repoBasePath, y.BasePath)
	}
	stacksBase := y.Stacks.BasePath
	if stacksBase == "" {
		stacksBase = "stacks"
	}
	tfBase := y.Components.Terraform.BasePath
	if tfBase == "" {
		tfBase = filepath.Join("components", "terraform")
	}

	return &Config{
		BasePath:          base,
		StacksBasePath:    filepath.Join(base, stacksBase),
		TerraformBasePath: filepath.Join(base, tfBase),
		IncludedPaths:     y.Stacks.IncludedPaths,
		ExcludedPaths:     y.Stacks.ExcludedPaths,
		NamePattern:       y.Stacks.NamePattern,
		NameTemplate:      y.Stacks.NameTemplate,
	}, nil
}

// Describe resolves a single (stack, component) without evaluating Go templates or
// Atmos YAML functions, mirroring Atmos describe with both processing toggles off.
func Describe(repoBasePath, stack, component string) (*ComponentConfig, error) {
	cfg, err := LoadConfig(repoBasePath)
	if err != nil {
		return nil, err
	}
	files, err := cfg.discoverStackFiles()
	if err != nil {
		return nil, err
	}
	for _, rel := range files {
		merged, err := cfg.resolveManifest(rel, map[string]bool{})
		if err != nil {
			return nil, err
		}
		name, err := cfg.stackName(merged)
		if err != nil {
			return nil, err
		}
		if name != stack {
			continue
		}
		out, err := cfg.assembleComponent(stack, component, merged)
		if err != nil {
			return nil, err
		}
		out.SourceDir = filepath.Join(cfg.TerraformBasePath, out.FinalComponent)
		if abs, err := cfg.manifestPath(rel); err == nil {
			out.StackFile = abs
		}
		return out, nil
	}
	return nil, fmt.Errorf("stack %q not found", stack)
}

// StackComponent identifies one deployable Atmos (stack, component) pair.
type StackComponent struct {
	Stack     string
	Component string
	// FinalComponent is the Terraform component directory (after metadata.component).
	FinalComponent string
}

// Enumerate returns every deployable (stack, component) pair in the repo: each
// discovered stack's name paired with its enabled, non-abstract Terraform components.
// This is the basis for config-level project detection. Templates and YAML functions
// are never evaluated. Results are sorted for determinism.
func Enumerate(repoBasePath string) ([]StackComponent, error) {
	cfg, err := LoadConfig(repoBasePath)
	if err != nil {
		return nil, err
	}
	files, err := cfg.discoverStackFiles()
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var out []StackComponent
	for _, rel := range files {
		merged, err := cfg.resolveManifest(rel, map[string]bool{})
		if err != nil {
			return nil, err
		}
		name, err := cfg.stackName(merged)
		if err != nil || name == "" {
			continue
		}
		tfComponents := asMap(asMap(merged[keyComponents])[keyTerraform])
		for comp := range tfComponents {
			section := asMap(tfComponents[comp])
			if isAbstract(section) {
				continue
			}
			vars, _, err := resolveComponentConfig(tfComponents, comp, map[string]bool{})
			if err != nil {
				return nil, err
			}
			if !isEnabled(vars) {
				continue
			}
			key := name + "\x00" + comp
			if seen[key] {
				continue
			}
			seen[key] = true
			final := comp
			if mc, ok := asMap(section[keyMetadata])[keyComponentS].(string); ok && mc != "" {
				final = mc
			}
			out = append(out, StackComponent{Stack: name, Component: comp, FinalComponent: final})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Stack != out[j].Stack {
			return out[i].Stack < out[j].Stack
		}
		return out[i].Component < out[j].Component
	})
	return out, nil
}

// isAbstract reports whether a component is an abstract base (metadata.type: abstract),
// which is never deployed on its own.
func isAbstract(section map[string]any) bool {
	t, _ := asMap(section[keyMetadata])["type"].(string)
	return t == "abstract"
}

// isEnabled reports whether a component's resolved vars leave it enabled (the Atmos
// convention vars.enabled: false disables a component in a stack).
func isEnabled(vars map[string]any) bool {
	if v, ok := vars["enabled"].(bool); ok {
		return v
	}
	return true
}

// discoverStackFiles returns stack manifest paths (relative to StacksBasePath, without
// extension) that match included_paths and are not excluded.
func (c *Config) discoverStackFiles() ([]string, error) {
	var out []string
	err := filepath.WalkDir(c.StacksBasePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		rel, err := filepath.Rel(c.StacksBasePath, path)
		if err != nil {
			return err
		}
		relNoExt := strings.TrimSuffix(rel, ext)
		if !c.pathIncluded(rel, relNoExt) {
			return nil
		}
		out = append(out, relNoExt)
		return nil
	})
	return out, err
}

// pathIncluded reports whether a manifest is selected by stacks.included_paths and
// not removed by excluded_paths. Atmos patterns are file-path globs that may or may
// not include the extension, so both the filename and its extensionless form are tried.
func (c *Config) pathIncluded(relWithExt, relNoExt string) bool {
	matchAny := func(pats []string) bool {
		for _, pat := range pats {
			if matchGlob(pat, relWithExt) || matchGlob(pat, relNoExt) {
				return true
			}
		}
		return false
	}
	if len(c.IncludedPaths) > 0 && !matchAny(c.IncludedPaths) {
		return false
	}
	return !matchAny(c.ExcludedPaths)
}

func matchGlob(pattern, name string) bool {
	ok, err := doublestar.Match(pattern, name)
	return err == nil && ok
}

// resolveManifest loads a manifest (relative to StacksBasePath, with or without
// extension), recursively resolves its local imports, and deep-merges them in order
// with the manifest's own content taking precedence. Remote imports are rejected.
func (c *Config) resolveManifest(rel string, chain map[string]bool) (map[string]any, error) {
	abs, err := c.manifestPath(rel)
	if err != nil {
		return nil, err
	}
	if chain[abs] {
		return nil, fmt.Errorf("import cycle detected at %q", rel)
	}
	chain[abs] = true
	defer delete(chain, abs)

	raw, err := loadManifest(abs)
	if err != nil {
		return nil, err
	}

	merged := map[string]any{}
	for _, imp := range importList(raw[keyImport]) {
		if isRemoteImport(imp) {
			return nil, fmt.Errorf("remote import %q is not supported", imp)
		}
		sub, err := c.resolveManifest(imp, chain)
		if err != nil {
			return nil, err
		}
		merged = deepMerge(merged, sub)
	}
	delete(raw, keyImport)
	return deepMerge(merged, raw), nil
}

func (c *Config) manifestPath(rel string) (string, error) {
	var candidate string
	if filepath.Ext(rel) != "" {
		candidate = filepath.Join(c.StacksBasePath, rel)
	} else {
		for _, ext := range []string{".yaml", ".yml"} {
			p := filepath.Join(c.StacksBasePath, rel+ext)
			if _, err := os.Stat(p); err == nil {
				candidate = p
				break
			}
		}
		if candidate == "" {
			return "", fmt.Errorf("manifest %q not found under %s", rel, c.StacksBasePath)
		}
	}
	// Jail imports to the stacks tree: a manifest must never resolve (after symlink
	// resolution) outside StacksBasePath, so a malicious "../" import can't read
	// arbitrary files.
	if !pathWithin(candidate, c.StacksBasePath) {
		return "", fmt.Errorf("import %q resolves outside the stacks directory", rel)
	}
	return candidate, nil
}

// pathWithin reports whether path resolves (after symlink resolution) inside parent.
func pathWithin(path, parent string) bool {
	resolve := func(p string) string {
		abs, err := filepath.Abs(p)
		if err != nil {
			return ""
		}
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			return r
		}
		return abs
	}
	rp, rParent := resolve(path), resolve(parent)
	if rp == "" || rParent == "" {
		return false
	}
	return rp == rParent || strings.HasPrefix(rp+string(filepath.Separator), rParent+string(filepath.Separator))
}

func loadManifest(abs string) (map[string]any, error) {
	raw, err := os.ReadFile(abs) // #nosec G304 - path is jailed under StacksBasePath
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", abs, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", abs, err)
	}
	v, err := nodeToValue(&root)
	if err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", abs, err)
	}
	out, _ := v.(map[string]any)
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// nodeToValue converts a YAML node tree to Go values, preserving Atmos YAML
// function tags (e.g. `!terraform.output vpc id`) as literal "!tag value" strings.
// A plain yaml.Unmarshal into map[string]any would discard the custom tag and keep
// only the scalar, so deferred YAML functions would look like concrete values and
// escape synthetic-pruning. This mirrors how Atmos preserves them when YAML-function
// processing is disabled.
func nodeToValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return map[string]any{}, nil
		}
		return nodeToValue(n.Content[0])
	case yaml.MappingNode:
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			val, err := nodeToValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			out[key] = val
		}
		return out, nil
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			val, err := nodeToValue(c)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
		}
		return out, nil
	case yaml.ScalarNode:
		// Custom (Atmos function) tags look like "!terraform.output"; standard
		// resolved tags look like "!!str"/"!!int"/etc. Preserve the former verbatim.
		if strings.HasPrefix(n.Tag, "!") && !strings.HasPrefix(n.Tag, "!!") {
			if n.Value == "" {
				return n.Tag, nil
			}
			return n.Tag + " " + n.Value, nil
		}
		var v any
		if err := n.Decode(&v); err != nil {
			return nil, err
		}
		return v, nil
	case yaml.AliasNode:
		return nodeToValue(n.Alias)
	default:
		return nil, nil
	}
}

func importList(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, it := range items {
		switch t := it.(type) {
		case string:
			out = append(out, t)
		case map[string]any:
			// import objects: {path: "...", context: {...}}; v1 honors the path only.
			if p, ok := t["path"].(string); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

// isRemoteImport reports whether an import path is a remote URL (go-getter style).
// v1 rejects these: Atmos resolves them via a process-global importer that the
// processing toggles do not gate, so they cannot be safely sandboxed.
func isRemoteImport(imp string) bool {
	return strings.Contains(imp, "://") || strings.Contains(imp, "::")
}

// stackName derives the stack name from name_pattern using the merged top-level vars.
func (c *Config) stackName(merged map[string]any) (string, error) {
	if c.NamePattern == "" {
		return "", fmt.Errorf("stacks.name_pattern is required (name_template is not yet supported)")
	}
	vars := asMap(merged[keyVars])
	name := c.NamePattern
	for _, tok := range []string{"namespace", "tenant", "environment", "stage"} {
		val, _ := vars[tok].(string)
		name = strings.ReplaceAll(name, "{"+tok+"}", val)
	}
	return name, nil
}

// assembleComponent produces the final inputs for a component, mirroring Atmos
// precedence: global -> terraform-level -> (inherited base components) -> component.
// Base-component config is inherited via metadata.inherits (ordered, later wins,
// resolved recursively); metadata.component only selects the Terraform source
// directory (FinalComponent) and does not contribute config.
func (c *Config) assembleComponent(stack, component string, merged map[string]any) (*ComponentConfig, error) {
	tf := asMap(merged[keyTerraform])
	tfComponents := asMap(asMap(merged[keyComponents])[keyTerraform])
	compSection := asMap(tfComponents[component])
	if compSection == nil {
		return nil, fmt.Errorf("component %q not found in stack %q", component, stack)
	}

	compVars, compProviders, err := resolveComponentConfig(tfComponents, component, map[string]bool{})
	if err != nil {
		return nil, fmt.Errorf("stack %q: %w", stack, err)
	}

	finalVars := deepMerge(deepMerge(asMap(merged[keyVars]), asMap(tf[keyVars])), compVars)
	finalProviders := deepMerge(deepMerge(asMap(merged[keyProviders]), asMap(tf[keyProviders])), compProviders)

	final := component
	if mc, ok := asMap(compSection[keyMetadata])[keyComponentS].(string); ok && mc != "" {
		final = mc
	}

	return &ComponentConfig{
		Stack:          stack,
		Component:      component,
		FinalComponent: final,
		Vars:           finalVars,
		Providers:      finalProviders,
	}, nil
}

// resolveComponentConfig returns a component's vars and providers including config
// inherited via metadata.inherits. Bases are merged in list order (later overrides
// earlier) and resolved recursively, then the component's own config is applied on
// top. visited guards against inheritance cycles.
func resolveComponentConfig(tfComponents map[string]any, component string, visited map[string]bool) (vars, providers map[string]any, err error) {
	section := asMap(tfComponents[component])
	if section == nil {
		return nil, nil, fmt.Errorf("inherited component %q is not defined", component)
	}
	if visited[component] {
		return nil, nil, fmt.Errorf("inheritance cycle detected at component %q", component)
	}
	visited[component] = true
	defer delete(visited, component)

	vars = map[string]any{}
	providers = map[string]any{}
	for _, base := range metadataInherits(section) {
		bVars, bProviders, err := resolveComponentConfig(tfComponents, base, visited)
		if err != nil {
			return nil, nil, err
		}
		vars = deepMerge(vars, bVars)
		providers = deepMerge(providers, bProviders)
	}

	vars = deepMerge(vars, asMap(section[keyVars]))
	providers = deepMerge(providers, asMap(section[keyProviders]))
	return vars, providers, nil
}

// metadataInherits returns the metadata.inherits list for a component section.
func metadataInherits(section map[string]any) []string {
	list, ok := asMap(section[keyMetadata])["inherits"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// deepMerge returns a new map with src deep-merged over dst: nested maps merge
// recursively; all other values (including slices) are replaced by src, matching
// Atmos's default list merge strategy ("replace").
func deepMerge(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst)+len(src))
	maps.Copy(out, dst)
	for k, sv := range src {
		if dv, ok := out[k]; ok {
			if dm, ok := dv.(map[string]any); ok {
				if sm, ok := sv.(map[string]any); ok {
					out[k] = deepMerge(dm, sm)
					continue
				}
			}
		}
		out[k] = sv
	}
	return out
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}
