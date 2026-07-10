package autodetect

import "github.com/infracost/config/types"

type Project struct {
	Name              string
	Path              string
	TerraformVarFiles []string
	DependencyPaths   []string
	Env               string
	Type              types.ProjectType
	Metadata          map[string]string
	// RawOptions is the plugin-specific, opaque parse-options blob (JSON) for this project. It is
	// either what the plugin returned from IdentifyEnvironments/IdentifyProjects, or - for plugins
	// that don't yet emit one - a value backfilled from the data config derived itself (see
	// rawoptions_backfill.go).
	RawOptions []byte
}

type RootModule struct {
	Path     string
	Projects []Project
	Type     types.ProjectType
}
