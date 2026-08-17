package config_test

import (
	"testing"

	"github.com/infracost/config"
)

// Test_Generate_NestedKubernetesProjects_WithPlugins checks that projects nested
// beneath a directory that is itself a project are still detected.
//
// A GitOps repository commonly keeps a controller manifest in its root - an
// object pointing at the manifests held elsewhere in the tree - which makes the
// root a Kubernetes project in its own right. Every real project in the
// repository then sits beneath it. Nesting like this is the normal layout for
// Kubernetes (a Kustomize app directory holds base/ and overlays/ that are
// themselves kustomization directories), so a project must not be dropped merely
// for having a project ancestor: doing so collapses the whole repository to its
// root, which is read non-recursively and yields nothing.
//
// Where a parent genuinely owns its nested directories, the parent's
// IdentifyEnvironments answer claims them and they are suppressed precisely -
// which is why app/base and app/overlays/dev do not appear here as projects of
// their own, while app itself expands into its dev environment.
func Test_Generate_NestedKubernetesProjects_WithPlugins(t *testing.T) {
	root := NewFilesystem(t)
	root.AddGitOpsControllerManifest("root-app.yaml")

	app := root.AddDirectory("app")
	base := app.AddDirectory("base")
	base.AddKustomization("deployment.yaml")
	base.AddKubernetesDeployment("deployment.yaml")
	dev := app.AddDirectory("overlays").AddDirectory("dev")
	dev.AddKustomization("../../base")

	wrappers := root.AddDirectory("wrappers")
	wrappers.AddGitOpsControllerManifest("frontend.yaml")

	testConfigGenerationWithPlugins(t, root.Path(), []*config.Project{
		{
			Name: "main",
			Path: ".",
			Type: "kubernetes",
		},
		{
			Name:            "app-dev",
			Path:            "app/overlays/dev",
			EnvName:         "dev",
			Type:            "kubernetes",
			DependencyPaths: []string{"app/base/**"},
		},
		{
			Name: "wrappers",
			Path: "wrappers",
			Type: "kubernetes",
		},
	})
}

// Test_Generate_CloudFormationInsideTerraform_WithPlugins pins the one nesting
// case that is still suppressed: a CloudFormation template inside a Terraform
// project is nearly always deployed by that Terraform (via
// aws_cloudformation_stack), so emitting it standalone would represent a single
// deployment twice. The same template outside a Terraform project is a project.
func Test_Generate_CloudFormationInsideTerraform_WithPlugins(t *testing.T) {
	root := NewFilesystem(t)

	infra := root.AddDirectory("infra")
	infra.AddTerraformFileWithProviderBlock("main.tf")
	infra.AddCFNYAML()

	standalone := root.AddDirectory("standalone")
	standalone.AddCFNYAML()

	testConfigGenerationWithPlugins(t, root.Path(), []*config.Project{
		{
			Name: "infra",
			Path: "infra",
			Type: "terraform",
		},
		{
			Name: "standalone-template",
			Path: "standalone/template.yml",
			Type: "cloudformation",
		},
	})
}
