package autodetect

import (
	"path/filepath"
	"slices"

	"github.com/infracost/config/types"
)

type Node struct {
	Name            string
	AbsolutePath    string
	Children        []*Node
	ProjectType     types.ProjectType
	Terragrunt      TerragruntFlags
	Terraform       TerraformFlags
	TFVars          TFVarsFlags
	Depth           int
	Parent          *Node
	DependencyPaths []string
	// RawOptions is the directory-level seed blob returned by the plugin's IdentifyProjects,
	// threaded into IdentifyEnvironments. Empty when the plugin does not emit one.
	RawOptions []byte
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
	return n.ProjectType != types.ProjectTypeUnknown
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
	return n.ProjectType == types.ProjectTypeTerraform
}

func (n *Node) IsTerragrunt() bool {
	return n.ProjectType == types.ProjectTypeTerragrunt
}

func (n *Node) IsCloudFormation() bool {
	return n.ProjectType == types.ProjectTypeCloudFormation
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

// AssociatePiblingTFVarFiles grabs TFVars from pibling dirs. Pibling is the gender-neutral term for aunt/uncle (TFVars files are non-binary)
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

// ModifyTFVarFileEnvs modifies env using suitable directory names
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
