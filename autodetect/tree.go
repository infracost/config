package autodetect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

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
	singleFileMode         bool

	// tfTasks accumulates terraform files discovered during the (single-threaded)
	// walk. They are parsed in parallel after the walk completes, since HCL
	// parsing dominates autodetect time on large repos.
	tfTasks []tfParseTask
}

// tfParseTask is a terraform file whose HCL parse is deferred to the parallel
// phase. The owning node already has Terraform.HasFiles set during the walk;
// only the parse-derived flags are filled in later.
type tfParseTask struct {
	node   *Node
	path   string
	isJSON bool
}

// Block keywords that the terraform sniff looks for. A file containing none of
// these as a byte substring cannot contribute a provider/backend/module block,
// so its HCL parse can be skipped (an over-approximation: it may parse a few
// extra files, but never skips one that matters).
var (
	kwProvider = []byte("provider")
	kwBackend  = []byte("backend")
	kwModule   = []byte("module")
)

func newTreeBuilder(identifier *plugin.Identifier, repoRoot string, config *Config, ignorePermissionErrors, ignoreHiddenDirs, singleFileMode bool, allowedDirs ...string) *treeBuilder {
	return &treeBuilder{
		identifier:             identifier,
		repoRoot:               repoRoot,
		config:                 config,
		allowedDirs:            allowedDirs,
		ignorePermissionErrors: ignorePermissionErrors,
		ignoreHiddenDirs:       ignoreHiddenDirs,
		singleFileMode:         singleFileMode,
	}
}

// build walks the tree starting at the builder's repoRoot, then parses the
// discovered terraform files in parallel.
func (b *treeBuilder) build(ctx context.Context) (*Node, error) {
	root, err := b.buildSubtree(ctx, b.repoRoot, 0, nil)
	if err != nil {
		return nil, err
	}
	if err := b.parseTerraformTasks(ctx); err != nil {
		return nil, err
	}
	return root, nil
}

// parseTerraformTasks parses all terraform files collected during the walk
// using a worker pool, then folds the sniff results back onto their nodes
// single-threaded (so node mutation stays race-free). It returns ctx.Err() if
// the context was cancelled, so a partial tree is never returned as success.
func (b *treeBuilder) parseTerraformTasks(ctx context.Context) error {
	if len(b.tfTasks) == 0 {
		return nil
	}
	defer func() { b.tfTasks = nil }()

	results := make([]terraformSniff, len(b.tfTasks))

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > len(b.tfTasks) {
		workers = len(b.tfTasks)
	}

	var next int64 = -1
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(atomic.AddInt64(&next, 1))
				if i >= len(b.tfTasks) {
					return
				}
				if ctx.Err() != nil {
					return
				}
				results[i] = sniffTerraformFile(b.tfTasks[i].path, b.tfTasks[i].isJSON)
			}
		}()
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return err
	}

	for i, task := range b.tfTasks {
		sniff := results[i]
		task.node.Terraform.HasBackend = task.node.Terraform.HasBackend || sniff.hasTerraformBackendBlock
		task.node.Terraform.HasProvider = task.node.Terraform.HasProvider || sniff.hasProviderBlock
		task.node.Terraform.LocalModuleSources = append(task.node.Terraform.LocalModuleSources, sniff.localModuleSources...)
	}
	return nil
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
		if pt := idResult.DirectoryType; pt != types.ProjectTypeUnknown && !b.singleFileMode {

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
			if b.singleFileMode {
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

		if b.singleFileMode {
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
			node.Terraform.HasFiles = true
			b.tfTasks = append(b.tfTasks, tfParseTask{node: node, path: fullPath, isJSON: false})
		case isTFContext && isTerraformJSONFile(name):
			node.Terraform.HasFiles = true
			b.tfTasks = append(b.tfTasks, tfParseTask{node: node, path: fullPath, isJSON: true})
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

// sniffTerraformFile reads and sniffs a single terraform (or terraform JSON)
// file. Safe to call concurrently: it only reads the file and returns a value.
func sniffTerraformFile(fullPath string, isJSON bool) terraformSniff {
	// #nosec G304
	src, err := os.ReadFile(fullPath)
	if err != nil {
		return terraformSniff{}
	}

	// Skip the (expensive) parse for files that cannot contain a
	// provider/backend/module block. Never a false skip: the sniff only ever
	// reports those blocks, whose keywords must appear as substrings, including
	// in .tf.json where they are the object keys.
	if !bytes.Contains(src, kwProvider) && !bytes.Contains(src, kwBackend) && !bytes.Contains(src, kwModule) {
		return terraformSniff{}
	}

	parse := parseHCLFile
	if isJSON {
		parse = parseHCLJSONFile
	}
	f, err := parse(src, fullPath)
	if err != nil {
		return terraformSniff{}
	}
	return sniffTerraform(fullPath, f)
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
