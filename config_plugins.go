package config

// This file holds the plumbing for the generic, plugin-keyed `plugins.<name>` config section: the
// map from a project type to the plugin that consumes its options. It does not understand a plugin's
// option schema - that stays opaque to config.

// pluginKeyForType returns the config-file plugins.<name> key for a project type: the name of the
// plugin that consumes that project's parse options. It is the identity for every type except the
// CDK variants, which are parsed by the cloudformation plugin and so share its options blob.
//
// Keying by consuming plugin (rather than project type) is what lets generation and the deprecated
// fold agree on where a project's blob lives - the two only diverge for CDK.
func pluginKeyForType(t ProjectType) string {
	switch t {
	case ProjectTypeCDKTypeScript, ProjectTypeCDKJavaScript, ProjectTypeCDKPython:
		return string(ProjectTypeCloudFormation)
	default:
		return string(t)
	}
}

// isTerraformFamily reports whether a project type is parsed with the terraform options schema
// (terraform, terragrunt and cisco stacks all share it), and so folds/sideloads into a
// terraform-shaped blob.
func isTerraformFamily(t ProjectType) bool {
	switch t {
	case ProjectTypeTerraform, ProjectTypeTerragrunt, ProjectTypeCiscoStacks:
		return true
	default:
		return false
	}
}

// The config file collapses a few project families onto a single heading (terraform-family -> the
// `terraform:` heading; cloudformation/CDK -> the historical `aws:` heading), while the in-memory
// Plugins map is keyed by the exact project type (what the CLI/runner look up). blobKeyForHeading and
// headingForBlobKey are the two directions of that mapping, so read and write agree.

// blobKeyForHeading returns the Plugins key that a given file heading maps to for a project of type t.
// It is the read direction: file heading -> in-memory Plugins key.
func blobKeyForHeading(heading string, t ProjectType) string {
	switch heading {
	case "terraform":
		return terraformBlobKey(t)
	case "aws":
		return string(ProjectTypeCloudFormation)
	default:
		return heading
	}
}

// terraformBlobKey returns the Plugins key for a terraform-family project's blob: the exact type for
// terraform/terragrunt/cisco, and terraform for an untyped project carrying terraform options.
func terraformBlobKey(t ProjectType) string {
	if isTerraformFamily(t) {
		return string(t)
	}
	return string(ProjectTypeTerraform)
}

// headingForBlobKey returns the file heading a Plugins key is written under. It is the write direction
// and the inverse of blobKeyForHeading: the terraform family collapses to `terraform:`, cloudformation
// (which cdk_* keys resolve to) to `aws:`, and everything else keeps its own name.
func headingForBlobKey(key string) string {
	switch ProjectType(key) {
	case ProjectTypeTerraform, ProjectTypeTerragrunt, ProjectTypeCiscoStacks:
		return "terraform"
	case ProjectTypeCloudFormation:
		return "aws"
	default:
		return key
	}
}

// terraformTypedKeys are the keys under a project's terraform: heading that config keeps as typed
// fields (read outside the plugin) rather than forwarding in the opaque blob. Everything else under
// terraform: is blob.
var terraformTypedKeys = map[string]bool{
	"cloud":     true,
	"spacelift": true,
	"workspace": true,
}

// setPluginOption stores a single option on p.Plugins[key], allocating the maps as needed.
func (p *Project) setPluginOption(key, option string, value any) {
	if p.Plugins == nil {
		p.Plugins = map[string]map[string]any{}
	}
	if p.Plugins[key] == nil {
		p.Plugins[key] = map[string]any{}
	}
	p.Plugins[key][option] = value
}

// isCDKType reports whether a project type is one of the CDK variants (parsed by the cloudformation
// plugin).
func isCDKType(t ProjectType) bool {
	switch t {
	case ProjectTypeCDKTypeScript, ProjectTypeCDKJavaScript, ProjectTypeCDKPython:
		return true
	default:
		return false
	}
}

