package autodetect

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/infracost/config/plugin"
	"github.com/infracost/config/types"
)

// treeBuilder carries the invariant inputs to the recursive directory walk so
// each recursive call only needs to pass through state that actually changes
// per frame (path, depth, parent).
type treeBuilder struct {
	identifier             *plugin.Identifier
	repoRoot               string
	config                 *Config
	allowedDirs            []string
	ignorePermissionErrors bool
	ignoreHiddenDirs       bool
}

func newTreeBuilder(identifier *plugin.Identifier, repoRoot string, config *Config, ignorePermissionErrors, ignoreHiddenDirs bool, allowedDirs ...string) *treeBuilder {
	return &treeBuilder{
		identifier:             identifier,
		repoRoot:               repoRoot,
		config:                 config,
		allowedDirs:            allowedDirs,
		ignorePermissionErrors: ignorePermissionErrors,
		ignoreHiddenDirs:       ignoreHiddenDirs,
	}
}

// build walks the tree starting at the builder's repoRoot.
func (b *treeBuilder) build(ctx context.Context) (*Node, error) {
	return b.buildSubtree(ctx, b.repoRoot, 0, nil)
}

func (b *treeBuilder) buildSubtree(ctx context.Context, path string, depth int, parent *Node) (*Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	resolvedPath, err := recursivelyResolveSymlink(path)
	if err != nil {
		if !b.ignorePermissionErrors || !errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("failed to resolve symlink: %w", err)
		}
	} else {
		path = resolvedPath
	}
	if !isPathAllowed(path, b.allowedDirs...) {
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
		if b.ignorePermissionErrors && errors.Is(err, fs.ErrPermission) {
			return node, nil
		}
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	idResult := b.identifyDirectory(ctx, path)
	if idResult != nil {
		if pt := idResult.DirectoryType; pt != types.ProjectTypeUnknown {

			for _, dep := range idResult.DependencyPaths {
				// dep is relative to "path", but we need it relative to the repo root
				// so we join it with "path" and then relativize it to the repo root
				absPath := filepath.Join(path, dep)
				relPath, err := filepath.Rel(b.repoRoot, absPath)
				if err != nil {
					continue
				}
				node.DependencyPaths = append(node.DependencyPaths, relPath)
			}

			node.ProjectType = pt
			switch pt {
			case types.ProjectTypeTerragrunt:
				node.Terragrunt.HasFiles = true
			case types.ProjectTypeTerraform:
				node.Terraform.HasFiles = true
			}
		} else {
			for fileName, fileType := range idResult.FileTypes {

				resolved, err := recursivelyResolveSymlink(filepath.Join(path, fileName))
				if err != nil {
					continue
				}
				if !isPathAllowed(resolved, b.allowedDirs...) {
					continue
				}
				node.Children = append(node.Children, &Node{
					Name:         fileName,
					AbsolutePath: resolved,
					Parent:       node,
					Depth:        depth + 1,
					ProjectType:  fileType,
				})
			}
		}
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
			if !isPathAllowed(resolved, b.allowedDirs...) {
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
			if b.ignoreHiddenDirs && strings.HasPrefix(info.Name(), ".") {
				continue
			}
			if depth+1 < b.config.MaxSearchDepth {
				childNode, err := b.buildSubtree(ctx, fullPath, depth+1, node)
				if err == nil {
					node.Children = append(node.Children, childNode)
				}
			}
			continue
		}

		// Unknown directories are treated as candidates for any known project
		// type. Terragrunt directories also accept terraform files because
		// terragrunt projects extend terraform.
		isTGContext := node.ProjectType == types.ProjectTypeTerragrunt || node.ProjectType == types.ProjectTypeUnknown
		isTFContext := isTGContext || node.ProjectType == types.ProjectTypeTerraform

		name := info.Name()
		switch {
		case isTGContext && isTerragruntFile(name):
			b.handleTerragruntFile(node, fullPath)
		case isTFContext && isTerraformFile(name):
			handleTerraformFile(node, fullPath)
		case isTFContext && isTerraformJSONFile(name):
			handleTerraformJSONFile(node, fullPath)
		case isTFContext && isTerraformVarFile(fullPath, b.config, b.allowedDirs):
			addTFVarFile(node, name, fullPath, b.config)
		}
	}

	return node, nil
}

func isTerragruntFile(name string) bool {
	return name == "terragrunt.hcl" || name == "terragrunt.hcl.json"
}

func isTerraformFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".tf") || strings.HasSuffix(lower, ".tofu")
}

func isTerraformJSONFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".tf.json") || strings.HasSuffix(lower, ".tofu.json")
}

func (b *treeBuilder) handleTerragruntFile(node *Node, fullPath string) {
	node.Terragrunt.HasFiles = true
	node.Terragrunt.LinkTFVars = b.config.LinkTFVarsToTerragrunt
	sniff, err := sniffTerragrunt(b.repoRoot, fullPath, b.allowedDirs...)
	if err != nil {
		return
	}
	node.Terragrunt.LocalOutsideTerraformSources = append(node.Terragrunt.LocalOutsideTerraformSources, sniff.Sources...)
	node.Terragrunt.IncludedOutsideTerragruntFiles = append(node.Terragrunt.IncludedOutsideTerragruntFiles, sniff.Includes...)
}

func handleTerraformFile(node *Node, fullPath string) {
	node.Terraform.HasFiles = true
	// #nosec G304
	src, err := os.ReadFile(fullPath)
	if err != nil {
		return
	}
	f, err := parseHCLFile(src, fullPath)
	if err != nil {
		return
	}
	sniff := sniffTerraform(fullPath, f)
	node.Terraform.HasBackend = node.Terraform.HasBackend || sniff.hasTerraformBackendBlock
	node.Terraform.HasProvider = node.Terraform.HasProvider || sniff.hasProviderBlock
	node.Terraform.LocalModuleSources = append(node.Terraform.LocalModuleSources, sniff.localModuleSources...)
}

func handleTerraformJSONFile(node *Node, fullPath string) {
	// #nosec G304
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return
	}
	f, err := parseHCLJSONFile(data, fullPath)
	node.Terraform.HasFiles = true
	if err != nil {
		return
	}
	sniff := sniffTerraform(fullPath, f)
	node.Terraform.HasBackend = node.Terraform.HasBackend || sniff.hasTerraformBackendBlock
	node.Terraform.HasProvider = node.Terraform.HasProvider || sniff.hasProviderBlock
	node.Terraform.LocalModuleSources = append(node.Terraform.LocalModuleSources, sniff.localModuleSources...)
}

func addTFVarFile(node *Node, name, fullPath string, config *Config) {
	node.TFVars.HasFiles = true
	node.TFVars.Files = append(node.TFVars.Files, TFVarsFile{
		Name:         name,
		Env:          config.EnvMatcher.EnvName(name),
		IsGlobal:     config.EnvMatcher.IsGlobalVarFile(name),
		AbsolutePath: fullPath,
		Owner:        node,
	})
}
