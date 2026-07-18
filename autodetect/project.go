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
	// RawOptions is the plugin-authored parse-options blob (always JSON) for this project, carried
	// from the IdentifyEnvironments RPC. It is only set when a plugin returned environments
	// authoritatively; otherwise it is nil and generation sideloads the locally-derived var files
	// into the plugins blob instead.
	RawOptions []byte
}

type RootModule struct {
	Path     string
	Projects []Project
	Type     types.ProjectType
}
