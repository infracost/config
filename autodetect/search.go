package autodetect

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/infracost/config/plugin"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

type SearchOption func(*SearchOptions)

type SearchOptions struct {
	Template               string
	Identifier             *plugin.Identifier
	MaxSearchDepth         int
	IgnorePermissionErrors bool
	IgnoreHiddenDirs       bool
	SingleFileMode         bool
}

func WithSearchTemplate(template string) SearchOption {
	return func(o *SearchOptions) {
		o.Template = template
	}
}

func WithSearchIdentifier(identifier *plugin.Identifier) SearchOption {
	return func(o *SearchOptions) {
		o.Identifier = identifier
	}
}

func WithSearchMaxDepth(depth int) SearchOption {
	return func(o *SearchOptions) {
		o.MaxSearchDepth = depth
	}
}

func WithSearchIgnorePermissionErrors(ignore bool) SearchOption {
	return func(o *SearchOptions) {
		o.IgnorePermissionErrors = ignore
	}
}

func WithSearchIgnoreHiddenDirs(ignore bool) SearchOption {
	return func(o *SearchOptions) {
		o.IgnoreHiddenDirs = ignore
	}
}

func WithSearchSingleFileMode(single bool) SearchOption {
	return func(o *SearchOptions) {
		o.SingleFileMode = single
	}
}

func SearchForProjects(ctx context.Context, rootDir string, opts ...SearchOption) ([]Project, []RootModule, error) {
	var options SearchOptions
	for _, opt := range opts {
		opt(&options)
	}

	var rawConfig YAML

	if options.Template != "" {
		// if the template has no projects section, we need to add one, so remember this
		if fromTemplate, err := readAutodetectConfigFromTemplate(options.Template); err == nil && fromTemplate != nil {
			rawConfig = *fromTemplate
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to read autodetect config from template: %s", err)
		}
	}

	config, err := rawConfig.Compile()
	if err != nil {
		return nil, nil, fmt.Errorf("autodetect configuration problem: %s", err)
	}

	if options.MaxSearchDepth > 0 {
		config.MaxSearchDepth = options.MaxSearchDepth
	}

	tree, err := newTreeBuilder(options.Identifier, rootDir, config, options.IgnorePermissionErrors, options.IgnoreHiddenDirs, options.SingleFileMode, rootDir).build(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to detect projects: %w", err)
	}

	tree.ModifyTFVarFileEnvs(config)

	projectNodes := filterProjects(tree, tree.FindProjects(), rootDir, config)

	return expandProjects(ctx, options.Identifier, projectNodes, rootDir, config)
}

