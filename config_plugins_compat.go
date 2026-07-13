package config

// BACKWARDS-COMPATIBILITY SHIM.
//
// This file folds the deprecated, structured fields (per-project terraform.* / aws.* and the
// repo-level terraform.source_map) into the generic plugins.<name> blob when a config file is read,
// so downstream consumers can read only the new plugins style regardless of which style the file was
// authored in. Old and hand-written configs keep working unchanged.
//
// It deliberately encodes a subset of each plugin's option schema (the JSON keys the plugin
// consumes). That is isolated here so it can be deleted wholesale once the deprecated fields are
// removed - delete this file and the foldDeprecatedFieldsIntoPlugins call in normalize() then.
//
// Precedence: the deprecated structured fields WIN over any existing blob value for the same key. A
// set structured field is hand-written (generation no longer emits them), so it is the most
// authoritative source. foldDeprecatedFieldsIntoPlugins therefore runs after the repo-level plugin
// defaults have been merged into the per-project blob, giving the overall order
// deprecated > per-project blob > repo-level default.

// foldDeprecatedFieldsIntoPlugins populates project.Plugins from the deprecated structured fields.
// repoSourceMap is the repo-level terraform.source_map, folded into each terraform-family project.
func (p *Project) foldDeprecatedFieldsIntoPlugins(repoSourceMap []TerraformRegexSource) {
	switch {
	case isTerraformFamily(p.Type):
		p.foldTerraformIntoPlugins(string(p.Type), repoSourceMap)
	case p.Type == ProjectTypeCloudFormation,
		p.Type == ProjectTypeCDKTypeScript,
		p.Type == ProjectTypeCDKJavaScript,
		p.Type == ProjectTypeCDKPython:
		// aws.* is CloudFormation stack context; CDK is parsed by the cloudformation plugin too, so
		// both fold into plugins.cloudformation (see pluginKeyForType). Guarding on type avoids a
		// spurious plugins.cloudformation entry on a terraform project that happens to set aws.*.
		p.foldAWSIntoPlugins()
	case p.Type == ProjectTypeUnknown:
		// A config with no explicit type that carries terraform.* fields (a legacy 0.1/0.2 config or a
		// hand-written one) is terraform by default - fold it into plugins.terraform.
		if p.hasTerraformFields() || len(repoSourceMap) > 0 {
			p.foldTerraformIntoPlugins(string(ProjectTypeTerraform), repoSourceMap)
		}
	}
}

// hasTerraformFields reports whether any deprecated per-project terraform.* field that folds into the
// blob is set. Workspace is excluded: it is not folded (it stays a top-level, caller-sourced field).
func (p *Project) hasTerraformFields() bool {
	return len(p.Terraform.VarFiles) > 0 ||
		len(p.Terraform.Vars) > 0 ||
		p.Terraform.Cloud.Org != "" ||
		p.Terraform.Cloud.Workspace != "" ||
		p.Terraform.Cloud.Host != ""
}

func (p *Project) foldTerraformIntoPlugins(key string, repoSourceMap []TerraformRegexSource) {
	if len(p.Terraform.VarFiles) > 0 {
		p.setPluginOption(key, "tfVarsFiles", p.Terraform.VarFiles)
	}
	// Workspace is deliberately NOT folded into the blob: it is a caller-sourced runtime option passed
	// via GenericOptions.Workspace (and read outside the plugin), so it stays on the top-level
	// terraform.workspace field.
	if len(p.Terraform.Vars) > 0 {
		p.setPluginOption(key, "vars", p.Terraform.Vars)
	}
	// Match the callers' gate: a Terraform Cloud configuration is only meaningful with a workspace.
	// token is a credential and is passed via GenericOptions, never persisted here.
	if p.Terraform.Cloud.Workspace != "" {
		p.setPluginOption(key, "terraformCloudConfiguration", map[string]any{
			"organization": p.Terraform.Cloud.Org,
			"workspace":    p.Terraform.Cloud.Workspace,
			"hostname":     p.Terraform.Cloud.Host,
		})
	}
	if len(repoSourceMap) > 0 {
		sm := make(map[string]any, len(repoSourceMap))
		for _, s := range repoSourceMap {
			sm[s.Match] = s.Replace
		}
		p.setPluginOption(key, "regexSourceMap", sm)
	}
}

func (p *Project) foldAWSIntoPlugins() {
	awsContext := map[string]any{}
	if p.AWS.Region != "" {
		awsContext["region"] = p.AWS.Region
	}
	if p.AWS.AccountID != "" {
		awsContext["accountId"] = p.AWS.AccountID
	}
	if p.AWS.StackID != "" {
		awsContext["stackId"] = p.AWS.StackID
	}
	if p.AWS.StackName != "" {
		awsContext["stackName"] = p.AWS.StackName
	}
	if len(awsContext) == 0 {
		return
	}
	p.setPluginOption(string(ProjectTypeCloudFormation), "awsContext", awsContext)
}
