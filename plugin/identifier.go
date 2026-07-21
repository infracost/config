package plugin

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/infracost/config/types"
	pb "github.com/infracost/proto/gen/go/infracost/plugin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	// RawOptions is the plugin-authored, opaque parse-options blob (always JSON) for this
	// environment. The caller persists it under plugins.<name> in the config file and forwards it
	// verbatim into Parse; it is never interpreted here.
	RawOptions []byte
}

// AttributedVarFile is the plugin-agnostic view of a var file the caller has already attributed
// to a project (including cross-directory sibling/pibling association) and passes into the
// IdentifyEnvironments RPC. It mirrors the proto AttributedVarFile message but keeps the generated
// type out of the autodetect package. Terraform/Terragrunt only - see the proto docs.
type AttributedVarFile struct {
	// Path to the file, relative to the project directory; may contain "..".
	Path string
	// Env is the resolved environment label; empty when the file applies to all environments.
	Env string
	// IsGlobal is true when the file is a global/shared input rather than environment-specific.
	IsGlobal bool
}

// Close terminates every plugin subprocess held by the Identifier. Safe to
// call once; further use of the Identifier after Close is not supported.
func (i *Identifier) Close() {
	for _, p := range i.plugins {
		p.Close()
	}
	i.plugins = nil
}

func (i *Identifier) IdentifyDirectory(ctx context.Context, dir string, singleFileMode bool, envNames []string) *IdentificationResult {
	var output *IdentificationResult
	for _, plugin := range i.plugins {
		pluginType := types.ProjectType(plugin.info.Name)
		if plugin.parserConfig.ConfigFileProjectType != nil {
			pluginType = types.ProjectType(*plugin.parserConfig.ConfigFileProjectType)
		}
		result, err := plugin.parser.IdentifyProjects(ctx, &pb.IdentifyProjectsRequest{
			Directory:        dir,
			EnvironmentNames: envNames,
		})
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
//
// A non-nil error means the owning plugin implements the RPC but failed to answer (transport
// error, timeout, panic, malformed response). This is deliberately distinct from codes.Unimplemented:
// an unimplemented RPC is the expected forward-compat signal and yields (nil, false, nil) so the
// caller falls back, whereas a genuine failure is returned so the caller can abort rather than
// silently degrading to fallback behaviour that produces different results.
//
// attributedFiles carries the var files the caller has already attributed to this project so the
// owning plugin can reproduce that attribution rather than re-derive it. It is a Terraform/Terragrunt
// migration aid; other plugins ignore it.
func (i *Identifier) IdentifyEnvironments(ctx context.Context, dir string, projectType types.ProjectType, attributedFiles []AttributedVarFile, envNames []string) ([]Environment, bool, error) {
	for _, plugin := range i.plugins {
		pluginType := types.ProjectType(plugin.info.Name)
		if plugin.parserConfig.ConfigFileProjectType != nil {
			pluginType = types.ProjectType(*plugin.parserConfig.ConfigFileProjectType)
		}
		if pluginType != projectType {
			continue
		}

		pbAttributedFiles := make([]*pb.AttributedVarFile, 0, len(attributedFiles))
		for _, f := range attributedFiles {
			pbAttributedFiles = append(pbAttributedFiles, &pb.AttributedVarFile{
				Path:     f.Path,
				Env:      f.Env,
				IsGlobal: f.IsGlobal,
			})
		}

		result, err := plugin.parser.IdentifyEnvironments(ctx, &pb.IdentifyEnvironmentsRequest{
			Directory:        dir,
			AttributedFiles:  pbAttributedFiles,
			EnvironmentNames: envNames,
		})
		if err != nil {
			// The RPC is optional: a plugin that doesn't implement it returns codes.Unimplemented.
			// That is the expected forward-compat signal, so we treat it as "no authoritative answer"
			// and let the caller fall back. Any other error means a plugin that DOES implement the RPC
			// failed to answer - we surface it so the caller can abort rather than silently degrade to
			// fallback behaviour that would produce a different result.
			if status.Code(err) == codes.Unimplemented {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("plugin %q failed to identify environments for %q: %w", pluginType, dir, err)
		}
		if result == nil {
			return nil, false, nil
		}

		environments := make([]Environment, 0, len(result.Environments))
		for _, e := range result.Environments {
			environments = append(environments, Environment{
				Name:            e.Name,
				Path:            e.Path,
				Files:           e.Files,
				DependencyPaths: e.DependencyPaths,
				RawOptions:      e.RawOptions,
			})
		}
		return environments, true, nil
	}

	return nil, false, nil
}
