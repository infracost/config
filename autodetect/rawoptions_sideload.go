package autodetect

import (
	"encoding/json"

	"github.com/infracost/config/types"
)

// The plugins author their own raw_options blob during identification (per-environment from
// IdentifyEnvironments, or a directory-level seed from IdentifyProjects) and config persists it
// verbatim under plugins.<name>. The one exception is a terraform-family project's var files:
// tfVarsFiles depends on the cross-directory (sibling/pibling) attribution that lives in config's
// tree passes, which the plugins do not have. So config sideloads just that one field into the blob.
//
// This is a deliberate, minimal special case - it only ever sets tfVarsFiles and leaves everything
// else the plugin (or a user) put in the blob untouched, so the blob stays extensible. It is
// expected to go away once IdentifyAllProjects moves the multi-directory attribution plugin-side.

// resolveRawOptions returns the blob to persist for a project. It starts from the blob the plugin
// authored - the per-environment blob from IdentifyEnvironments, else the directory-level seed from
// IdentifyProjects - and, for terraform-family projects, sideloads config's attributed var files
// into it without disturbing anything else the plugin produced.
func resolveRawOptions(pluginRawOptions, seedRawOptions []byte, projectType types.ProjectType, varFiles []string) []byte {
	base := pluginRawOptions
	if len(base) == 0 {
		base = seedRawOptions
	}
	return sideloadTFVarsFiles(base, projectType, varFiles)
}

// sideloadTFVarsFiles merges config's attributed var files into a terraform-family blob under the
// tfVarsFiles key, preserving every other key. Non-terraform-family types, and terraform-family
// projects with no attributed var files, are returned unchanged.
func sideloadTFVarsFiles(blob []byte, projectType types.ProjectType, varFiles []string) []byte {
	switch projectType {
	case types.ProjectTypeTerraform, types.ProjectTypeTerragrunt, types.ProjectTypeCiscoStacks:
	default:
		return blob
	}

	if len(varFiles) == 0 {
		return blob
	}

	options := map[string]any{}
	if len(blob) > 0 {
		if err := json.Unmarshal(blob, &options); err != nil {
			// don't clobber a blob we can't parse; leave it as the plugin/user authored it.
			return blob
		}
	}

	options["tfVarsFiles"] = varFiles

	merged, err := json.Marshal(options)
	if err != nil {
		return blob
	}

	return merged
}