// filterProjects runs the pre-expansion pipeline: dropping projects that
// shouldn't appear in the final output, decorating the tree with linked var
// files, and gathering the module sources needed by downstream filters.
// Returns the surviving project nodes in walk order.
func filterProjects(tree *Node, projectNodes []*Node, rootDir string, config *Config) []*Node {
	// exclude terragrunt projects whose children include the parent's terragrunt files
	filtered := make([]*Node, 0, len(projectNodes))
	for _, project := range projectNodes {
		// we only check if there are terragrunt files, we don;t care if the project type was overridden
		if !project.Terragrunt.HasFiles {
			filtered = append(filtered, project)
			continue
		}
		var includes bool
		project.VisitDescendants(func(child *Node) bool {
			for _, filename := range []string{"terragrunt.hcl", "terragrunt.hcl.json"} {
				if slices.Contains(child.Terragrunt.IncludedOutsideTerragruntFiles, filepath.Join(project.AbsolutePath, filename)) {
					includes = true
					return false
				}
			}
			return true
		})
		if !includes {
			filtered = append(filtered, project)
		}
	}
	projectNodes = filtered

	// grab all unique local module sources
	moduleSources := map[string]struct{}{}
	tree.WalkOutward(func(n *Node) {
		if n.Terraform.HasFiles {
			for _, source := range n.Terraform.LocalModuleSources {
				moduleSources[source] = struct{}{}
			}
		}
	})

	tree.AssociateLocalTFVarFiles()
	tree.AssociateChildTFVarFiles()
	tree.AssociateSiblingTFVarFiles()
	tree.AssociateParentTFVarFiles()
	tree.AssociatePiblingTFVarFiles()
	tree.AssociateTFVarFilesByProjectName(config)

	// skip terraform projects which have been included as a module by another project
	filtered = make([]*Node, 0, len(projectNodes))
	for _, project := range projectNodes {
		if _, ok := moduleSources[project.AbsolutePath]; !ok {
			filtered = append(filtered, project)
		}
	}
	projectNodes = filtered

	// drop projects rejected by shouldUseProject, with a forced fallback if it would empty the list
	filtered = make([]*Node, 0, len(projectNodes))
	for _, project := range projectNodes {
		if len(projectNodes) == 1 || !config.shouldUseProject(rootDir, project, moduleSources, false) {
			continue
		}
		filtered = append(filtered, project)
	}
	if len(filtered) > 0 {
		projectNodes = filtered
	} else {
		for _, project := range projectNodes {
			if !config.shouldUseProject(rootDir, project, nil, true) {
				continue
			}
			filtered = append(filtered, project)
		}
		projectNodes = filtered
	}

	// skip all terraform projects if terragrunt is present in the repo
	var hasTerragrunt bool
	for _, project := range projectNodes {
		if project.IsTerragrunt() {
			hasTerragrunt = true
			break
		}
	}
	if hasTerragrunt {
		filtered = make([]*Node, 0, len(projectNodes))
		for _, project := range projectNodes {
			if project.IsTerraform() && !config.shouldIncludeDir(rootDir, project.AbsolutePath) {
				continue
			}
			filtered = append(filtered, project)
		}
		projectNodes = filtered
	}

	// remove cfn projects that lie within a tf/tg project
	filtered = make([]*Node, 0, len(projectNodes))
	for _, project := range projectNodes {
		if (!project.IsTerragrunt() && !project.IsTerraform()) && project.IsInsideProject() {
			continue
		}
		filtered = append(filtered, project)
	}
	return filtered
}

