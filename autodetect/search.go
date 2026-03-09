package autodetect

import (
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
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

func SearchForProjects(rootDir, template string) ([]Project, []RootModule, error) {

	var rawConfig YAML

	if template != "" {
		// if the template has no projects section, we need to add one, so remember this
		if fromTemplate, err := readAutodetectConfigFromTemplate(template); err == nil && fromTemplate != nil {
			rawConfig = *fromTemplate
		} else if err != nil {
			return nil, nil, fmt.Errorf("failed to read autodetect config from template: %s", err)
		}

	}

	config, err := rawConfig.Compile()
	if err != nil {
		return nil, nil, fmt.Errorf("autodetect configuration problem: %s", err)
	}

	tree, err := buildDirectoryTree(rootDir, rootDir, config, 0, nil, rootDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to detect projects: %w", err)
	}

	tree.ModifyTFVarFileEnvs(config)

	projectNodes := tree.FindProjects()

	// exclude projects which have terragrunt and are included by a child
	filteredProjects := make([]*Node, 0, len(projectNodes))
	// we only include a terragrunt project if there are no children which include a parent
	for _, project := range projectNodes {
		// we only check if there are terragrunt files, we don;t care if the project type was overridden
		if !project.Terragrunt.HasFiles {
			filteredProjects = append(filteredProjects, project)
			continue
		}
		var includes bool
		// we only include a terragrunt project if there are no children which include a parent
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
			filteredProjects = append(filteredProjects, project)
		}
	}
	projectNodes = filteredProjects

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
	filteredProjects = make([]*Node, 0, len(projectNodes))
	for _, project := range projectNodes {
		if _, ok := moduleSources[project.AbsolutePath]; !ok {
			filteredProjects = append(filteredProjects, project)
		}
	}
	projectNodes = filteredProjects

	// filter out projects which should not be used
	filteredProjects = make([]*Node, 0, len(projectNodes))
	for _, project := range projectNodes {

		if len(projectNodes) == 1 || !config.shouldUseProject(rootDir, project, moduleSources, false) {
			continue
		}
		filteredProjects = append(filteredProjects, project)
	}
	if len(filteredProjects) > 0 {
		projectNodes = filteredProjects
	} else {
		// if we filtered out all of the projects with shouldUseProject, we can fall back to forcing
		for _, project := range projectNodes {

			if !config.shouldUseProject(rootDir, project, nil, true) {
				continue
			}
			filteredProjects = append(filteredProjects, project)
		}
		projectNodes = filteredProjects
	}

	// skip all terraform projects if terragrunt is present in the repo
	var hasTerragrunt bool
	for _, project := range projectNodes {
		if project.IsTerragrunt() {
			hasTerragrunt = true
		}
	}
	filteredProjects = make([]*Node, 0, len(projectNodes))
	for _, project := range projectNodes {
		if project.IsTerraform() && hasTerragrunt && !config.shouldIncludeDir(rootDir, project.AbsolutePath) {
			continue
		}
		filteredProjects = append(filteredProjects, project)
	}
	projectNodes = filteredProjects

	// remove cfn projects that lie within a tf/tg project
	filteredProjects = make([]*Node, 0, len(projectNodes))
	for _, project := range projectNodes {
		if project.IsCloudFormation() && project.IsInsideProject() {
			continue
		}
		filteredProjects = append(filteredProjects, project)
	}
	projectNodes = filteredProjects

	// duplicate projects by env
	projects := make([]Project, 0, len(projectNodes))
	rootModules := make([]RootModule, 0, len(projectNodes))
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
		var deps []string

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

		sort.Slice(globalFiles, func(i, j int) bool {
			return globalFiles[i] < globalFiles[j]
		})

		// we need to switch "**" paths for "./**" because yaml will freak out if the dormer isn't quoted properly, and config templates
		// probably aren't going to remember to do this - may as well remove the footgun
		for i, dep := range deps {
			if dep == "**" {
				deps[i] = "./**"
			}
		}

		sort.Slice(deps, func(i, j int) bool {
			return deps[i] < deps[j]
		})

		var projectType ProjectType
		switch {
		case project.IsTerraform():
			projectType = ProjectTypeTerraform
		case project.IsTerragrunt():
			projectType = ProjectTypeTerragrunt
		case project.IsCloudFormation():
			projectType = ProjectTypeCloudFormation
		}

		// dedup the deps list
		deps = dedupeStringList(deps)

		// we only expand projects if they are terraform projects (or are forced to be terraform projects)
		if len(envFiles) > 0 && (project.IsTerraform() || (project.IsTerragrunt() && project.Terragrunt.LinkTFVars)) {

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

				sort.Slice(tfvarFiles, func(i, j int) bool {
					return tfvarFiles[i] < tfvarFiles[j]
				})

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
		} else {
			expandedProjects = append(expandedProjects, Project{
				Name:              escapeStringForYAML(projectName),
				Path:              escapeStringForYAML(relativePath),
				TerraformVarFiles: escapeStringListForYAML(globalFiles),
				DependencyPaths:   escapeStringListForYAML(deps),
				Env:               "", // deliberately empty
				Type:              projectType,
			})
		}

		rootModules = append(rootModules, RootModule{
			Path:     escapeStringForYAML(relativePath),
			Projects: expandedProjects,
			Type:     projectType,
		})

		projects = append(projects, expandedProjects...)
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name == projects[j].Name {
			return projects[i].Path < projects[j].Path
		}
		return projects[i].Name < projects[j].Name
	})

	return projects, rootModules, nil
}

