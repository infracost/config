package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/infracost/config/autodetect"
	"github.com/infracost/config/cdk"
	"github.com/infracost/config/plugin"
	"github.com/infracost/config/template"
	"github.com/infracost/config/types"

	"gopkg.in/yaml.v3"
)

type GenerationOption func(*GenerationOptions)

func WithTemplate(template string) GenerationOption {
	return func(o *GenerationOptions) {
		o.Template = template
	}
}

func WithTemplateDebugging(debug bool) GenerationOption {
	return func(o *GenerationOptions) {
		o.DebugTemplate = debug
	}
}

func WithRepoName(name string) GenerationOption {
	return func(o *GenerationOptions) {
		o.RepoName = name
	}
}

func WithBranch(name string) GenerationOption {
	return func(o *GenerationOptions) {
		o.Branch = name
	}
}

func WithBaseBranch(name string) GenerationOption {
	return func(o *GenerationOptions) {
		o.BaseBranch = name
	}
}

func WithEnvVars(vars map[string]string) GenerationOption {
	return func(o *GenerationOptions) {
		o.EnvVars = vars
	}
}

func WithIsProjectProductionFunc(f func(project string) bool) GenerationOption {
	return func(o *GenerationOptions) {
		o.IsProjectProduction = f
	}
}

func WithPluginDir(dir string) GenerationOption {
	return func(o *GenerationOptions) {
		o.PluginDir = dir
	}
}

func WithDefaultPluginDir(useDefault bool) GenerationOption {
	return func(o *GenerationOptions) {
		o.DefaultPluginDir = useDefault
	}
}

func WithMaxSearchDepth(depth int) GenerationOption {
	return func(o *GenerationOptions) {
		o.MaxSearchDepth = depth
	}
}

func WithIgnorePermissionErrors(ignore bool) GenerationOption {
	return func(o *GenerationOptions) {
		o.IgnorePermissionErrors = ignore
	}
}

func WithIgnoreHiddenDirs(ignore bool) GenerationOption {
	return func(o *GenerationOptions) {
		o.IgnoreHiddenDirs = ignore
	}
}

func WithSkipCDK(skip bool) GenerationOption {
	return func(o *GenerationOptions) {
		o.SkipCDK = skip
	}
}

func WithSingleFileMode(single bool) GenerationOption {
	return func(o *GenerationOptions) {
		o.SingleFileMode = single
	}
}

type GenerationOptions struct {
	Template               string // template content
	DebugTemplate          bool   // debug template parsing
	RepoName               string
	Branch                 string
	BaseBranch             string
	IsProjectProduction    func(name string) bool
	EnvVars                map[string]string
	PluginDir              string
	DefaultPluginDir       bool
	MaxSearchDepth         int
	IgnorePermissionErrors bool
	IgnoreHiddenDirs       bool
	SkipCDK                bool
	SingleFileMode         bool
}

var defaultConfigGenerationOptions = GenerationOptions{
	Template: "",
}

var (
	ErrCDKConfigGenerationFailed = errors.New("failed to generate CDK config")
	ErrInvalidConfigYAML         = errors.New("invalid config YAML")
	ErrInvalidConfigTemplate     = errors.New("invalid config template")
)

