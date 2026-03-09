package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/infracost/config/autodetect"
	"github.com/infracost/config/cdk"
	"github.com/infracost/config/template"

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

type GenerationOptions struct {
	Template            string // template content
	DebugTemplate       bool   // debug template parsing
	RepoName            string
	Branch              string
	BaseBranch          string
	IsProjectProduction func(name string) bool
	EnvVars             map[string]string
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

	projects, rootModules, err := autodetect.SearchForProjects(rootDir, genOptions.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to locate projects: %w", err)
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
	if len(output.CDK.Projects) == 0 {
		cdkConfig, err := cdk.GenerateConfig(rootDir)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrCDKConfigGenerationFailed, err)
		}
		if len(cdkConfig) > 0 {
			output.CDK.Projects = cdkConfig
		}
	}

	// if there are cdk projects, finalize the config by merging in cdk defaults
	if len(output.CDK.Projects) > 0 {
		if err := finalizeCDKConfig(rootDir, output); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
		}
	}

	if !hasProjectsSection {
		output.Projects = make([]*Project, 0, len(projects))
		for _, project := range projects {
			output.Projects = append(output.Projects, &Project{
				Path:            project.Path,
				Name:            project.Name,
				EnvName:         project.Env,
				DependencyPaths: project.DependencyPaths,
				Terraform: ProjectTerraform{
					VarFiles:  project.TerraformVarFiles,
					Workspace: project.Env,
				},
				Type: ProjectType(project.Type),
			})
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