// expandProjects materialises one or more Project entries per surviving node. When the plugin
// that owns a node's format can enumerate its environments (via the IdentifyEnvironments RPC) it
// is authoritative; otherwise we fall back to the format-specific heuristic - for Terraform (and
// Terragrunt with linked var files), duplicating the project across the environments its var
// files describe.
func expandProjects(ctx context.Context, identifier *plugin.Identifier, projectNodes []*Node, rootDir string, config *Config) ([]Project, []RootModule, error) {
	projects := make([]Project, 0, len(projectNodes))
	rootModules := make([]RootModule, 0, len(projectNodes))

	// claimedDirs maps a repo-relative directory claimed by some project's environment (via its
	// path or dependency_paths) to the repo-relative path of the project that claimed it. Claimed
	// directories are suppressed from being emitted as standalone projects, so a directory that is
	// part of a parent project's environment isn't also counted as its own project. Only a plugin
	// answering IdentifyEnvironments authoritatively populates this, so it stays empty - and
	// suppression is a no-op - for the fallback path.
	claimedDirs := map[string]string{}

	// expanded holds each node's emitted projects until suppression has been computed across all
	// nodes (a directory can be claimed by a node that appears later in the walk).
	type expandedNode struct {
		node         *Node
		relativePath string
		projects     []Project
	}
	expanded := make([]expandedNode, 0, len(projectNodes))

	for _, project := range projectNodes {

		relativePath, err := filepath.Rel(rootDir, project.AbsolutePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get relative path: %w", err)
		}

		var expandedProjects []Project

		projectName := strings.ReplaceAll(relativePath, string(filepath.Separator), "-")
		if projectName == "." {
			projectName = "main"
		}
		// remove yml/json file ext from project name for CFN
		if project.IsCloudFormation() {
			projectName = trimFileExt(projectName)
		}

		var envFiles []TFVarsFile
		var globalFiles []string
		deps := project.DependencyPaths

		projectBase := filepath.Base(relativePath)
		projectBaseIsEnv := config.EnvMatcher.IsEnvName(projectBase)
		projectBaseEnv := config.EnvMatcher.EnvName(projectBase)

		for _, tfvarFile := range project.Terraform.LinkedTFVarFiles {
			if !tfvarFile.IsGlobal {
				if !projectBaseIsEnv || tfvarFile.Env == projectBaseEnv {
					envFiles = append(envFiles, tfvarFile)
				}
			} else {
				rel, err := filepath.Rel(project.AbsolutePath, tfvarFile.AbsolutePath)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to get relative path for tfvars file %q relative to %q: %w", tfvarFile.AbsolutePath, project.AbsolutePath, err)
				}
				globalFiles = append(globalFiles, rel)
			}
		}

		for _, call := range project.Terraform.LocalModuleSources {
			// only add deps for module includes if they're outside of the project path
			if projRel, err := filepath.Rel(project.AbsolutePath, call); err == nil {
				if strings.HasPrefix(projRel, "..") {
					if rel, err := filepath.Rel(rootDir, call); err == nil {
						deps = append(deps, filepath.Join(rel, "**"))
					}
				}
			}
		}

		for _, include := range project.Terragrunt.IncludedOutsideTerragruntFiles {
			if rel, err := filepath.Rel(rootDir, include); err == nil {
				deps = append(deps, rel)
			}
		}
		for _, source := range project.Terragrunt.LocalOutsideTerraformSources {
			if rel, err := filepath.Rel(rootDir, source); err == nil {
				deps = append(deps, filepath.Join(rel, "**"))
			}
		}

		slices.Sort(globalFiles)

		// we need to switch "**" paths for "./**" because yaml will freak out if the dormer isn't quoted properly, and config templates
		// probably aren't going to remember to do this - may as well remove the footgun
		for i, dep := range deps {
			if dep == "**" {
				deps[i] = "./**"
			}
		}

		slices.Sort(deps)

		projectType := project.ProjectType

		// dedup the deps list
		deps = dedupeStringList(deps)

		// Ask the plugin that owns this format whether it can enumerate the project's
		// environments. If it implements IdentifyEnvironments, its answer is authoritative and we
		// use it directly. If it returns codes.Unimplemented (authoritative == false) we use the
		// fallback below.
		//
		// We hand the plugin the var files config has already attributed to this project (including
		// the cross-directory sibling/pibling association derived by the tree passes) so a
		// Terraform/Terragrunt plugin can reproduce that attribution instead of re-deriving it. Paths
		// are relative to the project dir and may escape it (e.g. "../../env/prod.tfvars").
		var attributedFiles []plugin.AttributedVarFile
		for _, tfvarFile := range project.Terraform.LinkedTFVarFiles {
			rel, err := filepath.Rel(project.AbsolutePath, tfvarFile.AbsolutePath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get relative path for tfvars file %q relative to %q: %w", tfvarFile.AbsolutePath, project.AbsolutePath, err)
			}
			attributedFiles = append(attributedFiles, plugin.AttributedVarFile{
				Path:     rel,
				Env:      tfvarFile.Env,
				IsGlobal: tfvarFile.IsGlobal,
			})
		}

		var pluginEnvironments []plugin.Environment
		var authoritative bool
		if identifier != nil {
			pluginEnvironments, authoritative = identifier.IdentifyEnvironments(ctx, project.AbsolutePath, projectType, attributedFiles)
		}

		switch {
		case authoritative && len(pluginEnvironments) > 0:
			for _, env := range pluginEnvironments {
				if !project.isPathAllowedForEnv(relativePath, env.Name, config) {
					continue
				}

				// the plugin reports paths relative to the project directory; the rest of the
				// pipeline is repo-relative, so rebase them onto the project path.
				envPath := relativePath
				if env.Path != "" {
					envPath = filepath.Join(relativePath, env.Path)
				}

				varFiles := slices.Clone(env.Files)
				slices.Sort(varFiles)

				// merge the plugin's shared dependency dirs with the deps config derived itself
				// (module sources, terragrunt includes).
				envDeps := slices.Clone(deps)
				for _, dep := range env.DependencyPaths {
					envDeps = append(envDeps, filepath.Join(relativePath, dep))
				}
				envDeps = dedupeStringList(envDeps)
				slices.Sort(envDeps)

				envSpecificProjectName := projectName
				if !projectBaseIsEnv {
					envSpecificProjectName += "-" + env.Name
				}

				expandedProjects = append(expandedProjects, Project{
					Name:              escapeStringForYAML(envSpecificProjectName),
					Path:              escapeStringForYAML(envPath),
					TerraformVarFiles: escapeStringListForYAML(varFiles),
					DependencyPaths:   escapeStringListForYAML(envDeps),
					Env:               escapeStringForYAML(env.Name),
					Type:              projectType,
				})

				// record the directories this environment claims so they aren't also emitted as
				// standalone projects. The project's own directory is never suppressed.
				for _, claimed := range append([]string{env.Path}, env.DependencyPaths...) {
					dir := filepath.Clean(filepath.Join(relativePath, strings.TrimSuffix(claimed, "**")))
					if dir == relativePath {
						continue
					}
					if _, ok := claimedDirs[dir]; !ok {
						claimedDirs[dir] = relativePath
					}
				}
			}

		case authoritative:
			// the plugin answered with zero environments: this project genuinely has no variants.
			expandedProjects = append(expandedProjects, Project{
				Name:              escapeStringForYAML(projectName),
				Path:              escapeStringForYAML(relativePath),
				TerraformVarFiles: escapeStringListForYAML(globalFiles),
				DependencyPaths:   escapeStringListForYAML(deps),
				Env:               "", // deliberately empty
				Type:              projectType,
			})

		case len(envFiles) > 0 && (project.IsTerraform() || (project.IsTerragrunt() && project.Terragrunt.LinkTFVars)):
			// Fallback: the manual env detection config performed before the IdentifyEnvironments
			// RPC existed, kept so that earlier versions of plugins keep working (backwards
			// compatibility). This logic is frozen - no changes should be made to it.
			//
			// sometimes there are multiple files for the same org,
			// in this case don't want multiple projects for the same project/dir combo
			groupedEnvFiles := make(map[string][]TFVarsFile)

			for _, env := range envFiles {
				groupedEnvFiles[env.Env] = append(groupedEnvFiles[env.Env], env)
			}

			for envName, envs := range groupedEnvFiles {

				if !project.isPathAllowedForEnv(relativePath, envName, config) {
					continue
				}

				var tfvarFiles []string
				tfvarFiles = append(tfvarFiles, globalFiles...)

				for _, env := range envs {
					rel, err := filepath.Rel(project.AbsolutePath, env.AbsolutePath)
					if err != nil {
						return nil, nil, fmt.Errorf("failed to get relative path for tfvars file %q relative to %q: %w", envs[0].AbsolutePath, project.AbsolutePath, err)
					}

					tfvarFiles = append(tfvarFiles, rel)
				}

				slices.Sort(tfvarFiles)

				envSpecificProjectName := projectName
				if !projectBaseIsEnv {
					envSpecificProjectName += "-" + envName
				}

				expandedProjects = append(expandedProjects, Project{
					Name:              escapeStringForYAML(envSpecificProjectName),
					Path:              escapeStringForYAML(relativePath),
					TerraformVarFiles: escapeStringListForYAML(tfvarFiles),
					DependencyPaths:   escapeStringListForYAML(deps),
					Env:               escapeStringForYAML(envName),
					Type:              projectType,
				})
			}

		default:
			expandedProjects = append(expandedProjects, Project{
				Name:              escapeStringForYAML(projectName),
				Path:              escapeStringForYAML(relativePath),
				TerraformVarFiles: escapeStringListForYAML(globalFiles),
				DependencyPaths:   escapeStringListForYAML(deps),
				Env:               "", // deliberately empty
				Type:              projectType,
			})
		}

		expanded = append(expanded, expandedNode{node: project, relativePath: relativePath, projects: expandedProjects})
	}

	for _, e := range expanded {
		// drop projects whose directory is claimed by a different project's environment.
		if len(claimedDirs) > 0 && isClaimedByOtherProject(e.relativePath, claimedDirs) {
			continue
		}

		rootModules = append(rootModules, RootModule{
			Path:     escapeStringForYAML(e.relativePath),
			Projects: e.projects,
			Type:     e.node.ProjectType,
		})

		projects = append(projects, e.projects...)
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name == projects[j].Name {
			return projects[i].Path < projects[j].Path
		}
		return projects[i].Name < projects[j].Name
	})

	return projects, rootModules, nil
}

