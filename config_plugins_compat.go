package config

import "encoding/json"

// BACKWARDS-COMPATIBILITY SHIM.
//
// This file folds the deprecated, structured per-project fields (terraform.* and aws.*) into the
// generic plugins.<name> blob when a config file is read, so downstream consumers can read only
// the new plugins style regardless of which style the file was authored in.
//
// It deliberately encodes a subset of each plugin's option schema (the JSON keys the plugin
// consumes). That is isolated here so it can be deleted wholesale once the deprecated fields are
// removed. Delete this file and the call to foldDeprecatedFieldsIntoPlugins in normalize() then.
//
// Precedence: the structured fields WIN over any existing blob value for the same key - a set
// structured field is intentional (either it predates the blob or the user added it).

// foldDeprecatedFieldsIntoPlugins populates project.Plugins from the deprecated structured fields.
// repoSourceMap is the repo-level terraform.source_map, folded into each terraform-family project.
func (p *Project) foldDeprecatedFieldsIntoPlugins(repoSourceMap []TerraformRegexSource) {
	switch p.Type {
	case ProjectTypeTerraform, ProjectTypeTerragrunt, ProjectTypeCiscoStacks:
		p.foldTerraformIntoPlugins(string(p.Type), repoSourceMap)
	case ProjectTypeCloudFormation, ProjectTypeCDKTypeScript, ProjectTypeCDKJavaScript, ProjectTypeCDKPython:
		// aws.* is CloudFormation stack context; CDK is parsed by the cloudformation plugin too,
		// so both fold into plugins.cloudformation. Only these types carry aws.* - guarding here
		// avoids a spurious plugins.cloudformation entry on a terraform project that happens to
		// set aws.*.
		p.foldAWSIntoPlugins()
	case "":
		// A config with no explicit type that carries terraform.* fields (e.g. a legacy 0.1/0.2
		// config or a hand-written template) is terraform by default - fold it into plugins.terraform.
		if p.hasTerraformFields() || len(repoSourceMap) > 0 {
			p.foldTerraformIntoPlugins(string(ProjectTypeTerraform), repoSourceMap)
		}
	}
}

// hasTerraformFields reports whether any deprecated per-project terraform.* field is set.
func (p *Project) hasTerraformFields() bool {
	return len(p.Terraform.VarFiles) > 0 ||
		p.Terraform.Workspace != "" ||
		len(p.Terraform.Vars) > 0 ||
		p.Terraform.Cloud.Org != "" ||
		p.Terraform.Cloud.Workspace != "" ||
		p.Terraform.Cloud.Host != ""
}

func (p *Project) foldTerraformIntoPlugins(key string, repoSourceMap []TerraformRegexSource) {
	fold := map[string]any{}

	if len(p.Terraform.VarFiles) > 0 {
		fold["tfVarsFiles"] = p.Terraform.VarFiles
	}
	if p.Terraform.Workspace != "" {
		fold["workspace"] = p.Terraform.Workspace
	}
	if len(p.Terraform.Vars) > 0 {
		fold["vars"] = p.Terraform.Vars
	}
	// Match the callers' gate: a Terraform Cloud configuration is only meaningful with a
	// workspace. token is a credential and is passed via GenericOptions, never persisted here.
	if p.Terraform.Cloud.Workspace != "" {
		fold["terraformCloudConfiguration"] = map[string]any{
			"organization": p.Terraform.Cloud.Org,
			"workspace":    p.Terraform.Cloud.Workspace,
			"hostname":     p.Terraform.Cloud.Host,
		}
	}
	if len(repoSourceMap) > 0 {
		sm := make(map[string]any, len(repoSourceMap))
		for _, s := range repoSourceMap {
			sm[s.Match] = s.Replace
		}
		fold["regexSourceMap"] = sm
	}

	p.applyPluginFold(key, fold)
}

func (p *Project) foldAWSIntoPlugins() {
	a := p.AWS
	awsContext := map[string]any{}
	if a.Region != "" {
		awsContext["region"] = a.Region
	}
	if a.AccountID != "" {
		awsContext["accountId"] = a.AccountID
	}
	if a.StackID != "" {
		awsContext["stackId"] = a.StackID
	}
	if a.StackName != "" {
		awsContext["stackName"] = a.StackName
	}
	if len(awsContext) == 0 {
		return
	}
	p.applyPluginFold("cloudformation", map[string]any{"awsContext": awsContext})
}

// applyPluginFold overwrites the given keys on the project's plugins.<key> blob with the folded
// (structured) values. The fold is first canonicalised through JSON so its value types match the
// autodetect-emitted blob (lists as []any, maps as map[string]any, etc.).
func (p *Project) applyPluginFold(key string, fold map[string]any) {
	if len(fold) == 0 {
		return
	}

	canonical := fold
	if b, err := json.Marshal(fold); err == nil {
		var decoded map[string]any
		if err := json.Unmarshal(b, &decoded); err == nil {
			canonical = decoded
		}
	}

	if p.Plugins == nil {
		p.Plugins = map[string]map[string]any{}
	}
	if p.Plugins[key] == nil {
		p.Plugins[key] = map[string]any{}
	}
	for k, v := range canonical {
		p.Plugins[key][k] = v
	}
}
