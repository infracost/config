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

// Environment is the plugin-agnostic view of a single environment (e.g. a dev/staging/prod
// variant of a project) returned by a plugin's IdentifyEnvironments RPC. It mirrors the proto
// Environment message but keeps the generated type out of the autodetect package, matching the
// boundary IdentificationResult already establishes.
type Environment struct {
	Name            string
	Path            string
	Files           []string
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

func (i *Identifier) IdentifyDirectory(ctx context.Context, dir string, singleFileMode bool) *IdentificationResult {
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
		if result.Directory && !singleFileMode {
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

// IdentifyEnvironments asks the plugin that owns projectType for the environments of the given
// project root (a directory previously returned by IdentifyProjects).
//
// The bool return distinguishes two cases the caller must treat differently:
//   - false: no plugin owns this type, or the owning plugin returned codes.Unimplemented (the
//     gRPC forward-compat signal that it doesn't implement this optional RPC). The caller should
//     route to its own format-specific fallback.
//   - true: a plugin answered authoritatively. The returned slice may be empty, which means "this
//     project genuinely has no variants" (yielding a single project) and must NOT trigger the
//     fallback.
func (i *Identifier) IdentifyEnvironments(ctx context.Context, dir string, projectType types.ProjectType) ([]Environment, bool) {
	for _, plugin := range i.plugins {
		pluginType := types.ProjectType(plugin.info.Name)
		if plugin.parserConfig.ConfigFileProjectType != nil {
			pluginType = types.ProjectType(*plugin.parserConfig.ConfigFileProjectType)
		}
		if pluginType != projectType {
			continue
		}

		result, err := plugin.parser.IdentifyEnvironments(ctx, &pb.IdentifyEnvironmentsRequest{Directory: dir})
		if err != nil || result == nil {
			// The RPC is optional: a plugin that doesn't implement it returns codes.Unimplemented,
			// which arrives here as a non-nil error. We treat that - and any other error - as "no
			// authoritative answer" so the caller falls back rather than failing the whole run,
			// mirroring the lenient handling in IdentifyDirectory above.
			return nil, false
		}

		environments := make([]Environment, 0, len(result.Environments))
		for _, e := range result.Environments {
			environments = append(environments, Environment{
				Name:            e.Name,
				Path:            e.Path,
				Files:           e.Files,
				DependencyPaths: e.DependencyPaths,
			})
		}
		return environments, true
	}

	return nil, false
}