type Node struct {
	Name           string
	AbsolutePath   string
	Children       []*Node
	Terragrunt     TerragruntFlags
	Terraform      TerraformFlags
	CloudFormation CloudFormationFlags
	TFVars         TFVarsFlags
	Depth          int
	Parent         *Node
}

func (n *Node) IsRoot() bool {
	return n.Parent == nil
}

func (n *Node) LinkTFVarFiles(tfVarFiles []TFVarsFile, limitIfLinkedEnv bool) {
	var hasEnv bool
	for _, tfVarFile := range tfVarFiles {
		if n.LinkTFVarFile(tfVarFile, false) && !tfVarFile.IsGlobal {
			hasEnv = true
		}
	}
	if hasEnv && limitIfLinkedEnv {
		n.Terraform.LimitLinkedVarFilesToExistingEnvs = true
	}
}

func (n *Node) LinkTFVarFile(tfVarFile TFVarsFile, limitIfLinkedEnv bool) bool {
	if !tfVarFile.IsGlobal && n.Terraform.LimitLinkedVarFilesToExistingEnvs {
		exists := false
		for _, existing := range n.Terraform.LinkedTFVarFiles {
			if existing.Env == tfVarFile.Env {
				exists = true
				break
			}
		}
		if !exists {
			return false
		}
	}
	if !tfVarFile.IsGlobal && limitIfLinkedEnv {
		n.Terraform.LimitLinkedVarFilesToExistingEnvs = true
	}
	n.Terraform.LinkedTFVarFiles = append(n.Terraform.LinkedTFVarFiles, tfVarFile)
	return true
}

func (n *Node) IsProject() bool {
	return n.IsTerraform() || n.IsTerragrunt() || n.IsCloudFormation()
}

func (n *Node) HasProjects() bool {
	if n.IsProject() {
		return true
	}
	for _, child := range n.Children {
		if child.HasProjects() {
			return true
		}
	}
	return false
}

func (n *Node) FindProjects() []*Node {
	var projects []*Node
	if n.IsProject() {
		projects = append(projects, n)
	}
	for _, child := range n.Children {
		projects = append(projects, child.FindProjects()...)
	}
	return projects
}

func (n *Node) IsTerraform() bool {
	return n.Terraform.HasFiles && !n.Terragrunt.HasFiles
}

func (n *Node) IsTerragrunt() bool {
	return n.Terragrunt.HasFiles
}

func (n *Node) IsCloudFormation() bool {
	return n.CloudFormation.IsCloudFormation
}