// isClaimedByOtherProject reports whether dir sits inside (or equals) a directory that a
// different project's environment claimed (via its path or dependency_paths), and so should not
// also be emitted as a standalone project.
func isClaimedByOtherProject(dir string, claimedDirs map[string]string) bool {
	for claimed, owner := range claimedDirs {
		if owner == dir {
			continue
		}
		if dir == claimed || strings.HasPrefix(dir, claimed+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

type TerragruntFlags struct {
	HasFiles                       bool
	LinkTFVars                     bool
	LocalOutsideTerraformSources   []string // absolute paths
	IncludedOutsideTerragruntFiles []string // absolute paths
}

type TerraformFlags struct {
	HasFiles                          bool
	HasBackend                        bool
	HasProvider                       bool
	LocalModuleSources                []string // absolute paths
	LinkedTFVarFiles                  []TFVarsFile
	LimitLinkedVarFilesToExistingEnvs bool
}

type CloudFormationFlags struct {
	IsCloudFormation bool
}

type TFVarsFlags struct {
	HasFiles bool
	Files    []TFVarsFile
	Used     bool
}

type TFVarsFile struct {
	Name         string
	AbsolutePath string
	Env          string
	IsGlobal     bool
	Owner        *Node
}

func parseHCLFile(src []byte, absPath string) (*hcl.File, error) {
	f, d := hclsyntax.ParseConfig(src, absPath, hcl.Pos{Byte: 0, Line: 1, Column: 1})
	if d != nil && d.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL file: %w", d)
	}
	return f, nil
}

func parseHCLJSONFile(src []byte, absPath string) (*hcl.File, error) {
	f, d := hcljson.Parse(src, absPath)
	if d != nil && d.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL JSON file: %w", d)
	}
	return f, nil
}

const maxTFVarSize = 1 * 1024 * 1024

func isTerraformVarFile(absPath string, autodetect *Config, allowedDirs []string) bool {
	name := filepath.Base(absPath)

	for _, defaultExt := range defaultTFVarExtensions {
		if strings.HasSuffix(name, defaultExt) {
			return true
		}
	}

	// we also check for tfvars.json files as these are non-standard naming
	// conventions which are used by some projects.
	if strings.HasPrefix(name, "tfvars") && strings.HasSuffix(name, ".json") {
		return true
	}

	if len(autodetect.TerraformVarFileExtensions) == 0 {
		return false
	}

	// if we have custom extensions enabled in the autodetect configuration we need
	// to check the extension of the file to see if it matches any of the custom
	var matches bool
	for _, ext := range autodetect.TerraformVarFileExtensions {
		if hasExtension(name, ext) {
			matches = true
			break
		}
	}
	if !matches {
		return false
	}

	// ignore huge files as they're probably not valid tfvars
	info, err := os.Stat(absPath)
	if err != nil {
		return false
	}
	if info.Size() > maxTFVarSize {
		return false
	}

	// if we have custom extensions enabled in the autodetect configuration we need
	// to make sure that this file is a valid HCL file before we add it to the list
	// of discovered var files. This is because we can have collisions with custom
	// env var extensions and other files that are not valid HCL files. e.g. with an
	// empty/wildcard extension we could match a file called "tfvars" and also
	// "Jenkinsfile", the latter being a non-HCL file.
	data, err := readFileWithSymlinkResolution(absPath, allowedDirs)
	if err != nil {
		return false
	}
	// if not valid ut8, this file is probably binary and not a valid tfvars file
	if !utf8.Valid(data) {
		return false
	}
	f, err := parseHCLFile(data, absPath)
	if err != nil {
		return false
	}

	// If the file is empty or has a comment, it would still be considered valid, but would be useless
	// So we check it has at least one attribute defined.
	attr, _ := f.Body.JustAttributes()
	return len(attr) > 0
}

// hasExtension checks if a filename has the provided extension. In
// contrast to [filepath.Ext], this also supports empty / no extensions and
// hidden files correctly.
func hasExtension(filename, ext string) bool {
	// remove leading dot for hidden files, as those interact badly with
	// the rest of the checks: filepath.Ext returns the full filename on
	// hidden files without an extension.
	filename = strings.TrimPrefix(filename, ".")

	// filepath.Ext only returns 'the suffix beginning at the last dot', so
	// for extensions with dots in them (e.g. '.tfvars.json') it would only
	// return the last part.
	if strings.Count(ext, ".") > 1 {
		return strings.HasSuffix(filename, ext)
	}

	return filepath.Ext(filename) == ext
}

func readFileWithSymlinkResolution(path string, allowedDirs []string) ([]byte, error) {
	resolved, err := recursivelyResolveSymlink(path)
	if err != nil {
		return nil, err
	}
	if !isPathAllowed(resolved, allowedDirs...) {
		return nil, fmt.Errorf("path %s is not allowed", resolved)
	}
	// #nosec G304
	return os.ReadFile(resolved)
}

type terraformSniff struct {
	hasProviderBlock         bool
	hasTerraformBackendBlock bool
	localModuleSources       []string
}

func sniffTerraform(fullPath string, file *hcl.File) terraformSniff {
	var sniff terraformSniff

	if file == nil {
		return sniff
	}

	body, content, diags := file.Body.PartialContent(terraformAndProviderBlocks)
	if diags != nil && diags.HasErrors() {
		return sniff
	}

	providerBlocks := body.Blocks.OfType("provider")
	if len(providerBlocks) > 0 {
		sniff.hasProviderBlock = true
	}

	terraformBlocks := body.Blocks.OfType("terraform")
	for _, block := range terraformBlocks {
		backend, _, _ := block.Body.PartialContent(nestedBackendBlock)
		if len(backend.Blocks) > 0 {
			sniff.hasTerraformBackendBlock = true
			break
		}
	}

	dir := filepath.Dir(fullPath)

	moduleBody, _, _ := content.PartialContent(justModuleBlocks)
	for _, module := range moduleBody.Blocks {
		a, _ := module.Body.JustAttributes()
		if src, ok := a["source"]; ok {
			val, _ := src.Expr.Value(nil)

			if val.Type() != cty.String || val.IsNull() || !val.IsKnown() {
				continue
			}

			realPath := val.AsString()

			// we only care about local modules for building a dependency tree
			// so skip any remote modules here.
			if !strings.HasPrefix(realPath, "./") &&
				!strings.HasPrefix(realPath, "../") &&
				!strings.HasPrefix(realPath, ".\\") &&
				!strings.HasPrefix(realPath, "..\\") {
				continue
			}

			mp := filepath.Clean(filepath.Join(dir, realPath))
			sniff.localModuleSources = append(sniff.localModuleSources, mp)
		}
	}
	return sniff
}

var (
	terraformBlocks = &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{
				Type: "terraform",
			},
		},
	}
	terraformAndProviderBlocks = &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{
				Type: "terraform",
			},
			{
				Type:       "provider",
				LabelNames: []string{"name"},
			},
		},
	}
	nestedBackendBlock = &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{
				Type:       "backend",
				LabelNames: []string{"name"},
			},
		},
	}
	justModuleBlocks = &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{
				Type:       "module",
				LabelNames: []string{"name"},
			},
		},
	}
	anonymousIncludeBlocks = &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{
				Type: "include",
			},
		},
	}
	namedIncludeBlocks = &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{
				Type:       "include",
				LabelNames: []string{"name"},
			},
		},
	}
)

