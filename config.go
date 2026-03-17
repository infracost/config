package config

import (
	"github.com/infracost/config/cdk"
	"gopkg.in/yaml.v3"
)

const CurrentVersion = "0.3"

var legacyVersions = []string{
	"0.1", // The 0.1 version of the config file was used in the original CLI.
	"0.2", // The 0.2 version of the config file was used in the original version of the Infracost Cloud Platform.
}

type ConfigBase struct {
	Version string `yaml:"version"`
}

// this handles the parameters that were supported in 0.1 and 0.2 but are not supported in 0.3. This allows us to parse legacy config files.
type ConfigWithLegacySupport struct {
	ConfigBase              `yaml:",inline"`
	Currency                string                      `yaml:"currency,omitempty"`
	UsageFilePath           string                      `yaml:"usage_file,omitempty"`
	TerraformRegexSourceMap []TerraformRegexSource      `yaml:"terraform_source_map,omitempty"`
	TerraformCloudHost      string                      `yaml:"terraform_cloud_host,omitempty"`
	TerraformCloudOrg       string                      `yaml:"terraform_cloud_org,omitempty"`
	TerraformCloudWorkspace string                      `yaml:"terraform_cloud_workspace,omitempty"`
	TerraformCloudToken     string                      `yaml:"terraform_cloud_token,omitempty"`
	SpaceliftAPIKeyEndpoint string                      `yaml:"spacelift_api_key_endpoint,omitempty"`
	SpaceliftAPIKeyID       string                      `yaml:"spacelift_api_key_id,omitempty"`
	SpaceliftAPIKeySecret   string                      `yaml:"spacelift_api_key_secret,omitempty"`
	TerraformWorkspace      string                      `yaml:"terraform_workspace,omitempty"`
	Projects                []*ProjectWithLegacySupport `yaml:"projects"`
	Autodetect              yaml.Node                   `yaml:"autodetect,omitempty"`
	CDK                     []*cdk.ConfigEntry          `yaml:"cdk,omitempty"`
	CDKDefaults             cdk.Defaults                `yaml:"cdk_defaults,omitempty"`
}

type Config struct {
	ConfigBase    `yaml:",inline"`
	Currency      string     `yaml:"currency,omitempty"`
	UsageFilePath string     `yaml:"usage_file,omitempty"`
	Terraform     Terraform  `yaml:"terraform,omitempty"`
	Projects      []*Project `yaml:"projects"`
	CDK           cdk.Config `yaml:"cdk,omitempty"`
}

type ConfigWithAutodetect struct {
	*Config    `yaml:",inline"`
	Autodetect yaml.Node `yaml:"autodetect,omitempty"`
}

type Terraform struct {
	SourceMap []TerraformRegexSource `yaml:"source_map,omitempty"`
	Defaults  TerraformDefaults      `yaml:"defaults,omitempty"`
}

type TerraformDefaults struct {
	Cloud     TerraformCloud `yaml:"cloud,omitempty"`
	Spacelift Spacelift      `yaml:"spacelift,omitempty"`
	Workspace string         `yaml:"workspace,omitempty"`
}

type TerraformCloud struct {
	Host      string `yaml:"host,omitempty"`
	Org       string `yaml:"org,omitempty"`
	Workspace string `yaml:"workspace,omitempty"`
	Token     string `yaml:"token,omitempty"`
}

type Spacelift struct {
	APIKey SpaceliftAPIKey `yaml:"api_key"`
}

type SpaceliftAPIKey struct {
	Endpoint string `yaml:"endpoint,omitempty"`
	ID       string `yaml:"id,omitempty"`
	Secret   string `yaml:"secret,omitempty"`
}

type ProjectWithLegacySupport struct {
	Project                 `yaml:",inline"`
	TerraformVars           map[string]any `yaml:"terraform_vars,omitempty"`
	TerraformWorkspace      string         `yaml:"terraform_workspace,omitempty"`
	TerraformCloudHost      string         `yaml:"terraform_cloud_host,omitempty"`
	TerraformCloudOrg       string         `yaml:"terraform_cloud_org,omitempty"`
	TerraformCloudWorkspace string         `yaml:"terraform_cloud_workspace,omitempty"`
	TerraformCloudToken     string         `yaml:"terraform_cloud_token,omitempty"`
	SpaceliftAPIKeyEndpoint string         `yaml:"spacelift_api_key_endpoint,omitempty"`
	SpaceliftAPIKeyID       string         `yaml:"spacelift_api_key_id,omitempty"`
	SpaceliftAPIKeySecret   string         `yaml:"spacelift_api_key_secret,omitempty"`
	TerraformVarFiles       []string       `yaml:"terraform_var_files,omitempty"`
	ProjectType             ProjectType    `yaml:"project_type,omitempty"`      // terraform, terragrunt
	SkipAutodetect          bool           `yaml:"skip_autodetect,omitempty"`   // deprecated, ignored
	IncludeAllPaths         bool           `yaml:"include_all_paths,omitempty"` // deprecated, ignored
}

type ProjectTerraform struct {
	Cloud     TerraformCloud `yaml:"cloud,omitempty"`
	Spacelift Spacelift      `yaml:"spacelift,omitempty"`
	Vars      map[string]any `yaml:"vars,omitempty"`
	VarFiles  []string       `yaml:"var_files,omitempty"`
	Workspace string         `yaml:"workspace,omitempty"`
}

// Project defines a specific terraform project config. This can be used
// specify per folder/project configurations. Fields are documented below.
// More info is outlined here: https://www.infracost.io/config-file
type Project struct {
	Name            string            `yaml:"name,omitempty"`
	Type            ProjectType       `yaml:"type,omitempty"` // terraform, terragrunt
	Path            string            `yaml:"path"`
	Terraform       ProjectTerraform  `yaml:"terraform,omitempty"`
	Env             map[string]string `yaml:"env,omitempty"`
	Metadata        map[string]string `yaml:"metadata,omitempty"`
	EnvName         string            `yaml:"env_name,omitempty"`
	UsageFile       string            `yaml:"usage_file,omitempty"`
	YorConfigPath   string            `yaml:"yor_config_path,omitempty"`
	ExcludePaths    []string          `yaml:"exclude_paths,omitempty"`
	DependencyPaths []string          `yaml:"dependency_paths,omitempty"`
	AWS             ProjectAWSConfig  `yaml:"aws,omitempty"`
	CDKSynthError   string            `yaml:"cdk_synth_error,omitempty"`
}

type ProjectAWSConfig struct {
	Region    string `yaml:"region,omitempty"`
	StackID   string `yaml:"stack_id,omitempty"`
	StackName string `yaml:"stack_name,omitempty"`
	AccountID string `yaml:"account_id,omitempty"`
}

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
)

type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	AccessToken     string
	Region          string
}

func defaultConfig() *Config {
	return &Config{
		ConfigBase: ConfigBase{
			Version: CurrentVersion,
		},
		Currency: "USD",
		Projects: []*Project{{
			Path: ".",
		}},
	}
}

func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	clone := *c
	return &clone
}