func (n *Node) IsInsideProject() bool {
	if n == nil {
		return false
	}
	n = n.Parent
	for n != nil {
		if n.IsProject() {
			return true
		}
		n = n.Parent
	}
	return false
}

func (n *Node) VisitDescendants(fn func(n *Node) bool) {
	if fn == nil {
		return
	}
	for _, child := range n.Children {
		if !fn(child) {
			return
		}
		child.VisitDescendants(fn)
	}
}

func (n *Node) WalkInward(fn func(n *Node)) {
	if fn == nil {
		return
	}
	for _, child := range n.Children {
		child.WalkInward(fn)
	}
	fn(n)
}

func (n *Node) WalkOutward(fn func(n *Node)) {
	if fn == nil {
		return
	}
	fn(n)
	for _, child := range n.Children {
		child.WalkOutward(fn)
	}
}

func (n *Node) IsExclusivelyTFVarsDirectory() bool {
	return n.TFVars.HasFiles && !n.IsTerraform() && !n.IsTerragrunt()
}

func (n *Node) DescendantsWithAssignableVarFiles() []*Node {
	var descendants []*Node
	for _, child := range n.Children {
		if child.IsExclusivelyTFVarsDirectory() && !child.TFVars.Used {
			descendants = append(descendants, child)
		}
	}
	// otherwise look for more distant descendants
	for _, child := range n.Children {
		descendants = append(descendants, child.DescendantsWithAssignableVarFiles()...)
	}
	return descendants
}

func (n *Node) IsNonEmpty() bool {
	return n.IsTerraform() || n.IsTerragrunt() || n.IsExclusivelyTFVarsDirectory()
}

func (n *Node) ChildNodes() []*Node {
	var children []*Node
	for _, child := range n.Children {
		if child.IsNonEmpty() {
			children = append(children, child)
		}
	}

	if len(children) > 0 {
		return children
	}

	for _, child := range n.Children {
		children = append(children, child.ChildNodes()...)
	}

	return children
}

func (n *Node) CanLinkTFVarsFiles() bool {
	return n.IsTerraform() || (n.IsTerragrunt() && n.Terragrunt.LinkTFVars)
}

func (n *Node) AssociateLocalTFVarFiles() {
	n.WalkInward(func(n *Node) {

		// don't assign var files to non-project paths
		if !n.CanLinkTFVarsFiles() {
			return
		}

		if n.TFVars.HasFiles {
			n.LinkTFVarFiles(n.TFVars.Files, true)

			n.TFVars.Used = true
		}
	})
}

func (n *Node) GetSiblings() []*Node {
	if n.Parent == nil {
		return nil
	}
	var siblings []*Node
	for _, child := range n.Parent.Children {
		if child != n {
			siblings = append(siblings, child)
		}
	}
	return siblings
}

// AssociateChildTFVarFiles makes sure that any projects with directories which
// contain var files are associated with the project. These are only associated
// if they are within 2 levels of the project and not if the child directory is a
// valid sibling directory.
func (n *Node) AssociateChildTFVarFiles() {
	n.WalkInward(func(n *Node) {

		// don't assign var files to non-terraform paths
		if !n.CanLinkTFVarsFiles() {
			return
		}

		descendants := n.DescendantsWithAssignableVarFiles()

		for _, descendant := range descendants {
			// if the child has already been associated with a project skip it as the var
			// directory has already been associated with a root module which is a closer
			// relation to it than the current root path.
			if descendant.TFVars.Used {
				continue
			}

			depth := descendant.Depth - n.Depth
			if depth > 3 {
				continue
			}

			// if the child dir is also a valid sibling diretory, AND there are more valid
			// sibling directories further up the hierarchy, skip it, because we want to prefer
			// siblings in this case.
			siblingHasProject := false
			siblings := descendant.GetSiblings()
			for _, sibling := range siblings {
				if (sibling.CanLinkTFVarsFiles()) && len(sibling.Terraform.LinkedTFVarFiles) == 0 {
					siblingHasProject = true
					break
				}
			}
			if siblingHasProject {
				ancestorHasSiblingDir := false
				parent := n
				for parent != nil {
					for _, sib := range parent.GetSiblings() {
						if sib.IsExclusivelyTFVarsDirectory() {
							ancestorHasSiblingDir = true
							break
						}
					}
					if ancestorHasSiblingDir {
						break
					}
					parent = parent.Parent
				}
				if ancestorHasSiblingDir {
					continue
				}
			}

			n.LinkTFVarFiles(descendant.TFVars.Files, false)
			descendant.TFVars.Used = true
		}
	})
}