// foldRepoPluginDefaults folds repo-level config into each project's plugin blob, then re-syncs the
// deprecated typed mirrors. Two things fold in:
//   - the legacy repo-level terraform.source_map (a list of {match,replace}) is transformed into the
//     blob's regex_source_map (a map) under the terraform repo defaults - a compat shim, so it then
//     flows through the generic merge like any other repo-level terraform option; and
//   - repo-level plugin defaults (c.Plugins) are deep-merged into each project whose type they apply
//     to, with the per-project value winning.
func (c *Config) foldRepoPluginDefaults() {
	for _, project := range c.Projects {
		for repoKey, defaults := range c.Plugins {
			if len(defaults) == 0 {
				continue
			}
			target, ok := repoDefaultAppliesTo(repoKey, project.Type)
			if !ok {
				continue
			}
			if merged := mergePluginBlobs(defaults, project.Plugins[target]); len(merged) > 0 {
				setProjectPlugin(project, target, merged)
			}
		}

		c.foldSourceMapInto(project)

		// re-sync the deprecated typed mirrors, which may now include merged-in repo defaults.
		mirrorBlobToDeprecated(project)
	}
}

// foldSourceMapInto is the compat shim for the deprecated repo-level terraform.source_map: it folds
// it into each terraform-family project's blob as regex_source_map (transforming the list of
// {match,replace} into a map), unless the project already sets one (per-project wins). It stays on
// c.Terraform.SourceMap (the deprecated typed field / what's serialized) rather than being moved to a
// repo blob, so it survives the env-var marshal/re-parse round-trip which runs before normalize.
func (c *Config) foldSourceMapInto(project *Project) {
	if len(c.Terraform.SourceMap) == 0 {
		return
	}
	if !isTerraformFamily(project.Type) && project.Type != ProjectTypeUnknown {
		return
	}
	key := terraformBlobKey(project.Type)
	if project.Plugins[key]["regex_source_map"] != nil {
		return
	}
	m := make(map[string]any, len(c.Terraform.SourceMap))
	for _, s := range c.Terraform.SourceMap {
		m[s.Match] = s.Replace
	}
	project.setPluginOption(key, "regex_source_map", m)
}

// repoDefaultAppliesTo reports whether a repo-level plugin default (keyed by repoKey) applies to a
// project of type t and, if so, which of the project's blob keys it merges into. The terraform family
// shares the "terraform" repo default (merged into each member's exact-type blob); cloudformation
// covers the CDK variants.
func repoDefaultAppliesTo(repoKey string, t ProjectType) (string, bool) {
	switch repoKey {
	case string(ProjectTypeTerraform):
		if isTerraformFamily(t) || t == ProjectTypeUnknown {
			return terraformBlobKey(t), true
		}
	case string(ProjectTypeCloudFormation):
		if t == ProjectTypeCloudFormation || isCDKType(t) {
			return string(ProjectTypeCloudFormation), true
		}
	default:
		if string(t) == repoKey {
			return repoKey, true
		}
	}
	return "", false
}

// mergePluginBlobs deep-merges repo-level defaults under a per-project blob and returns the result;
// the per-project value wins. Nested maps merge recursively; every other value (scalars and lists) is
// atomic. Neither input is mutated.
func mergePluginBlobs(defaults, project map[string]any) map[string]any {
	if m, ok := deepMergeAny(defaults, project).(map[string]any); ok {
		return m
	}
	return nil
}

// deepMergeAny merges override onto base: only maps merge recursively; every other value (scalars AND
// lists) is atomic, with override winning when set. base branches are deep-copied so the shared
// repo-defaults map is never aliased into a project's result.
func deepMergeAny(base, override any) any {
	bm, bok := base.(map[string]any)
	om, ook := override.(map[string]any)
	if bok && ook {
		out := make(map[string]any, len(bm)+len(om))
		for k, v := range bm {
			out[k] = deepCopyAny(v)
		}
		for k, v := range om {
			if bv, ok := bm[k]; ok {
				out[k] = deepMergeAny(bv, v)
			} else {
				out[k] = v
			}
		}
		return out
	}
	if override == nil {
		return deepCopyAny(base)
	}
	return override
}

func deepCopyAny(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = deepCopyAny(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = deepCopyAny(val)
		}
		return out
	default:
		return v
	}
}
