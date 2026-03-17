package autodetect

type Project struct {
	Name              string
	Path              string
	TerraformVarFiles []string
	DependencyPaths   []string
	Env               string
	Type              ProjectType
	Metadata          map[string]string
}

type ProjectType string

const (
	ProjectTypeUnknown        ProjectType = ""
	ProjectTypeTerraform      ProjectType = "terraform"
	ProjectTypeTerragrunt     ProjectType = "terragrunt"
	ProjectTypeCloudFormation ProjectType = "cloudformation"
	ProjectTypeCiscoStacks    ProjectType = "cisco_stacks"
)

type RootModule struct {
	Path     string
	Projects []Project
	Type     ProjectType
}