func (n *Node) AssociateSiblingTFVarFiles() {
	n.WalkOutward(func(n *Node) {
		var rootPaths []*Node
		var varDirs []*Node
		for _, node := range n.Children {
			if node.CanLinkTFVarsFiles() {
				rootPaths = append(rootPaths, node)
			}

			if node.IsExclusivelyTFVarsDirectory() && !node.TFVars.Used {
				varDirs = append(varDirs, node)
			}
		}

		for _, path := range rootPaths {
			if len(path.Terraform.LinkedTFVarFiles) == 0 {
				for _, dir := range varDirs {
					dir.TFVars.Used = true
					path.LinkTFVarFiles(dir.TFVars.Files, false)
				}
			}
		}
	})
}

func (n *Node) UnusedParentVarFiles() []TFVarsFile {

	if n.Parent == nil {
		return nil
	}

	var varFiles []TFVarsFile
	if n.Parent.TFVars.HasFiles && !n.Parent.TFVars.Used {
		varFiles = append(varFiles, n.Parent.TFVars.Files...)
	}

	return append(varFiles, n.Parent.UnusedParentVarFiles()...)
}

func (n *Node) AssociateParentTFVarFiles() {
	n.WalkInward(func(n *Node) {
		varFiles := n.UnusedParentVarFiles()
		if n.CanLinkTFVarsFiles() {
			n.LinkTFVarFiles(varFiles, false)
		}
	})
}

// Pibling is the gender-neutral term for aunt/uncle (TFVars files are non-binary)
func (n *Node) AssociatePiblingTFVarFiles() {

	n.WalkInward(func(n *Node) {
		if n.IsProject() {
			varFiles := n.UnusedParentVarFiles()
			for _, varFile := range varFiles {
				varFile.Owner.TFVars.Used = true
			}
		}
	})

	// then find all tfvars files that are not used and link them to their common parent
	n.WalkInward(func(n *Node) {
		if !n.TFVars.HasFiles || n.TFVars.Used || n.IsRoot() {
			return
		}

		commonParent := n.FindTfvarsCommonParent()
		if commonParent == nil {
			return
		}

		for _, node := range commonParent.ChildNodesRecursivelyExcluding(n, nil) {
			if node.CanLinkTFVarsFiles() {
				node.LinkTFVarFiles(n.TFVars.Files, false)
			}
		}

	})

	n.WalkInward(func(n *Node) {
		varFiles := n.UnusedParentVarFiles()
		for _, varFile := range varFiles {
			varFile.Owner.TFVars.Used = true
		}
	})
}

// by default, the env name of a tfvar file is based on its filename, e.g. prod.tfvars -> prod
// however, the env name can also be inferred from the directory name, e.g. dev/config.tfvars -> dev
func (n *Node) ModifyTFVarFileEnvs(autodetect *Config) {
	n.WalkInward(func(n *Node) {
		// walk every tfvars directory
		if !n.IsExclusivelyTFVarsDirectory() {
			return
		}

		var possibleDirEnvName string

		// find parent dirs that contain nothing but tfvars files
		parent := n
		for len(parent.ChildNodesRecursivelyExcluding(n, func(n *Node) bool {
			return !n.IsProject()
		})) == 0 {

			base := filepath.Base(parent.AbsolutePath)
			if autodetect.EnvMatcher.IsEnvName(base) {
				possibleDirEnvName = autodetect.EnvMatcher.EnvName(base)
				break
			}

			parent = parent.Parent
			if parent == nil || parent.IsRoot() {
				break
			}
		}

		// no env dir found for this tfvars directory, move on
		if possibleDirEnvName == "" {
			return
		}

		for i, f := range n.TFVars.Files {
			// if this file has no env, or we prefer the folder name for env, set it to the possible dir env
			if f.IsGlobal || autodetect.PreferFolderNameForEnv {
				f.Env = possibleDirEnvName
				f.IsGlobal = false
				n.TFVars.Files[i] = f
			}
		}

	})
}

