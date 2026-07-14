package config

import (
	"bytes"
	"fmt"

	"github.com/infracost/config/cdk"
	"gopkg.in/yaml.v3"
)

// This file isolates the deprecated 0.1/0.2 config-file support: the intermediary types that mirror
// the old flat field names and the parser that migrates them into the current API Config. It is a
// verbatim relocation of what used to live in config.go / config_file.go, kept together so it can be
// deleted wholesale once 0.1/0.2 support is dropped. It deliberately produces a *Config directly (it
// is same-package), so it never touches the configfile file-syntax layer used by the current format.

var legacyVersions = []string{
	"0.1", // The 0.1 version of the config file was used in the original CLI.
	"0.2", // The 0.2 version of the config file was used in the original version of the Infracost Cloud Platform.
}

// ConfigWithLegacySupport handles the parameters that were supported in 0.1 and 0.2 but are not
// supported in 0.3. This allows us to parse legacy config files.
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

func parseLegacyVersion(content []byte, config *Config) error {

	var intermediary ConfigWithLegacySupport
	if len(config.Projects) > 0 {
		// if the config came with projectsd set (e.g. default main) we need to set it here to see if it gets overridden by legacy projects
		intermediary.Projects = make([]*ProjectWithLegacySupport, 0, len(config.Projects))
		for _, project := range config.Projects {
			intermediary.Projects = append(intermediary.Projects, &ProjectWithLegacySupport{
				Project: *project,
			})
		}
	}

	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)

	if err := decoder.Decode(&intermediary); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidConfigYAML, simplifyYAMLError(err))
	}

	// copy across fields that exist in both
	config.ConfigBase = intermediary.ConfigBase
	config.Version = CurrentVersion // force version to latest after converting
	config.Currency = intermediary.Currency
	config.UsageFilePath = intermediary.UsageFilePath
	config.CDK.Projects = intermediary.CDK
	config.CDK.Defaults = intermediary.CDKDefaults

	// copy legacy fields to their new locations
	config.Terraform.SourceMap = intermediary.TerraformRegexSourceMap
	config.Terraform.Defaults.Cloud.Host = intermediary.TerraformCloudHost
	config.Terraform.Defaults.Cloud.Org = intermediary.TerraformCloudOrg
	config.Terraform.Defaults.Cloud.Workspace = intermediary.TerraformCloudWorkspace
	config.Terraform.Defaults.Cloud.Token = intermediary.TerraformCloudToken
	config.Terraform.Defaults.Spacelift.APIKey.Endpoint = intermediary.SpaceliftAPIKeyEndpoint
	config.Terraform.Defaults.Spacelift.APIKey.ID = intermediary.SpaceliftAPIKeyID
	config.Terraform.Defaults.Spacelift.APIKey.Secret = intermediary.SpaceliftAPIKeySecret
	config.Terraform.Defaults.Workspace = intermediary.TerraformWorkspace

	// remove default projects and take whatever the decode gace us - if the user didn't specify the projects key, we'll get the defaults preserved anyway
	config.Projects = nil

	// convert legacy projects to new ones
	// this is deliberately a nil check rather than checking length, as we want to preserve an empty projects section if it was explicitly set to empty in the legacy config
	if len(intermediary.Projects) > 0 {
		config.Projects = make([]*Project, 0, len(intermediary.Projects))

		for _, legacyProject := range intermediary.Projects {
			project := legacyProject.Project
			if len(legacyProject.TerraformVars) > 0 {
				if project.Terraform.Vars == nil {
					project.Terraform.Vars = make(map[string]any)
				}
				for k, v := range legacyProject.TerraformVars {
					project.Terraform.Vars[k] = v
				}
			}
			if legacyProject.TerraformWorkspace != "" {
				project.Terraform.Workspace = legacyProject.TerraformWorkspace
			}
			if legacyProject.TerraformCloudHost != "" {
				project.Terraform.Cloud.Host = legacyProject.TerraformCloudHost
			}
			if legacyProject.TerraformCloudOrg != "" {
				project.Terraform.Cloud.Org = legacyProject.TerraformCloudOrg
			}
			if legacyProject.TerraformCloudWorkspace != "" {
				project.Terraform.Cloud.Workspace = legacyProject.TerraformCloudWorkspace
			}
			if legacyProject.TerraformCloudToken != "" {
				project.Terraform.Cloud.Token = legacyProject.TerraformCloudToken
			}
			if legacyProject.SpaceliftAPIKeyEndpoint != "" {
				project.Terraform.Spacelift.APIKey.Endpoint = legacyProject.SpaceliftAPIKeyEndpoint
			}
			if legacyProject.SpaceliftAPIKeyID != "" {
				project.Terraform.Spacelift.APIKey.ID = legacyProject.SpaceliftAPIKeyID
			}
			if legacyProject.SpaceliftAPIKeySecret != "" {
				project.Terraform.Spacelift.APIKey.Secret = legacyProject.SpaceliftAPIKeySecret
			}
			if len(legacyProject.TerraformVarFiles) > 0 {
				project.Terraform.VarFiles = legacyProject.TerraformVarFiles
			}
			if legacyProject.ProjectType != "" {
				project.Type = legacyProject.ProjectType
			}
			foldDeprecatedIntoPlugins(&project)
			config.Projects = append(config.Projects, &project)
		}
	}

	return nil
}

// foldDeprecatedIntoPlugins copies a legacy project's typed terraform var files/vars and aws context
// into the opaque Plugins blob, keyed exactly as the 0.3 read path keys them. It does NOT clear the
// typed fields: the blob is the canonical source (serialized, consumer-preferred) and the typed fields
// remain as a backwards-compat mirror (matching mirrorBlobToDeprecated on the 0.3 path). configlegacy
// is read-only, so it maps straight to the final API shape - and the blob copy is what survives the
// env-var marshal/re-parse round-trip (which keeps only workspace/cloud/spacelift typed).
func foldDeprecatedIntoPlugins(p *Project) {
	tfKey := terraformBlobKey(p.Type)
	if len(p.Terraform.VarFiles) > 0 {
		p.setPluginOption(tfKey, "var_files", p.Terraform.VarFiles)
	}
	if len(p.Terraform.Vars) > 0 {
		p.setPluginOption(tfKey, "vars", p.Terraform.Vars)
	}

	if aws := awsContextBlob(p.AWS); len(aws) > 0 {
		if p.Plugins == nil {
			p.Plugins = map[string]map[string]any{}
		}
		p.Plugins[string(ProjectTypeCloudFormation)] = aws
	}
}

// awsContextBlob renders the deprecated typed aws context as the flat snake_case blob the
// cloudformation plugin reads (matching the aws: heading keys).
func awsContextBlob(a ProjectAWSConfig) map[string]any {
	blob := map[string]any{}
	if a.Region != "" {
		blob["region"] = a.Region
	}
	if a.AccountID != "" {
		blob["account_id"] = a.AccountID
	}
	if a.StackID != "" {
		blob["stack_id"] = a.StackID
	}
	if a.StackName != "" {
		blob["stack_name"] = a.StackName
	}
	return blob
}
