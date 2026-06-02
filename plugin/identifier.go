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
	DirectoryType   types.ProjectType
	FileTypes       map[string]types.ProjectType
	DependencyPaths []string
}

func (i *Identifier) IdentifyDirectory(ctx context.Context, dir string) *IdentificationResult {
	var output *IdentificationResult
	for _, plugin := range i.plugins {
		pluginType := types.ProjectType(plugin.info.Name)
		if plugin.parserConfig.ConfigFileProjectType != nil {
			pluginType = types.ProjectType(*plugin.parserConfig.ConfigFileProjectType)
		}
		result, err := plugin.parser.IdentifyProjects(ctx, &pb.IdentifyProjectsRequest{Directory: dir})
		if err != nil || result == nil {
			continue
		}
		if result.Directory {
			return &IdentificationResult{
				DirectoryType:   pluginType,
				FileTypes:       nil,
				DependencyPaths: result.DependencyPaths,
			}
		}
		if len(result.Files) > 0 && (output == nil || output.DirectoryType == types.ProjectTypeUnknown) {
			m := make(map[string]types.ProjectType, len(result.Files))
			for _, file := range result.Files {
				if filepath.IsAbs(file) {
					file, err = filepath.Rel(dir, file)
					if err != nil {
						continue
					}
				}
				m[file] = pluginType
			}
			output = &IdentificationResult{
				DirectoryType:   "",
				FileTypes:       m,
				DependencyPaths: result.DependencyPaths,
			}
		}
	}
	return output
}
