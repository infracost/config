package autodetect

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
	"github.com/stretchr/testify/assert/yaml"
)

type sniff struct {
	Autodetect YAML `yaml:"autodetect,omitempty" ignored:"true"`
}

type YAML struct {
	EnvNames                   []string       `yaml:"env_names,omitempty"`
	ExcludeDirs                []string       `yaml:"exclude_dirs,omitempty"`
	IncludeDirs                []string       `yaml:"include_dirs,omitempty"`
	PathOverrides              []PathOverride `yaml:"path_overrides,omitempty"`
	TerraformVarFileExtensions []string       `yaml:"terraform_var_file_extensions,omitempty"`
	MaxSearchDepth             int            `yaml:"max_search_depth,omitempty"`
	PreferFolderNameForEnv     bool           `yaml:"prefer_folder_name_for_env,omitempty"`
	// TODO: remove this once we've updated existing configs
	ForceProjectType       string `yaml:"force_project_type,omitempty"` // DEPRECATED - should use link_tfvars_to_terragrunt=true instead of forcing the type to Terraform
	LinkTFVarsToTerragrunt bool   `yaml:"link_tfvars_to_terragrunt,omitempty"`
}

type Config struct {
	ExcludeDirs                []glob.Glob
	RawExcludeDirs             []string
	IncludeDirs                []glob.Glob
	RawIncludeDirs             []string
	PathOverrides              []CompiledPathOverride
	TerraformVarFileExtensions []string
	MaxSearchDepth             int
	PreferFolderNameForEnv     bool
	EnvMatcher                 *EnvFileMatcher
	LinkTFVarsToTerragrunt     bool
}

var defaultTFVarExtensions = []string{
	".tfvars",
	".auto.tfvars",
	".tfvars.json",
	".auto.tfvars.json",
}

// higher than default from cli (7) because CLI ignored max for terragrunt projects
var defaultMaxSearchDepth = 14

type CompiledPathOverride struct {
	Path    glob.Glob `yaml:"path"`
	Exclude []string  `yaml:"exclude"`
	Only    []string  `yaml:"only"`
}

func (a *YAML) Compile() (*Config, error) {
	compiled := Config{
		TerraformVarFileExtensions: a.TerraformVarFileExtensions,
		MaxSearchDepth:             a.MaxSearchDepth,
		PreferFolderNameForEnv:     a.PreferFolderNameForEnv,
		RawExcludeDirs:             a.ExcludeDirs,
		RawIncludeDirs:             a.IncludeDirs,
		LinkTFVarsToTerragrunt:     a.LinkTFVarsToTerragrunt || a.ForceProjectType == "terraform",
	}

	if compiled.MaxSearchDepth == 0 {
		compiled.MaxSearchDepth = defaultMaxSearchDepth
	}

	for _, excludeDir := range a.ExcludeDirs {
		g, err := glob.Compile(excludeDir)
		if err != nil {
			return nil, fmt.Errorf("failed to compile glob pattern for exclude dir %q: %w", excludeDir, err)
		}
		compiled.ExcludeDirs = append(compiled.ExcludeDirs, g)
	}

	for _, includeDir := range a.IncludeDirs {
		g, err := glob.Compile(includeDir)
		if err != nil {
			return nil, fmt.Errorf("failed to compile glob pattern for include dir %q: %w", includeDir, err)
		}
		compiled.IncludeDirs = append(compiled.IncludeDirs, g)
	}

	for _, pathOverride := range a.PathOverrides {
		g, err := glob.Compile(pathOverride.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to compile glob pattern for path override %q: %w", pathOverride.Path, err)
		}
		compiled.PathOverrides = append(compiled.PathOverrides, CompiledPathOverride{
			Path:    g,
			Exclude: pathOverride.Exclude,
			Only:    pathOverride.Only,
		})
	}

	exts := defaultTFVarExtensions
	if len(a.TerraformVarFileExtensions) > 0 {
		exts = a.TerraformVarFileExtensions
	}
	compiled.EnvMatcher = CreateEnvFileMatcher(a.EnvNames, exts)

	return &compiled, nil
}

func (c *Config) shouldSkipDir(rootPath string, dir string) bool {
	if rel, err := filepath.Rel(rootPath, dir); err == nil {
		dir = rel
	}
	for _, g := range c.ExcludeDirs {
		if g.Match(dir) {
			return true
		}
	}
	return false
}

func (c *Config) shouldIncludeDir(rootPath string, dir string) bool {
	if rel, err := filepath.Rel(rootPath, dir); err == nil {
		dir = rel
	}
	for _, g := range c.IncludeDirs {
		if g.Match(dir) {
			return true
		}
	}
	return false
}

func (c *Config) shouldUseProject(rootPath string, node *Node, moduleSources map[string]struct{}, force bool) bool {

	// if the directory is marked as excluded and not included we skip it.
	// The include_dirs setting takes precedence over exclude_dirs.
	if c.shouldSkipDir(rootPath, node.AbsolutePath) && !c.shouldIncludeDir(rootPath, node.AbsolutePath) {
		return false
	}

	if c.shouldIncludeDir(rootPath, node.AbsolutePath) {
		return true
	}

	// if the directory was used as a module source we skip it, even if forced
	if moduleSources != nil {
		if _, ok := moduleSources[node.AbsolutePath]; ok {
			return false
		}
	}

	if force {
		return true
	}

	// skip terraform directories that don't have a backend or provider, and are not terragrunt projects
	if (!node.Terraform.HasBackend && !node.Terraform.HasProvider) && !node.Terragrunt.HasFiles && node.Terraform.HasFiles {
		return false
	}

	return true
}

type PathOverride struct {
	Path    string   `yaml:"path"`
	Exclude []string `yaml:"exclude"`
	Only    []string `yaml:"only"`
}

// parse the autodetect section out of a config template
// we can't simply do this with the yaml package, as it's a go tempalte and likely not valid yaml!
// however, we can assume that the autodetect section is valid yaml, as it's just a struct with simple fields, and no templating, so we can extract that section and parse it separately
func readAutodetectConfigFromTemplate(template string) (*YAML, error) {
	r := bufio.NewReader(strings.NewReader(template))
	var recording bool
	var indent int
	var w bytes.Buffer
	for {
		line, err := r.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}

		if line == "autodetect:\n" {
			recording = true
			indent = getIndentation(line)
		} else if recording && getIndentation(line) <= indent && line != "\n" {
			break
		}

		if recording {
			w.WriteString(line)
		}

		if err == io.EOF {
			break
		}
	}

	if w.Len() == 0 {
		return nil, nil
	}

	var autodetect sniff
	if err := yaml.Unmarshal(w.Bytes(), &autodetect); err != nil {
		return nil, fmt.Errorf("yaml unmarshal failed: %w", err)
	}

	return &autodetect.Autodetect, nil
}

func getIndentation(s string) int {
	for i, c := range s {
		if c != ' ' {
			return i
		}
	}
	return len(s)
}