// AssociateTFVarFilesByProjectName associates tfvars files with projects of the same name
// and disassociates the tfvars from other projects. For example, foo.tfvars would be linked
// to the "foo" projects and unlinked from others. If no project is found with the same name
// the tfvars file is left as is, and no linking/unlinking is performed.
func (n *Node) AssociateTFVarFilesByProjectName(autodetect *Config) {

	found := make(map[string]bool)
	n.WalkOutward(func(n *Node) {
		base := filepath.Base(n.AbsolutePath)
		for _, varFile := range n.Terraform.LinkedTFVarFiles {
			name := autodetect.EnvMatcher.clean(varFile.Name)
			if base == name {
				found[varFile.AbsolutePath] = true
			}
		}
	})

	// filter terraform var files from the root paths that have
	// the same name as another root directory. This means that
	// terraform var files that are scoped to a specific project
	// are not added to another project.
	n.WalkOutward(func(n *Node) {
		base := filepath.Base(n.AbsolutePath)
		var filtered []TFVarsFile
		for _, varFile := range n.Terraform.LinkedTFVarFiles {
			name := autodetect.EnvMatcher.clean(varFile.Name)
			if found[varFile.AbsolutePath] && base != name {
				continue
			}
			filtered = append(filtered, varFile)
		}
		n.Terraform.LinkedTFVarFiles = filtered
	})

}

// look at each path_override in turn, if the override has an "only" list:
// - if any "only" rule is matched, allow the env, otherwise disallow it
// look at each path_override in turn, if the override has an "exclude" list:
// - if any "exclude" rule is matched, disallow the env, otherwise continue
// finally, after processing all rules, allow the env
func (n *Node) isPathAllowedForEnv(relativePath, env string, autodetect *Config) bool {
	if len(autodetect.PathOverrides) == 0 {
		return true
	}
	for _, override := range autodetect.PathOverrides {
		if override.Path.Match(relativePath) {
			if len(override.Only) > 0 {
				// if any "only" rule is matched, alow the env
				return slices.Contains(override.Only, env)
			}
		}
	}
	for _, override := range autodetect.PathOverrides {
		if override.Path.Match(relativePath) {
			if slices.Contains(override.Exclude, env) {
				return false
			}
		}
	}
	return true
}

// ChildNodesRecursivelyExcluding collects all the child nodes of the current node,
// excluding the given root node.
func (n *Node) ChildNodesRecursivelyExcluding(exclude *Node, excludeFunc func(n *Node) bool) []*Node {
	var children []*Node
	for _, child := range n.Children {
		if excludeFunc != nil && excludeFunc(child) {
			continue
		}
		if child != exclude {
			children = append(children, child)
		}
	}

	for _, child := range n.Children {
		if child != exclude {
			children = append(children, child.ChildNodesRecursivelyExcluding(exclude, excludeFunc)...)
		}
	}

	return children
}

