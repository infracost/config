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
}

type RootModule struct {
	Path     string
	Projects []Project
	Type     types.ProjectType
}
