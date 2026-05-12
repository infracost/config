package autodetect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

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

// IdentifyARMPath returns true if the given path is an ARM deployment
// template. ARM templates are JSON — Bicep input is transpiled to ARM
// JSON by callers before the parser sees it (mirrors the CDK→CFN
// pattern). The schema URL and the `resources[].type` shape are what
// distinguish ARM JSON from CloudFormation JSON.
func IdentifyARMPath(path string) bool {
	if strings.ToLower(filepath.Ext(path)) != ".json" {
		return false
	}
	return identifyARMJSON(path)
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

// armSchemaMarker appears in the $schema URL of every ARM deployment
// template (resource-group / subscription / management-group / tenant
// scope all use a URL of the form
// https://schema.management.azure.com/schemas/<version>/deploymentTemplate.json#).
const armSchemaMarker = "deploymentTemplate.json"

// armResourceTypeMarker is the prefix every ARM resource type carries
// (e.g. "Microsoft.Storage/storageAccounts"). Used as a fallback signal
// when the $schema field is absent, e.g. nested templates.
const armResourceTypeMarker = "Microsoft."

// armIdentificationSniff captures just enough of an ARM template to
// distinguish it from other JSON. Resources can be either a list (the
// original ARM template language) or a map keyed by symbolic name
// (template language v2), so we deserialize into json.RawMessage and
// inspect both shapes.
type armIdentificationSniff struct {
	Schema     string          `json:"$schema"`
	ContentVer string          `json:"contentVersion"`
	Resources  json.RawMessage `json:"resources"`
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
