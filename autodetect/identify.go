package autodetect

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/infracost/config/plugin"
	projecttype "github.com/infracost/go-proto/pkg/project"
	"gopkg.in/yaml.v3"
)

// maxIdentifyBytes caps how much of a candidate file identification reads.
// CloudFormation templates are hand-authored documents rarely beyond a few
// hundred KB; anything bigger than this is not worth sniffing whole.
const maxIdentifyBytes = 10 * 1024 * 1024

// markerFormatVersion and markerResourceTypeSep are byte markers, at least
// one combination of which must appear in any document IsCF accepts: either
// an AWSTemplateFormatVersion key, or a Resources map holding a Type like
// "AWS::EC2::Instance" (the "::" separator). Scanning for them before
// unmarshaling rejects nearly all non-CloudFormation JSON/YAML — package
// locks, OpenAPI specs, CI config — without paying for a parse. The key scans
// fold case because encoding/json matches keys case-insensitively. (A
// document spelling a marker via escape sequences would slip past the scan —
// real templates never do that.)
const (
	markerFormatVersion   = "awstemplateformatversion"
	markerResources       = "resources"
	markerResourceTypeSep = "::"
)

func maybeCloudFormationContent(content []byte) bool {
	if containsFold(content, markerFormatVersion) {
		return true
	}
	return containsFold(content, markerResources) && bytes.Contains(content, []byte(markerResourceTypeSep))
}

// IdentifyCloudFormationPath returns true if the given path is a cloudformation template
func IdentifyCloudFormationPath(path string) bool {
	if isOfCDKOrigin(path) {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".yml", ".yaml":
	default:
		return false
	}

	content := readCapped(path, maxIdentifyBytes)
	if content == nil || !maybeCloudFormationContent(content) {
		return false
	}

	if strings.ToLower(filepath.Ext(path)) == ".json" {
		return identifyCloudFormationJSON(content)
	}
	return identifyCloudFormationYAML(content)
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

func identifyCloudFormationJSON(content []byte) bool {
	var sniff identificationSniff
	if err := json.Unmarshal(content, &sniff); err != nil {
		return false
	}
	return sniff.IsCF()
}

func identifyCloudFormationYAML(content []byte) bool {
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
	return b.identifier.IdentifyDirectory(ctx, dir, b.repoRoot, b.singleFileMode, b.config.EnvNames)
}

// PathType returns the single project type detected for a project path, which may be either a
// directory or a single file (e.g. a CloudFormation template referenced directly). It returns
// projecttype.Unknown ("") when nothing can be identified or identifier is nil. Config generation
// uses it to backfill a project's type when a config or user template emits a project without one.
func PathType(ctx context.Context, identifier *plugin.Identifier, path, repoRoot string, singleFileMode bool, envNames []string) projecttype.Type {
	if identifier == nil {
		return projecttype.Unknown
	}
	info, err := os.Stat(path)
	if err != nil {
		return projecttype.Unknown
	}
	if info.IsDir() {
		return DirectoryType(ctx, identifier, path, repoRoot, singleFileMode, envNames)
	}
	return fileType(ctx, identifier, path, repoRoot, singleFileMode, envNames)
}

// DirectoryType returns the single project type the plugins detect for a directory. It returns
// projecttype.Unknown ("") when nothing can be identified (including when the directory holds a
// mix of conflicting file types) or identifier is nil.
func DirectoryType(ctx context.Context, identifier *plugin.Identifier, dir, repoRoot string, singleFileMode bool, envNames []string) projecttype.Type {
	if identifier == nil {
		return projecttype.Unknown
	}
	return directoryTypeFromResult(identifier.IdentifyDirectory(ctx, dir, repoRoot, singleFileMode, envNames))
}

// fileType identifies the type of a single file by asking the plugins about its parent directory
// and picking out this file's entry, falling back to the directory type.
func fileType(ctx context.Context, identifier *plugin.Identifier, path, repoRoot string, singleFileMode bool, envNames []string) projecttype.Type {
	result := identifier.IdentifyDirectory(ctx, filepath.Dir(path), repoRoot, singleFileMode, envNames)
	if result == nil {
		return projecttype.Unknown
	}
	if t, ok := result.FileTypes[filepath.Base(path)]; ok && t != projecttype.Unknown {
		return t
	}
	return result.DirectoryType
}

// directoryTypeFromResult collapses an IdentificationResult into a single project type. A directory
// type wins outright; otherwise the per-file types are used, but only when they all agree.
func directoryTypeFromResult(result *plugin.IdentificationResult) projecttype.Type {
	if result == nil {
		return projecttype.Unknown
	}
	if result.DirectoryType != projecttype.Unknown {
		return result.DirectoryType
	}

	var found projecttype.Type
	for _, t := range result.FileTypes {
		if t == projecttype.Unknown {
			continue
		}
		if found != projecttype.Unknown && found != t {
			return projecttype.Unknown
		}
		found = t
	}
	return found
}