func escapeStringListForYAML(strs []string) []string {
	for i, str := range strs {
		strs[i] = escapeStringForYAML(str)
	}
	return strs
}

func dedupeStringList(strs []string) []string {
	unique := make(map[string]struct{})
	deduped := make([]string, 0)
	for _, str := range strs {
		if _, ok := unique[str]; !ok {
			unique[str] = struct{}{}
			deduped = append(deduped, str)
		}
	}
	return deduped
}

func escapeStringForYAML(str string) string {
	node := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Value: str,
	}
	out, err := yaml.Marshal(node)
	if err != nil {
		return str
	}
	return strings.TrimRight(string(out), "\n")
}

func trimFileExt(name string) string {
	if ext := filepath.Ext(name); ext != "" {
		return strings.TrimSuffix(name, ext)
	}
	return name
}

type terragruntSniffResult struct {
	Includes []string
	Sources  []string
}

// isPathInsideDirectory checks if an absolute path is inside an absolute directory
func isPathInsideDirectory(path, dir string) bool {
	return strings.HasPrefix(path+string(filepath.Separator), dir+string(filepath.Separator))
}

func sniffTerragrunt(repoRoot string, fullPath string, allowedDirs ...string) (*terragruntSniffResult, error) {
	// read include dirs and terraform source dirs from a terragrunt.hcl path, limiting to an include depth of 10
	result, err := sniffTerragruntWithDepthLimit(repoRoot, fullPath, 10, allowedDirs...)
	if err != nil {
		return nil, err
	}

	// filter results based to only include sources + includes outside of the directory containing the original file
	// this is because we're looking for includes + sources OUTSIDE of the project directory
	// we also filter out duplicates and paths we're not allowed to read, e.g. if a path traverses out of the repo
	// directory, e.g. ../../../../../../etc/shadow
	dir := filepath.Dir(fullPath)
	var filtered terragruntSniffResult
	for _, include := range result.Includes {
		if !isPathAllowed(include, allowedDirs...) {
			continue
		}
		if slices.Contains(filtered.Includes, include) {
			continue
		}
		if !isPathInsideDirectory(include, dir) {
			filtered.Includes = append(filtered.Includes, include)
		}
	}
	for _, source := range result.Sources {
		if !isPathAllowed(source, allowedDirs...) {
			continue
		}
		if slices.Contains(filtered.Sources, source) {
			continue
		}
		if !isPathInsideDirectory(source, dir) {
			filtered.Sources = append(filtered.Sources, source)
		}
	}
	return &filtered, nil
}

