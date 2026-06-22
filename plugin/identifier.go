package plugin

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/infracost/config/types"
	pb "github.com/infracost/proto/gen/go/infracost/plugin"
)

type Identifier struct {
	plugins []*Plugin
}

func CreateIdentifier(ctx context.Context, pluginDir string) (*Identifier, error) {
	manager, err := NewManager(pluginDir)
	if err != nil {
		return nil, err
	}
	plugins, err := manager.List(ctx)
	if err != nil {
		return nil, err
	}
	// order plugins by identification priority
	slices.SortFunc(plugins, func(a, b *Plugin) int {
		return int(b.parserConfig.IdentificationPriority) - int(a.parserConfig.IdentificationPriority)
	})
	return &Identifier{plugins: plugins}, nil
}

type IdentificationResult struct {
	// DirectoryType is the highest-priority plugin that claims the whole
	// directory as a single project. Directory-level claims remain
	// winner-takes-all: terraform/terragrunt are mutually exclusive by design
	// (a terragrunt dir always contains .tf), so only one wins.
	DirectoryType types.ProjectType
	// FileTypes maps a file (relative to the directory) to the plugin that
	// claims it, merged across all plugins. Populated even when a DirectoryType
	// exists, so file-level parsers (e.g. kubernetes manifests, app code)
	// surface alongside a whole-directory parser like terraform — this is what
	// lets a single scan see everything in a mixed directory.
	FileTypes       map[string]types.ProjectType
	DependencyPaths []string
}

// Close terminates every plugin subprocess held by the Identifier. Safe to
// call once; further use of the Identifier after Close is not supported.
func (i *Identifier) Close() {
	for _, p := range i.plugins {
		p.Close()
	}
	i.plugins = nil
}

// IdentifyDirectory asks every loaded parser plugin (in identification-priority
// order) what it finds in dir, accumulating file-level claims rather than
// stopping at the first match. A directory can be claimed as a whole-directory
// project by one plugin (DirectoryType, winner-takes-all by priority) and
// simultaneously carry file-level projects from other plugins (FileTypes) —
// e.g. a terraform directory that also contains kubernetes manifests and
// app-code call sites. Returns nil only when nothing is identified.
func (i *Identifier) IdentifyDirectory(ctx context.Context, dir string) *IdentificationResult {
	result := &IdentificationResult{FileTypes: map[string]types.ProjectType{}}

	for _, plugin := range i.plugins {
		pluginType := types.ProjectType(plugin.info.Name)
		if plugin.parserConfig.ConfigFileProjectType != nil {
			pluginType = types.ProjectType(*plugin.parserConfig.ConfigFileProjectType)
		}
		res, err := plugin.parser.IdentifyProjects(ctx, &pb.IdentifyProjectsRequest{Directory: dir})
		if err != nil || res == nil {
			continue
		}
		result.DependencyPaths = append(result.DependencyPaths, res.DependencyPaths...)

		if res.Directory {
			// Whole-directory claim: first (highest-priority) wins, matching the
			// pre-existing behaviour. Keep scanning for file-level claims though.
			if result.DirectoryType == types.ProjectTypeUnknown {
				result.DirectoryType = pluginType
			}
			continue
		}
		for _, file := range res.Files {
			if filepath.IsAbs(file) {
				rel, err := filepath.Rel(dir, file)
				if err != nil {
					continue
				}
				file = rel
			}
			// plugins are visited highest-priority first, so the first claim wins.
			if _, exists := result.FileTypes[file]; !exists {
				result.FileTypes[file] = pluginType
			}
		}
	}

	if len(result.FileTypes) == 0 {
		result.FileTypes = nil
	}
	if result.DirectoryType == types.ProjectTypeUnknown && result.FileTypes == nil {
		return nil
	}
	return result
}
