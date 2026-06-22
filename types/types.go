// Package types holds value types shared by the root config package and its
// child packages. Keeping the canonical definitions here lets child packages
// (e.g. plugin) reference them without creating an import cycle back through
// the root package.
package types

type ProjectType string

const (
	ProjectTypeUnknown        ProjectType = ""
	ProjectTypeTerraform      ProjectType = "terraform"
	ProjectTypeTerragrunt     ProjectType = "terragrunt"
	ProjectTypeCloudFormation ProjectType = "cloudformation"
	ProjectTypeCDKTypeScript  ProjectType = "cdk_typescript"
	ProjectTypeCDKJavaScript  ProjectType = "cdk_javascript"
	ProjectTypeCDKPython      ProjectType = "cdk_python"
	ProjectTypeCiscoStacks    ProjectType = "cisco_stacks"
	ProjectTypeAtmos          ProjectType = "atmos"
)
