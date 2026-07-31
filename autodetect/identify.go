package autodetect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/infracost/config/plugin"
	"github.com/infracost/config/types"
	"gopkg.in/yaml.v3"
)

// IdentifyCloudFormationPath returns true if the given path is a cloudformation template
func IdentifyCloudFormationPath(path string) bool {
	if isOfCDKOrigin(path) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return identifyCloudFormationJSON(path)
	case ".yml", ".yaml":
		return identifyCloudFormationYAML(path)
	default:
		return false
	}
}

type identificationSniff struct {
	Service   string `yaml:"service"`
	Resources map[string]struct {
		Type string `json:"Type" yaml:"Type"`
	} `json:"Resources" yaml:"Resources"`
	AWSTemplateFormatVersion string `json:"AWSTemplateFormatVersion" yaml:"AWSTemplateFormatVersion"`
}

func (sniff *identificationSniff) IsCF() bool {
	if sniff.AWSTemplateFormatVersion != "" {
		return true
	}
	for _, v := range sniff.Resources {
		if !strings.HasPrefix(v.Type, "::") && strings.Contains(v.Type, "::") {
			return true
		}
	}
	return false
}

func identifyCloudFormationJSON(path string) bool {
	// #nosec G304
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var sniff identificationSniff
	if err := json.Unmarshal(content, &sniff); err != nil {
		return false
	}
	return sniff.IsCF()
}

func identifyCloudFormationYAML(path string) bool {
	// #nosec G304
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var sniff identificationSniff
	if err := yaml.Unmarshal(content, &sniff); err != nil {
		return false
	}

	return sniff.IsCF()
}

// isOfCDKOrigin returns true if the given path is a CDK sample, or synthesized template
// CDK projects are added elsewhere, so ignore them when looking for native CFN
func isOfCDKOrigin(path string) bool {
	return strings.Contains(path, "node_modules") || strings.Contains(path, "infracost.cdk.out")
}

func (b *treeBuilder) identifyDirectory(ctx context.Context, dir string) *plugin.IdentificationResult {
	if b.identifier != nil {
		return b.identifier.IdentifyDirectory(ctx, dir, b.singleFileMode, b.config.EnvNames)
	}

	return identifyDirectoryLocal(dir, b.singleFileMode)
}

// PathType returns the single project type detected for a project path, which may be either a
// directory or a single file (e.g. a CloudFormation template referenced directly). It returns
// types.ProjectTypeUnknown ("") when nothing can be identified. This is the standalone entry config
// generation uses to backfill a project's type when a config or user template emits a project
// without one (see FIX-495).
func PathType(ctx context.Context, identifier *plugin.Identifier, path string, singleFileMode bool, envNames []string) types.ProjectType {
	info, err := os.Stat(path)
	if err != nil {
		return types.ProjectTypeUnknown
	}
	if info.IsDir() {
		return DirectoryType(ctx, identifier, path, singleFileMode, envNames)
	}
	return fileType(ctx, identifier, path, singleFileMode, envNames)
}

// DirectoryType returns the single project type detected for a directory, using the plugin
// identifier when one is available and falling back to local file sniffing otherwise. It returns
// types.ProjectTypeUnknown ("") when nothing can be identified (including when the directory holds a
// mix of conflicting file types).
func DirectoryType(ctx context.Context, identifier *plugin.Identifier, dir string, singleFileMode bool, envNames []string) types.ProjectType {
	var result *plugin.IdentificationResult
	if identifier != nil {
		result = identifier.IdentifyDirectory(ctx, dir, singleFileMode, envNames)
	} else {
		result = identifyDirectoryLocal(dir, singleFileMode)
	}
	return directoryTypeFromResult(result)
}

// fileType identifies the type of a single file. With a plugin configured we ask the identifier
// about the file's parent directory and pick out this file's entry (falling back to the directory
// type); otherwise we sniff the file locally.
func fileType(ctx context.Context, identifier *plugin.Identifier, path string, singleFileMode bool, envNames []string) types.ProjectType {
	if identifier != nil {
		result := identifier.IdentifyDirectory(ctx, filepath.Dir(path), singleFileMode, envNames)
		if result == nil {
			return types.ProjectTypeUnknown
		}
		if t, ok := result.FileTypes[filepath.Base(path)]; ok && t != types.ProjectTypeUnknown {
			return t
		}
		return result.DirectoryType
	}
	return localFileType(path)
}

func localFileType(path string) types.ProjectType {
	lower := strings.ToLower(path)
	base := filepath.Base(path)
	switch {
	case strings.HasSuffix(lower, ".tf"), strings.HasSuffix(lower, ".tofu"),
		strings.HasSuffix(lower, ".tf.json"), strings.HasSuffix(lower, ".tofu.json"):
		return types.ProjectTypeTerraform
	case base == "terragrunt.hcl" || base == "terragrunt.hcl.json":
		return types.ProjectTypeTerragrunt
	case IdentifyCloudFormationPath(path):
		return types.ProjectTypeCloudFormation
	}
	return types.ProjectTypeUnknown
}

// directoryTypeFromResult collapses an IdentificationResult into a single project type. A directory
// type wins outright; otherwise the per-file types are used, but only when they all agree.
func directoryTypeFromResult(result *plugin.IdentificationResult) types.ProjectType {
	if result == nil {
		return types.ProjectTypeUnknown
	}
	if result.DirectoryType != types.ProjectTypeUnknown {
		return result.DirectoryType
	}

	var found types.ProjectType
	for _, t := range result.FileTypes {
		if t == types.ProjectTypeUnknown {
			continue
		}
		if found != types.ProjectTypeUnknown && found != t {
			return types.ProjectTypeUnknown
		}
		found = t
	}
	return found
}

func identifyDirectoryLocal(dir string, singleFileMode bool) *plugin.IdentificationResult {
	result := new(plugin.IdentificationResult)
	result.FileTypes = make(map[string]types.ProjectType)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	dirProjectTypes := make(map[types.ProjectType]struct{})

	for _, entry := range entries {

		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		switch {
		case !singleFileMode && (strings.HasSuffix(strings.ToLower(info.Name()), ".tf") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".tofu") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".tf.json") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".tofu.json")):
			dirProjectTypes[types.ProjectTypeTerraform] = struct{}{}
		case !singleFileMode && (info.Name() == "terragrunt.hcl" || info.Name() == "terragrunt.hcl.json"):
			dirProjectTypes[types.ProjectTypeTerragrunt] = struct{}{}
		case len(dirProjectTypes) == 0 && IdentifyCloudFormationPath(filepath.Join(dir, info.Name())):
			result.FileTypes[entry.Name()] = types.ProjectTypeCloudFormation
		}
	}

	if len(dirProjectTypes) > 0 {
		// file types don't matter if we have a directory type
		result.FileTypes = nil
		// if multiple project types are detected, we prioritize terraform over terragrunt
		if len(dirProjectTypes) > 1 {
			if _, ok := dirProjectTypes[types.ProjectTypeTerragrunt]; ok {
				result.DirectoryType = types.ProjectTypeTerragrunt
			} else if _, ok := dirProjectTypes[types.ProjectTypeCiscoStacks]; ok {
				result.DirectoryType = types.ProjectTypeCiscoStacks
			} else {
				result.DirectoryType = types.ProjectTypeTerraform
			}
		} else {
			for t := range dirProjectTypes {
				result.DirectoryType = t
				break
			}
		}
	}

	return result
}
