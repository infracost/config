# Config

This repo handles the parsing, templating, and generation of Infracost repository configuration and usage files.

## Config files

Infracost configuration files are YAML files that define:

- Which projects exists in the repository, where they are located, which dependencies they should use (e.g. tfvars files), and what _type_ they are (e.g. Terraform, CloudFormation, etc).
- How Infracost should run on the repository e.g. the currency to use.

They MUST be placed in the root of the repository and named `infracost.yml`.

### Versions

There are three supported versions of the config file. All are currently backward compatible.

- `0.1` - The initial version of the config file. This version is defined and used by the original Infracost CLI.
- `0.2` - The second version of the config file. This version is defined and has been used by the Infracost Cloud Platform.
- `0.3` - The third version of the config file. This version groups configuration items into sections and is fully backward compatible with the previous versions. This version will be used by the next version of the Infracost CLI, and the ICP going forward.

## Templating

The config file supports templating using the [Go template syntax](https://pkg.go.dev/text/template). This allows you to use variables and functions in your config file, which can be useful for generating dynamic config files based on your repository structure.

If you wish to use a templated config file, you should name it `infracost.yml.tmpl`.

The following variables are available in the template:

| Name | Description |
| ---- | ----------- |
| `RepoName` | The name of the repository. |
| `Branch` | The name of the branch being analyzed. |
| `BaseBranch` | The name of the base branch being compared against. |
| `DetectedProjects` | A list of projects detected in the repository. See below. |
| `DetectedRootModules` | A list of root modules detected in the repository. A root module is a project that can be used by multiple environments. See below. |

A detected project has the following properties:

| Name | Description |
| ---- | ----------- |
|	`Name`              | The name of the project. This should be unique across the repository.
|	`Path`              | The path to the project relative to the root of the repository.
|	`TerraformVarFiles` | List of var files that should be used when running Infracost on the project. 
|	`DependencyPaths`   | List of glob pattern paths which should be considered dependencies of the project.
|	`Env`               | Environment name
|	`Type`              | Project type e.g. terraform, cloudformation, etc.

A detected root module has the following properties:

| Name | Description |
| ---- | ----------- |
|	`Path`              | The path to the project relative to the root of the repository.
|	`Type`              | Project type e.g. terraform, cloudformation, etc.
| `Projects` | List of projects which use this root module. See above. |

## Autodetect

Autodetect is used to automatically find IaC projects within a repository (or simply a directory). This is useful for repositories that have a large number of projects, or for repositories that have a dynamic structure.

Autodetect can be used when no config is available, or it can be used in conjuction with the template. When used with the template, the detected projects and root modules can be used to generate a config file that is tailored to the repository structure. This allows you to have a dynamic config file that can adapt to changes in the repository structure without having to manually update the config file.

## Usage Files

Infracost usage files are YAML files that define the usage amounts of resources in a project. These have associated costs that could not be detected from the code alone. For example, the number of hours a resource is expected to run for, or the amount of data transfer expected.
