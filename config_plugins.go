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
