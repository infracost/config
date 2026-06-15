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

// armSchemaMarker appears in the $schema URL of every ARM deployment template
// (resource-group / subscription / management-group / tenant scope all use a
// URL of the form
// https://schema.management.azure.com/schemas/<version>/deploymentTemplate.json#).
const armSchemaMarker = "deploymentTemplate.json"

// armResourceTypeMarker is the prefix every ARM resource type carries (e.g.
// "Microsoft.Storage/storageAccounts"). Used as a fallback signal when the
// $schema field is absent, e.g. nested templates.
const armResourceTypeMarker = "Microsoft."

// IdentifyARMPath returns true if the given path is an ARM JSON deployment
// template. Bicep is transpiled to ARM JSON before parsing, so only .json is
// considered here. ARM also has a parser plugin; this inline check keeps
// autodetection working when no plugin identifier is supplied (parity with
// CloudFormation).
func IdentifyARMPath(path string) bool {
	if strings.ToLower(filepath.Ext(path)) != ".json" {
		return false
	}
	return identifyARMJSON(path)
}

// armIdentificationSniff captures just enough of an ARM template to distinguish
// it from other JSON. Resources can be either a list (the original ARM template
// language) or a map keyed by symbolic name (template language v2), so we
// deserialize into json.RawMessage and inspect both shapes.
type armIdentificationSniff struct {
	Schema    string          `json:"$schema"`
	Resources json.RawMessage `json:"resources"`
}

func (sniff *armIdentificationSniff) IsARM() bool {
	if strings.Contains(sniff.Schema, armSchemaMarker) {
		return true
	}
	if len(sniff.Resources) == 0 {
		return false
	}
	// Try list shape: [{"type": "Microsoft.X/Y", ...}, ...]
	var asList []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(sniff.Resources, &asList); err == nil {
		for _, r := range asList {
			if strings.HasPrefix(r.Type, armResourceTypeMarker) {
				return true
			}
		}
	}
	// Try map shape: {"name": {"type": "Microsoft.X/Y", ...}, ...}
	var asMap map[string]struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(sniff.Resources, &asMap); err == nil {
		for _, r := range asMap {
			if strings.HasPrefix(r.Type, armResourceTypeMarker) {
				return true
			}
		}
	}
	return false
}

func identifyARMJSON(path string) bool {
	// #nosec G304
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var sniff armIdentificationSniff
	if err := json.Unmarshal(content, &sniff); err != nil {
		return false
	}
	return sniff.IsARM()
}

func (b *treeBuilder) identifyDirectory(ctx context.Context, dir string) *plugin.IdentificationResult {
	if b.identifier != nil {
		return b.identifier.IdentifyDirectory(ctx, dir)
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
		case strings.HasSuffix(strings.ToLower(info.Name()), ".tf"),
			strings.HasSuffix(strings.ToLower(info.Name()), ".tofu"),
			strings.HasSuffix(strings.ToLower(info.Name()), ".tf.json"),
			strings.HasSuffix(strings.ToLower(info.Name()), ".tofu.json"):
			dirProjectTypes[types.ProjectTypeTerraform] = struct{}{}
		case info.Name() == "terragrunt.hcl" || info.Name() == "terragrunt.hcl.json":
			dirProjectTypes[types.ProjectTypeTerragrunt] = struct{}{}
		case len(dirProjectTypes) == 0 && IdentifyARMPath(filepath.Join(dir, info.Name())):
			result.FileTypes[entry.Name()] = types.ProjectTypeARM
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
