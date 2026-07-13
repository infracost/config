package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/infracost/config/autodetect"
	"github.com/infracost/config/cdk"
	"github.com/infracost/config/plugin"
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

		if _, err := parseWithAutodetectAllowed(buf.Bytes(), output, rootDir); err != nil {
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
				Type:            ProjectType(project.Type),
				// Workspace is a caller-sourced runtime option (passed to the plugin via
				// GenericOptions.Workspace) and is also read outside the plugin, so it stays a top-level
				// field rather than going into the plugins blob.
				Terraform: ProjectTerraform{Workspace: project.Env},
			}

			// Persist the plugin's parse options under plugins.<name>, keyed by the consuming plugin,
			// as a native YAML map so it stays readable and editable. This is now the ONLY place
			// generated config carries these options - the deprecated terraform.* / aws.* fields are
			// no longer emitted (they are folded into plugins.<name> on read for hand-written configs).
			opts, err := generatedPluginOptions(project)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to build plugin options for project %q: %s", ErrInvalidConfigYAML, project.Path, err)
			}
			if len(opts) > 0 {
				p.Plugins = map[string]map[string]any{pluginKeyForType(p.Type): opts}
			}

			output.Projects = append(output.Projects, p)
		}
	}

	if err := output.normalize(); err != nil {
		return nil, fmt.Errorf("%w (failed to normalize config file): %s", ErrInvalidConfigYAML, err)
	}

	if err := output.validate(rootDir); err != nil {
		return nil, fmt.Errorf("%w (failed to validate config file): %s", ErrInvalidConfigYAML, err)
	}

	// if we're generating it for use, we need to replace env vars etc. after generation
	raw, err := yaml.Marshal(output)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	if _, err = parseConfigFile(raw, output, rootDir); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidConfigYAML, err)
	}

	return output, nil
}

// generatedPluginOptions returns the plugins.<name> blob to persist for an autodetected project, as
// a native map ready to store under plugins.<name>.
//
// When a plugin authored a raw_options blob during identification (only when it returned
// environments authoritatively) that blob is persisted verbatim. Otherwise - when the plugin
// returned no environments, was unimplemented, or no plugin was used - config sideloads the var
// files it attributed locally into a terraform-family blob. Those var files depend on the
// whole-directory (sibling/pibling) attribution config performs in its tree passes, which the
// plugins cannot yet reproduce, so config still owns them; this sideload is the one deliberate,
// minimal special case and is expected to go away once IdentifyAllProjects moves that attribution
// plugin-side. Non-terraform-family projects get no generated blob - their options (e.g. aws
// context) are user-authored and folded in on read.
func generatedPluginOptions(project autodetect.Project) (map[string]any, error) {
	if len(project.RawOptions) > 0 {
		var opts map[string]any
		if err := json.Unmarshal(project.RawOptions, &opts); err != nil {
			return nil, err
		}
		return opts, nil
	}

	if !isTerraformFamily(ProjectType(project.Type)) {
		return nil, nil
	}

	opts := map[string]any{}
	if len(project.TerraformVarFiles) > 0 {
		opts["var_files"] = project.TerraformVarFiles
	}
	// Workspace is not part of the blob - it is written to the top-level terraform.workspace field
	// (see Generate) and passed to the plugin via GenericOptions.Workspace.
	return opts, nil
}