// Generate takes a repository root  directory and produces a config.
// Options can be used to supply a template etc.
func Generate(
	ctx context.Context,
	rootDir string,
	options ...GenerationOption,
) (*Config, error) {
	if !filepath.IsAbs(rootDir) {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		rootDir = filepath.Join(wd, rootDir)
	}

	genOptions := defaultConfigGenerationOptions

	for _, opt := range options {
		opt(&genOptions)
	}

	output := &Config{
		ConfigBase: ConfigBase{
			Version: CurrentVersion,
		},
	}

	hasProjectsSection := strings.Contains(genOptions.Template, "\nprojects:") || strings.HasPrefix(genOptions.Template, "projects:")

	var identifier *plugin.Identifier
	if genOptions.DefaultPluginDir || genOptions.PluginDir != "" {
		var err error
		identifier, err = plugin.CreateIdentifier(ctx, genOptions.PluginDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create plugin identifier: %w", err)
		}
		defer identifier.Close()
	}

	projects, rootModules, err := autodetect.SearchForProjects(ctx, rootDir,
		autodetect.WithSearchTemplate(genOptions.Template),
		autodetect.WithSearchIdentifier(identifier),
		autodetect.WithSearchMaxDepth(genOptions.MaxSearchDepth),
		autodetect.WithSearchIgnorePermissionErrors(genOptions.IgnorePermissionErrors),
		autodetect.WithSearchIgnoreHiddenDirs(genOptions.IgnoreHiddenDirs),
		autodetect.WithSearchSingleFileMode(genOptions.SingleFileMode),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to locate projects: %w", err)
	}

	// if we're not using plugins, fall back to this legacy ciscostacks bolt-on
	if identifier == nil {
		stacksProjects, stackNames := autodetect.DetectCiscoStacksProjects(rootDir)
		if len(stackNames) > 0 {
			// Filter out autodetected terraform projects under stacks/<name>/
			// where we've detected cisco stacks layers for that stack.
			filtered := make([]autodetect.Project, 0, len(projects))
			for _, p := range projects {
				rel := p.Path
				if filepath.IsAbs(rel) {
					rel, _ = filepath.Rel(rootDir, rel)
				}
				if strings.HasPrefix(rel, "stacks"+string(filepath.Separator)) {
					parts := strings.SplitN(rel, string(filepath.Separator), 3)
					if len(parts) >= 2 && stackNames[parts[1]] {
						continue
					}
				}
				filtered = append(filtered, p)
			}
			projects = append(filtered, stacksProjects...)
		}
	}

	// Atmos detection runs regardless of whether plugins are loaded: the plugin
	// identification path is per-directory and cannot express Atmos's
	// one-component-directory -> N-stack-projects expansion, so it is detected here
	// from atmos.yaml. Overlapping plain-Terraform projects under the Atmos
	// Terraform components directory are filtered out in favour of the Atmos projects.
	atmosProjects, atmosCovered, atmosErr := autodetect.DetectAtmosProjects(rootDir)
	if atmosErr != nil {
		return nil, fmt.Errorf("failed to detect atmos projects: %w", atmosErr)
	}
	if len(atmosProjects) > 0 {
		filtered := make([]autodetect.Project, 0, len(projects))
		for _, p := range projects {
			rel := p.Path
			if filepath.IsAbs(rel) {
				rel, _ = filepath.Rel(rootDir, rel)
			}
			rel = filepath.ToSlash(rel)
			covered := false
			for prefix := range atmosCovered {
				if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
					covered = true
					break
				}
			}
			if !covered {
				filtered = append(filtered, p)
			}
		}
		projects = append(filtered, atmosProjects...)
	}

	variables := template.Variables{
		RepoName:            genOptions.RepoName,
		Branch:              genOptions.Branch,
		BaseBranch:          genOptions.BaseBranch,
		DetectedProjects:    projects,
		DetectedRootModules: rootModules,
	}

	if genOptions.Template != "" {
		var buf bytes.Buffer
		parser := template.NewParser(rootDir, variables, genOptions.IsProjectProduction)
		if err := parser.Compile(genOptions.Template, &buf); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidConfigTemplate, err)
		}

		if genOptions.DebugTemplate {
			lines := strings.Split(buf.String(), "\n")
			fmt.Println("TEMPLATE:")
			for i, line := range lines {
				fmt.Printf("%4d: %s\n", i+1, line)
			}
			fmt.Println()
		}

		if _, err := parseWithAutodetectAllowed(buf.Bytes(), output); err != nil {
			return nil, fmt.Errorf("%w (after template compilation): %s", ErrInvalidConfigTemplate, err)
		}
	}

	// if the user didn't provide a template with a CDK section, try to generate one for them (if needed)
	if !genOptions.SkipCDK && len(output.CDK.Projects) == 0 {
		cdkConfig, err := cdk.GenerateConfig(rootDir, genOptions.IgnorePermissionErrors, genOptions.IgnoreHiddenDirs)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrCDKConfigGenerationFailed, err)
		}
		if len(cdkConfig) > 0 {
			output.CDK.Projects = cdkConfig
		}
	}

	// if there are cdk projects, finalize the config by merging in cdk defaults
	if !genOptions.SkipCDK && len(output.CDK.Projects) > 0 {
		if err := finalizeCDKConfig(rootDir, output, genOptions.IgnorePermissionErrors, genOptions.IgnoreHiddenDirs); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
		}
	}

	if !hasProjectsSection {
		output.Projects = make([]*Project, 0, len(projects))
		for _, project := range projects {
			p := &Project{
				Path:            project.Path,
				Name:            project.Name,
				EnvName:         project.Env,
				DependencyPaths: project.DependencyPaths,
				Metadata:        project.Metadata,
				Terraform: ProjectTerraform{
					VarFiles: project.TerraformVarFiles,
					// Atmos components don't use TF workspaces — the stack name is
					// the workspace abstraction at the Atmos level, not inside TF.
					// Setting Workspace to the stack name would alter terraform.workspace
					// expressions inside component code in ways that diverge from real Atmos.
					Workspace: project.Env,
				},
				Type: ProjectType(project.Type),
			}
			if project.Type == types.ProjectTypeAtmos {
				p.Terraform.Workspace = ""
			}
			output.Projects = append(output.Projects, p)
		}
	}

	if err := output.normalize(); err != nil {
		return nil, fmt.Errorf("%w (failed to normalize config file): %s", ErrInvalidConfigYAML, err)
	}

	if err := output.validate(); err != nil {
		return nil, fmt.Errorf("%w (failed to validate config file): %s", ErrInvalidConfigYAML, err)
	}

	// if we're generating it for use, we need to replace env vars etc. after generation
	raw, err := yaml.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	if _, err = parseConfigFile(raw, genOptions.EnvVars, output); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
	}

	return output, nil
}
