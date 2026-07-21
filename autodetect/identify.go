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
		case !b.singleFileMode && (strings.HasSuffix(strings.ToLower(info.Name()), ".tf") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".tofu") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".tf.json") ||
			strings.HasSuffix(strings.ToLower(info.Name()), ".tofu.json")):
			dirProjectTypes[types.ProjectTypeTerraform] = struct{}{}
		case !b.singleFileMode && (info.Name() == "terragrunt.hcl" || info.Name() == "terragrunt.hcl.json"):
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