// FindTfvarsCommonParent returns the first parent directory that has a child
// directory with a root Terraform project.
func (n *Node) FindTfvarsCommonParent() *Node {
	parent := n.Parent

	for {
		if parent == nil {
			return nil
		}

		if len(parent.ChildNodesRecursivelyExcluding(n, func(n *Node) bool {
			return !n.IsTerraform() && !n.IsTerragrunt()
		})) > 0 {
			return parent
		}

		parent = parent.Parent
	}
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

func buildDirectoryTree(repoRoot, path string, autodetectConfig *Config, depth int, parent *Node, allowedDirs ...string) (*Node, error) {

	path, err := recursivelyResolveSymlink(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve symlink: %w", err)
	}
	if !isPathAllowed(path, allowedDirs...) {
		return nil, fmt.Errorf("path %q is not allowed", path)
	}

	node := &Node{
		Name:         filepath.Base(path),
		Parent:       parent,
		AbsolutePath: path,
		Depth:        depth,
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {

		info, err := entry.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(path, info.Name())

		var isSymlink bool

		// if entry is symlink
		if info.Mode()&os.ModeSymlink != 0 {

			isSymlink = true

			// resolve symlink
			resolved, err := recursivelyResolveSymlink(fullPath)
			if err != nil {
				continue
			}
			if !isPathAllowed(resolved, allowedDirs...) {
				continue
			}
			info, err = os.Stat(resolved)
			if err != nil {
				continue
			}
			fullPath = resolved
		}

		if info.IsDir() {
			// don't recurse down symlinks, we walk the whole tree anyway
			if isSymlink {
				continue
			}
			if slices.Contains(defaultExcludedDirs, info.Name()) {
				continue
			}
			if depth+1 < autodetectConfig.MaxSearchDepth {
				childNode, err := buildDirectoryTree(repoRoot, fullPath, autodetectConfig, depth+1, node, allowedDirs...)
				if err == nil {
					node.Children = append(node.Children, childNode)
				}
			}
			continue
		}

		switch {
		case strings.HasSuffix(strings.ToLower(info.Name()), ".tf"),
			strings.HasSuffix(strings.ToLower(info.Name()), ".tofu"):
			node.Terraform.HasFiles = true
			// #nosec G304
			src, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			f, err := parseHCLFile(src, fullPath)
			if err == nil {
				sniff := sniffTerraform(fullPath, f)
				node.Terraform.HasBackend = node.Terraform.HasBackend || sniff.hasTerraformBackendBlock
				node.Terraform.HasProvider = node.Terraform.HasProvider || sniff.hasProviderBlock
				node.Terraform.LocalModuleSources = append(node.Terraform.LocalModuleSources, sniff.localModuleSources...)
			}
		case strings.HasSuffix(strings.ToLower(info.Name()), ".tf.json"),
			strings.HasSuffix(strings.ToLower(info.Name()), ".tofu.json"):
			// #nosec G304
			data, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}
			f, err := parseHCLJSONFile(data, fullPath)
			node.Terraform.HasFiles = true
			if err == nil {
				sniff := sniffTerraform(fullPath, f)
				node.Terraform.HasBackend = node.Terraform.HasBackend || sniff.hasTerraformBackendBlock
				node.Terraform.HasProvider = node.Terraform.HasProvider || sniff.hasProviderBlock
				node.Terraform.LocalModuleSources = append(node.Terraform.LocalModuleSources, sniff.localModuleSources...)
			}
		case info.Name() == "terragrunt.hcl" || info.Name() == "terragrunt.hcl.json":
			node.Terragrunt.HasFiles = true
			node.Terragrunt.LinkTFVars = autodetectConfig.LinkTFVarsToTerragrunt
			sniff, err := sniffTerragrunt(repoRoot, fullPath, allowedDirs...)
			if err != nil {
				continue
			}
			node.Terragrunt.LocalOutsideTerraformSources = append(node.Terragrunt.LocalOutsideTerraformSources, sniff.Sources...)
			node.Terragrunt.IncludedOutsideTerragruntFiles = append(node.Terragrunt.IncludedOutsideTerragruntFiles, sniff.Includes...)
		case IdentifyCloudFormationPath(fullPath):
			node.Children = append(node.Children, &Node{
				Name:         info.Name(),
				AbsolutePath: fullPath,
				Parent:       node,
				Depth:        depth + 1,
				CloudFormation: CloudFormationFlags{
					IsCloudFormation: true,
				},
			})
		case isTerraformVarFile(fullPath, autodetectConfig, allowedDirs):
			node.TFVars.HasFiles = true
			node.TFVars.Files = append(node.TFVars.Files, TFVarsFile{
				Name:         info.Name(),
				Env:          autodetectConfig.EnvMatcher.EnvName(info.Name()),
				IsGlobal:     autodetectConfig.EnvMatcher.IsGlobalVarFile(info.Name()),
				AbsolutePath: fullPath,
				Owner:        node,
			})

		}
	}

	return node, nil
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
