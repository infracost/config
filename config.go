package config

import (
	"crypto/sha1" // nolint:gosec
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/infracost/config/cdk"
	"github.com/infracost/config/types"
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

// ConfigSHA computes a deterministic SHA for the project config based on its
// key fields. This is used by the dashboard to detect config changes and
// deduplicate breakdowns.
func (p *Project) ConfigSHA() string {
	var inputs []string
	inputs = append(inputs, p.Name)
	inputs = append(inputs, p.Path)
	inputs = append(inputs, p.ExcludePaths...)
	inputs = append(inputs, p.DependencyPaths...)
	orderVars := make([]string, 0, len(p.Terraform.Vars))
	for k, v := range p.Terraform.Vars {
		orderVars = append(orderVars, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(orderVars)
	inputs = append(inputs, orderVars...)
	inputs = append(inputs, p.Terraform.VarFiles...)
	inputs = append(inputs, p.Terraform.Workspace)
	inputs = append(inputs, p.Terraform.Cloud.Host)
	inputs = append(inputs, p.Terraform.Cloud.Org)
	inputs = append(inputs, p.Terraform.Cloud.Workspace)
	inputs = append(inputs, p.Terraform.Cloud.Token)
	inputs = append(inputs, p.UsageFile)
	inputs = append(inputs, p.Terraform.Spacelift.APIKey.Endpoint)
	inputs = append(inputs, p.Terraform.Spacelift.APIKey.ID)
	inputs = append(inputs, p.Terraform.Spacelift.APIKey.Secret)
	orderEnv := make([]string, 0, len(p.Env))
	for k, v := range p.Env {
		orderEnv = append(orderEnv, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(orderEnv)
	inputs = append(inputs, orderEnv...)

	// nolint:gosec
	gen := sha1.New()
	_, _ = gen.Write([]byte(strings.Join(inputs, "|")))
	return hex.EncodeToString(gen.Sum(nil))
}

type ProjectAWSConfig struct {
	Region    string `yaml:"region,omitempty"`
	StackID   string `yaml:"stack_id,omitempty"`
	StackName string `yaml:"stack_name,omitempty"`
	AccountID string `yaml:"account_id,omitempty"`
}

// ProjectType is aliased to types.ProjectType so the canonical definition can
// live in a child-importable package while existing consumers of
// config.ProjectType keep compiling unchanged.
type ProjectType = types.ProjectType

const (
	ProjectTypeUnknown        = types.ProjectTypeUnknown
	ProjectTypeTerraform      = types.ProjectTypeTerraform
	ProjectTypeTerragrunt     = types.ProjectTypeTerragrunt
	ProjectTypeCloudFormation = types.ProjectTypeCloudFormation
	ProjectTypeCDKTypeScript  = types.ProjectTypeCDKTypeScript
	ProjectTypeCDKJavaScript  = types.ProjectTypeCDKJavaScript
	ProjectTypeCDKPython      = types.ProjectTypeCDKPython
	ProjectTypeCiscoStacks    = types.ProjectTypeCiscoStacks
	ProjectTypeKubernetes     = types.ProjectTypeKubernetes
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
