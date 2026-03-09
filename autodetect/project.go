package autodetect

type Project struct {
	Name              string
	Path              string
	TerraformVarFiles []string
	DependencyPaths   []string
	Env               string
	Type              ProjectType
}

type ProjectType string

const (
	ProjectTypeUnknown        ProjectType = ""
	ProjectTypeTerraform      ProjectType = "terraform"
	ProjectTypeTerragrunt     ProjectType = "terragrunt"
	ProjectTypeCloudFormation ProjectType = "cloudformation"
)

type RootModule struct {
	Path     string
	Projects []Project
	Type     ProjectType
}
