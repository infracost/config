package autodetect

import (
	"encoding/json"

	"github.com/infracost/config/types"
)

// BACKWARDS-COMPATIBILITY SHIM.
//
// This whole file exists to keep plugins that predate the raw_options blob working. Once every
// parser plugin emits its own raw_options from IdentifyProjects/IdentifyEnvironments, delete this
// file and the calls to resolveRawOptions in search.go.
//
// When a plugin does not return a raw_options blob, we synthesise one from the data config
// derived itself (the attributed var files and the resolved environment) so the persisted blob
// matches what the callers historically built from the config file. That deliberately duplicates
// a subset of the plugin's option schema here; it is isolated in this file so it can be removed
// wholesale later.

// terraformRawOptions mirrors the subset of the terraform-family plugin options that config can
// derive on its own. The JSON tags MUST match the plugin's options.Options wire shape.
type terraformRawOptions struct {
	TfVarsFiles []string `json:"tfVarsFiles,omitempty"`
	Workspace   string   `json:"workspace,omitempty"`
}

// resolveRawOptions returns the blob to persist for a project: the plugin's own blob when it
// provided one (either per-environment or the directory-level seed), otherwise a backfilled blob
// derived from config's own attribution.
func resolveRawOptions(pluginRawOptions, seedRawOptions []byte, projectType types.ProjectType, varFiles []string, workspace string) []byte {
	if len(pluginRawOptions) > 0 {
		return pluginRawOptions
	}
	if len(seedRawOptions) > 0 {
		return seedRawOptions
	}
	return backfillRawOptions(projectType, varFiles, workspace)
}

// backfillRawOptions builds a raw_options blob from config-derived data for plugins that don't
// emit one. Only the terraform family consumes the shape it produces; other types get no blob.
func backfillRawOptions(projectType types.ProjectType, varFiles []string, workspace string) []byte {
	switch projectType {
	case types.ProjectTypeTerraform, types.ProjectTypeTerragrunt, types.ProjectTypeCiscoStacks:
	default:
		return nil
	}

	if len(varFiles) == 0 && workspace == "" {
		return nil
	}

	blob, err := json.Marshal(terraformRawOptions{TfVarsFiles: varFiles, Workspace: workspace})
	if err != nil {
		return nil
	}
	return blob
}