func sniffTerragruntWithDepthLimit(repoRoot, fullPath string, depth int, allowedDirs ...string) (*terragruntSniffResult, error) {
	// sanity check for many/recursive includes
	if depth <= 0 {
		return nil, fmt.Errorf("reached maximum depth limit of %d", depth)
	}

	// don't read files outside of the repo and safe dirs
	if !isPathAllowed(fullPath, allowedDirs...) {
		return nil, fmt.Errorf("path %q is not allowed", fullPath)
	}

	// #nosec G304
	// read the terragrunt file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}

	// parse the terragrunt as either hcl or hcljson
	var f *hcl.File
	if strings.HasSuffix(fullPath, ".json") {
		f, err = parseHCLJSONFile(data, fullPath)
	} else {
		f, err = parseHCLFile(data, fullPath)
	}
	if err != nil {
		return nil, err
	}

	var sniff terragruntSniffResult

	// grab the directory the terragrunt file lives in
	dir := filepath.Dir(fullPath)

	// ask hcl to parse the terragrunt file for terraform blocks only
	body, content, diags := f.Body.PartialContent(terraformBlocks)
	if diags != nil && diags.HasErrors() {
		return nil, diags
	}

	// process all of the terraform blocks and grab any static source paths
	terraformBlocks := body.Blocks.OfType("terraform")
	for _, block := range terraformBlocks {

		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			continue
		}
		if src, ok := attrs["source"]; ok {
			if value := valueFromSimpleExpr(src.Expr); value != "" {
				path := filepath.Clean(filepath.Join(dir, value))
				if isPathAllowed(path, allowedDirs...) {
					sniff.Sources = append(sniff.Sources, path)
				}
			}
		}
	}

	// grab all include blocks
	// NOTE that we have two operations here because terragrunt allows two different schemas:
	// - named include blocks
	// - anonymous include blocks (for backward compatibility)
	anonIncludes, _, _ := content.PartialContent(anonymousIncludeBlocks)
	namedIncludes, _, _ := content.PartialContent(namedIncludeBlocks)
	var includeBlocks hcl.Blocks
	if anonIncludes != nil {
		includeBlocks = append(includeBlocks, anonIncludes.Blocks...)
	}
	if namedIncludes != nil {
		includeBlocks = append(includeBlocks, namedIncludes.Blocks...)
	}

	// wherever we find a path attribute, we need to add this include path to the list of includes,
	// and also parse the included file for further tf sources + include paths
	for _, module := range includeBlocks {
		a, _ := module.Body.JustAttributes()
		src, ok := a["path"]
		if !ok {
			continue
		}

		include := pathFromComplexExpr(repoRoot, dir, src.Expr, allowedDirs...)
		if include == "" {
			continue
		}

		// add the include and recurse into it to find any transitive includes.
		sniff.Includes = append(sniff.Includes, include)

		parentSniff, err := sniffTerragruntWithDepthLimit(repoRoot, include, depth-1, allowedDirs...)
		if err != nil {
			continue
		}

		for _, path := range parentSniff.Includes {
			if !slices.Contains(sniff.Includes, path) {
				sniff.Includes = append(sniff.Includes, path)
			}
		}
		for _, path := range parentSniff.Sources {
			if !slices.Contains(sniff.Sources, path) {
				sniff.Sources = append(sniff.Sources, path)
			}
		}
	}

	return &sniff, nil
}

