package config

// This file holds the plumbing for the generic, plugin-keyed `plugins.<name>` config section: the
// map from a project type to the plugin that consumes its options, and the schema-agnostic deep
// merge used to combine repo-level defaults with per-project blobs. Neither understands a plugin's
// option schema - that stays opaque to config. The backwards-compatibility fold of the deprecated
// structured fields lives separately in config_plugins_compat.go.

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

// mergePluginBlobs deep-merges a repo-level plugin defaults map (global) with a per-project plugin
// map (project) and returns the result. It is schema-agnostic: nested maps merge recursively, and
// every other value (scalars and lists alike) is atomic with the per-project value winning. Neither
// input is mutated.
func mergePluginBlobs(global, project map[string]any) map[string]any {
	if m, ok := deepMergeAny(global, project).(map[string]any); ok {
		return m
	}
	return nil
}

// deepMergeAny merges project onto global following the plugin-merge convention (see
// mergePluginBlobs). Only maps merge recursively; every other value - scalars AND lists - is treated
// atomically, with the per-project value winning when it is set.
func deepMergeAny(global, project any) any {
	gm, gok := global.(map[string]any)
	pm, pok := project.(map[string]any)
	if gok && pok {
		out := make(map[string]any, len(gm)+len(pm))
		for k, gv := range gm {
			// deep-copy global branches: global is the shared repo-level defaults map reused for
			// every project, so aliasing its nested maps/slices into a project's result would let a
			// later mutation of one project's blob corrupt the global and every sibling project.
			out[k] = deepCopyAny(gv)
		}
		for k, pv := range pm {
			if gv, ok := gm[k]; ok {
				out[k] = deepMergeAny(gv, pv)
			} else {
				out[k] = pv
			}
		}
		return out
	}

	// Non-map values (scalars and lists) are atomic: the per-project value wins, unless the project
	// didn't set anything, in which case fall back to a copy of the global default. Lists are
	// deliberately NOT concatenated - a project's list replaces the global one.
	if project == nil {
		return deepCopyAny(global)
	}
	return project
}

// deepCopyAny returns a deep copy of a decoded YAML/JSON value (nested maps, slices, scalars) so a
// merged result never aliases the shared global-defaults map.
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

// setPluginOption stores a single key on a project's plugins.<key> blob, allocating the maps as
// needed. It is used by both the sideload (generation) and the deprecated fold (read).
func (p *Project) setPluginOption(key, option string, value any) {
	if p.Plugins == nil {
		p.Plugins = map[string]map[string]any{}
	}
	if p.Plugins[key] == nil {
		p.Plugins[key] = map[string]any{}
	}
	p.Plugins[key][option] = value
}