// pathFromComplexExpr extracts a path from an expression that's either a function
// call to 'find_in_parent_folders' or a simple expression (see [pathFromSimpleExpr]).
// Any returned path is ensured to be within allowedDirs.
func pathFromComplexExpr(repoRoot, dir string, expr hcl.Expression, allowedDirs ...string) string {
	val := valueFromComplexExpr(repoRoot, dir, expr, allowedDirs...)
	if val == "" {
		return ""
	}
	val = filepath.Clean(val)

	if !filepath.IsAbs(val) {
		val = filepath.Join(dir, val)
	}

	if !isPathAllowed(val, allowedDirs...) {
		return ""
	}

	return val
}

// valueFromComplexExpr extracts a value from a complex expression, falling back to valueFromSimpleExpression
func valueFromComplexExpr(repoRoot, dir string, expr hcl.Expression, allowedDirs ...string) string {
	switch v := expr.(type) {
	case *hclsyntax.TemplateWrapExpr:
		return valueFromComplexExpr(repoRoot, dir, v.Wrapped, allowedDirs...)

	case *hclsyntax.TemplateExpr:

		var sb strings.Builder
		for _, expr := range v.Parts {
			part := valueFromComplexExpr(repoRoot, dir, expr, allowedDirs...)
			sb.WriteString(part)
		}

		return sb.String()

	case *hclsyntax.FunctionCallExpr:

		switch v.Name {
		case "get_path_to_repo_root":
			return repoRoot

		case "find_in_parent_folders":

			// terragrunt includes use terragrunt.hcl unless a filename is specified
			filename := "terragrunt.hcl"
			if len(v.Args) > 0 {
				fv, _ := v.Args[0].Value(nil)
				if fv.Type() == cty.String && !fv.IsNull() && fv.IsKnown() && !fv.IsMarked() {
					filename = fv.AsString()
					// recalculate the directory in case the filename contained e.g. ../
					dir = filepath.Dir(filepath.Join(dir, filename))
					filename = filepath.Base(filepath.Join(dir, filename))
				}
			}

			// look upward up to 10 directories
			for range 10 {
				dir = filepath.Dir(dir)
				if !isPathAllowed(dir, allowedDirs...) {
					break
				}
				path := filepath.Join(dir, filename)
				if _, err := os.Stat(path); err != nil {
					continue
				}
				return path
			}
		}

		return ""

	default:
		return valueFromSimpleExpr(expr)
	}
}

// valueFromSimpleExpr extracts a value from an expression that's either a template wrap or
// an expression that can derive a string without context.
func valueFromSimpleExpr(expr hcl.Expression) string {
	switch expr.(type) {
	// skip function calls and wrapped expressions.
	case *hclsyntax.FunctionCallExpr, *hclsyntax.TemplateWrapExpr:
		return ""

	default:
		val, _ := expr.Value(nil)
		if val.Type() != cty.String || val.IsNull() || !val.IsKnown() || val.IsMarked() {
			return ""
		}

		return val.AsString()
	}
}
