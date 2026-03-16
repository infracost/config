package config_test

import (
	"testing"

	"github.com/infracost/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Generate_SimpleTFInRoot(t *testing.T) {

	root := NewFilesystem(t)
	root.AddFile("main.tf")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "main",
			Path: ".",
			Type: "terraform",
		},
	},
	)
}

func Test_Generate_NoProjects(t *testing.T) {
	root := NewFilesystem(t)
	generated, err := config.Generate(root.Path())
	require.NoError(t, err)
	assert.Len(t, generated.Projects, 0)
}

func Test_Generate_SimpleTFInRootWithTemplate(t *testing.T) {
	root := NewFilesystem(t)
	root.AddFile("main.tf")

	template := `version: 0.1
projects:
  {{ range .DetectedProjects }}
    - name: {{ .Name }}
      path: {{ .Path }}
  {{ end }}
`
	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "main",
			Path: ".",
		},
	})
}

func Test_Generate_SimpleTFInRootWithTemplateMissingProjectsSection(t *testing.T) {
	root := NewFilesystem(t)
	root.AddFile("main.tf")

	template := `version: 0.1
`
	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "main",
			Path: ".",
			Type: "terraform",
		},
	})
}

func Test_Generate_Aunt(t *testing.T) {
	root := NewFilesystem(t)

	appsDir := root.AddDirectory("apps")
	barDir := appsDir.AddDirectory("bar")
	{
		us1Dir := barDir.AddDirectory("us1")
		us1Dir.AddTerraformFileWithProviderBlock("main.tf")
		us2Dir := barDir.AddDirectory("us2")
		us2Dir.AddTerraformFileWithProviderBlock("main.tf")
	}
	fooDir := appsDir.AddDirectory("foo")
	{
		us1Dir := fooDir.AddDirectory("us1")
		us1Dir.AddTerraformFileWithProviderBlock("main.tf")
		us2Dir := fooDir.AddDirectory("us2")
		us2Dir.AddTerraformFileWithProviderBlock("main.tf")
	}
	envsDir := appsDir.AddDirectory("envs")
	envsDir.AddTFVarsFile("dev.tfvars")
	envsDir.AddTFVarsFile("prod.tfvars")
	envsDir.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "apps-bar-us1-dev",
			Path:    "apps/bar/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-us1-prod",
			Path:    "apps/bar/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-us2-dev",
			Path:    "apps/bar/us2",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-us2-prod",
			Path:    "apps/bar/us2",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us1-dev",
			Path:    "apps/foo/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us1-prod",
			Path:    "apps/foo/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us2-dev",
			Path:    "apps/foo/us2",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us2-prod",
			Path:    "apps/foo/us2",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_AuntAndGreatAunt(t *testing.T) {
	root := NewFilesystem(t)

	infraDir := root.AddDirectory("infra")
	appsDir := infraDir.AddDirectory("apps")
	barDir := appsDir.AddDirectory("bar")
	{
		us1Dir := barDir.AddDirectory("us1")
		us1Dir.AddTerraformFileWithProviderBlock("main.tf")
		us2Dir := barDir.AddDirectory("us2")
		us2Dir.AddTerraformFileWithProviderBlock("main.tf")
	}
	fooDir := appsDir.AddDirectory("foo")
	{
		us1Dir := fooDir.AddDirectory("us1")
		us1Dir.AddTerraformFileWithProviderBlock("main.tf")
		us2Dir := fooDir.AddDirectory("us2")
		us2Dir.AddTerraformFileWithProviderBlock("main.tf")
	}
	appsEnvsDir := appsDir.AddDirectory("envs")
	appsEnvsDir.AddTFVarsFile("dev.tfvars")
	appsEnvsDir.AddTFVarsFile("prod.tfvars")
	appsEnvsDir.AddTFVarsFile("default.tfvars")
	infraEnvsDir := infraDir.AddDirectory("envs")
	infraEnvsDir.AddTFVarsFile("dev.tfvars")
	infraEnvsDir.AddTFVarsFile("prod.tfvars")
	infraEnvsDir.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "infra-apps-bar-us1-dev",
			Path:    "infra/apps/bar/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/dev.tfvars",
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-bar-us1-prod",
			Path:    "infra/apps/bar/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/prod.tfvars",
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-bar-us2-dev",
			Path:    "infra/apps/bar/us2",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/dev.tfvars",
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-bar-us2-prod",
			Path:    "infra/apps/bar/us2",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/prod.tfvars",
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-foo-us1-dev",
			Path:    "infra/apps/foo/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/dev.tfvars",
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-foo-us1-prod",
			Path:    "infra/apps/foo/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/prod.tfvars",
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-foo-us2-dev",
			Path:    "infra/apps/foo/us2",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/dev.tfvars",
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-foo-us2-prod",
			Path:    "infra/apps/foo/us2",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/prod.tfvars",
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_AuntFilenameMatchesProject(t *testing.T) {
	root := NewFilesystem(t)

	/*
		infra
			components
				foo
					main.tf
				bar
					main.tf
				baz
					main.tf
			variables
				envs
					prod
						defaults.tfvars
						foo.tfvars
						bar.tfvars
					dev
						defaults.tfvars
						bar.tfvars
	*/

	infraDir := root.AddDirectory("infra")
	componentsDir := infraDir.AddDirectory("components")
	fooDir := componentsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	barDir := componentsDir.AddDirectory("bar")
	barDir.AddTerraformFileWithProviderBlock("main.tf")
	bazDir := componentsDir.AddDirectory("baz")
	bazDir.AddTerraformFileWithProviderBlock("main.tf")

	variablesDir := infraDir.AddDirectory("variables")
	envDir := variablesDir.AddDirectory("env")
	prodDir := envDir.AddDirectory("prod")
	prodDir.AddTFVarsFile("defaults.tfvars")
	prodDir.AddTFVarsFile("foo.tfvars")
	prodDir.AddTFVarsFile("bar.tfvars")
	devDir := envDir.AddDirectory("dev")
	devDir.AddTFVarsFile("defaults.tfvars")
	devDir.AddTFVarsFile("bar.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "infra-components-bar-dev",
			Path:    "infra/components/bar",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/env/dev/bar.tfvars",
					"../../variables/env/dev/defaults.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-components-bar-prod",
			Path:    "infra/components/bar",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/env/prod/bar.tfvars",
					"../../variables/env/prod/defaults.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-components-baz-dev",
			Path:    "infra/components/baz",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/env/dev/defaults.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-components-baz-prod",
			Path:    "infra/components/baz",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/env/prod/defaults.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-components-foo-dev",
			Path:    "infra/components/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/env/dev/defaults.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-components-foo-prod",
			Path:    "infra/components/foo",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/env/prod/defaults.tfvars",
					"../../variables/env/prod/foo.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_AuntFilenamesWithEnv(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		├── components
		│   ├── age
		│   │   └── main.tf
		│   ├── airflow
		│   │   └── main.tf
		│   ├── apm-events
		│   │   └── main.tf
		└── variables
			├── cko-dev
			│   ├── age.tfvars
			│   ├── airflow.tfvars
			│   ├── dev.tfvars
			│   └── apm-events.tfvars
			├── cko-mgmt
			│   ├── age.tfvars
			│   ├── airflow.tfvars
			│   └── apm-events.tfvars
			├── cko-playground
			│   ├── age.tfvars
			│   ├── airflow.tfvars
			│   └── apm-events.tfvars
			├── cko-prod
			│   ├── age.tfvars
			│   ├── airflow.tfvars
			│   └── apm-events.tfvars
			└── default.tfvars
	*/

	componentsDir := root.AddDirectory("components")
	ageDir := componentsDir.AddDirectory("age")
	ageDir.AddTerraformFileWithProviderBlock("main.tf")
	airflowDir := componentsDir.AddDirectory("airflow")
	airflowDir.AddTerraformFileWithProviderBlock("main.tf")
	apmEventsDir := componentsDir.AddDirectory("apm-events")
	apmEventsDir.AddTerraformFileWithProviderBlock("main.tf")

	variablesDir := root.AddDirectory("variables")
	variablesDir.AddTFVarsFile("default.tfvars")

	ckoDevDir := variablesDir.AddDirectory("cko-dev")
	ckoDevDir.AddTFVarsFile("age.tfvars")
	ckoDevDir.AddTFVarsFile("airflow.tfvars")
	ckoDevDir.AddTFVarsFile("dev.tfvars")
	ckoDevDir.AddTFVarsFile("apm-events.tfvars")

	ckoMgmtDir := variablesDir.AddDirectory("cko-mgmt")
	ckoMgmtDir.AddTFVarsFile("age.tfvars")
	ckoMgmtDir.AddTFVarsFile("airflow.tfvars")
	ckoMgmtDir.AddTFVarsFile("apm-events.tfvars")

	ckoPlaygroundDir := variablesDir.AddDirectory("cko-playground")
	ckoPlaygroundDir.AddTFVarsFile("age.tfvars")
	ckoPlaygroundDir.AddTFVarsFile("airflow.tfvars")
	ckoPlaygroundDir.AddTFVarsFile("apm-events.tfvars")

	ckoProdDir := variablesDir.AddDirectory("cko-prod")
	ckoProdDir.AddTFVarsFile("age.tfvars")
	ckoProdDir.AddTFVarsFile("airflow.tfvars")
	ckoProdDir.AddTFVarsFile("apm-events.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "components-age-dev",
			Path:    "components/age",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-dev/age.tfvars",
					"../../variables/cko-dev/dev.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-age-mgmt",
			Path:    "components/age",
			EnvName: "mgmt",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-mgmt/age.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-age-playground",
			Path:    "components/age",
			EnvName: "playground",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-playground/age.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-age-prod",
			Path:    "components/age",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-prod/age.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-airflow-dev",
			Path:    "components/airflow",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-dev/airflow.tfvars",
					"../../variables/cko-dev/dev.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-airflow-mgmt",
			Path:    "components/airflow",
			EnvName: "mgmt",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-mgmt/airflow.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-airflow-playground",
			Path:    "components/airflow",
			EnvName: "playground",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-playground/airflow.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-airflow-prod",
			Path:    "components/airflow",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-prod/airflow.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-apm-events-dev",
			Path:    "components/apm-events",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-dev/apm-events.tfvars",
					"../../variables/cko-dev/dev.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-apm-events-mgmt",
			Path:    "components/apm-events",
			EnvName: "mgmt",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-mgmt/apm-events.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-apm-events-playground",
			Path:    "components/apm-events",
			EnvName: "playground",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-playground/apm-events.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "components-apm-events-prod",
			Path:    "components/apm-events",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/cko-prod/apm-events.tfvars",
					"../../variables/default.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_Child(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── apps
			├── bar
			│   ├── main.tf
			│   └── envs
			│       ├── prod.tfvars
			│       ├── dev.tfvars
			│       └── default.tfvars
			└── foo
				├── main.tf
				└── envs
					├── staging.tfvars
					├── dev.tfvars
					└── default.tfvars
	*/

	appsDir := root.AddDirectory("apps")

	barDir := appsDir.AddDirectory("bar")
	barDir.AddTerraformFileWithProviderBlock("main.tf")
	barEnvsDir := barDir.AddDirectory("envs")
	barEnvsDir.AddTFVarsFile("prod.tfvars")
	barEnvsDir.AddTFVarsFile("dev.tfvars")
	barEnvsDir.AddTFVarsFile("default.tfvars")

	fooDir := appsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	fooEnvsDir := fooDir.AddDirectory("envs")
	fooEnvsDir.AddTFVarsFile("staging.tfvars")
	fooEnvsDir.AddTFVarsFile("dev.tfvars")
	fooEnvsDir.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "apps-bar-dev",
			Path:    "apps/bar",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/default.tfvars",
					"envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-prod",
			Path:    "apps/bar",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/default.tfvars",
					"envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-dev",
			Path:    "apps/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/default.tfvars",
					"envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-staging",
			Path:    "apps/foo",
			EnvName: "staging",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/default.tfvars",
					"envs/staging.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_ChildWithDot(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── apps
			└── foo
				├── main.tf
				└── envs
					├── staging.eu-west-1.tfvars
					├── dev.eu-west-1.tfvars
					└── default.tfvars
	*/

	appsDir := root.AddDirectory("apps")
	fooDir := appsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	envsDir := fooDir.AddDirectory("envs")
	envsDir.AddTFVarsFile("staging.eu-west-1.tfvars")
	envsDir.AddTFVarsFile("dev.eu-west-1.tfvars")
	envsDir.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "apps-foo-dev",
			Path:    "apps/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/default.tfvars",
					"envs/dev.eu-west-1.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-staging",
			Path:    "apps/foo",
			EnvName: "staging",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/default.tfvars",
					"envs/staging.eu-west-1.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_Cousin(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── infra
			├── components
			│   └── foo
			│       └── main.tf
			├── variables
			│   └── envs
			│       ├── dev
			│       │   └── dev.tfvars
			│       └── prod
			│           └── prod.tfvars
			└── nested
				├── components
				│   └── baz
				│       └── main.tf
				└── variables
					└── envs
						├── stag
						│   └── stag.tfvars
						├── dev
						│   └── dev.tfvars
						└── defaults.tfvars
	*/

	infraDir := root.AddDirectory("infra")

	componentsDir := infraDir.AddDirectory("components")
	fooDir := componentsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")

	variablesDir := infraDir.AddDirectory("variables")
	envsDir := variablesDir.AddDirectory("envs")
	devDir := envsDir.AddDirectory("dev")
	devDir.AddTFVarsFile("dev.tfvars")
	prodDir := envsDir.AddDirectory("prod")
	prodDir.AddTFVarsFile("prod.tfvars")

	nestedDir := infraDir.AddDirectory("nested")
	nestedComponentsDir := nestedDir.AddDirectory("components")
	bazDir := nestedComponentsDir.AddDirectory("baz")
	bazDir.AddTerraformFileWithProviderBlock("main.tf")

	nestedVariablesDir := nestedDir.AddDirectory("variables")
	nestedEnvsDir := nestedVariablesDir.AddDirectory("envs")
	nestedStagDir := nestedEnvsDir.AddDirectory("stag")
	nestedStagDir.AddTFVarsFile("stag.tfvars")
	nestedDevDir := nestedEnvsDir.AddDirectory("dev")
	nestedDevDir.AddTFVarsFile("dev.tfvars")
	nestedEnvsDir.AddTFVarsFile("defaults.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "infra-components-foo-dev",
			Path:    "infra/components/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/envs/dev/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-components-foo-prod",
			Path:    "infra/components/foo",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/envs/prod/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-nested-components-baz-dev",
			Path:    "infra/nested/components/baz",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../variables/envs/dev/dev.tfvars",
					"../../variables/envs/defaults.tfvars",
					"../../variables/envs/dev/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-nested-components-baz-prod",
			Path:    "infra/nested/components/baz",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../variables/envs/prod/prod.tfvars",
					"../../variables/envs/defaults.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-nested-components-baz-stag",
			Path:    "infra/nested/components/baz",
			EnvName: "stag",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/envs/defaults.tfvars",
					"../../variables/envs/stag/stag.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_CousinFlat(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		├── main.tf
		└── variables
			└── envs
				├── dev
				│   └── dev.tfvars
				├── prod
				│   └── prod.tfvars
				└── defaults.tfvars
	*/

	root.AddTerraformFileWithProviderBlock("main.tf")
	variablesDir := root.AddDirectory("variables")
	envsDir := variablesDir.AddDirectory("envs")
	devDir := envsDir.AddDirectory("dev")
	devDir.AddTFVarsFile("dev.tfvars")
	prodDir := envsDir.AddDirectory("prod")
	prodDir.AddTFVarsFile("prod.tfvars")
	envsDir.AddTFVarsFile("defaults.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "main-dev",
			Path:    ".",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"variables/envs/defaults.tfvars",
					"variables/envs/dev/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "main-prod",
			Path:    ".",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"variables/envs/defaults.tfvars",
					"variables/envs/prod/prod.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_DetectedProjects(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── apps
			├── bar
			│   └── main.tf
			├── foo
			│   └── main.tf
			├── dev.tfvars
			├── prod.tfvars
			└── default.tfvars
	*/

	appsDir := root.AddDirectory("apps")
	barDir := appsDir.AddDirectory("bar")
	barDir.AddTerraformFileWithProviderBlock("main.tf")
	fooDir := appsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	appsDir.AddTFVarsFile("dev.tfvars")
	appsDir.AddTFVarsFile("prod.tfvars")
	appsDir.AddTFVarsFile("default.tfvars")

	template := `version: 0.1

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
    terraform_vars:
      environment: {{ $project.Env }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-bar-dev",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
				Vars: map[string]interface{}{
					"environment": "dev",
				},
			},
		},
		{
			Name: "apps-bar-prod",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
				Vars: map[string]interface{}{
					"environment": "prod",
				},
			},
		},
		{
			Name: "apps-foo-dev",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
				Vars: map[string]interface{}{
					"environment": "dev",
				},
			},
		},
		{
			Name: "apps-foo-prod",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
				Vars: map[string]interface{}{
					"environment": "prod",
				},
			},
		},
	})
}

func Test_Generate_DetectedRootModules(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── apps
			├── bar
			│   └── main.tf
			├── foo
			│   └── main.tf
			├── dev.tfvars
			├── prod.tfvars
			└── default.tfvars
	*/

	appsDir := root.AddDirectory("apps")
	barDir := appsDir.AddDirectory("bar")
	barDir.AddTerraformFileWithProviderBlock("main.tf")
	fooDir := appsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	appsDir.AddTFVarsFile("dev.tfvars")
	appsDir.AddTFVarsFile("prod.tfvars")
	appsDir.AddTFVarsFile("default.tfvars")

	template := `version: 0.1
projects:
{{- range $mod := .DetectedRootModules }}
  {{- range $project := $mod.Projects }}
  - path: {{ $mod.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
  {{- end}}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-bar-dev",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-bar-prod",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-dev",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-prod",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
		},
	})
}

func Test_Generate_EnvDirs(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── infra
			├── components
			│   ├── foo
			│   │   └── main.tf
			│   └── baz
			│       └── main.tf
			└── variables
				├── stag
				│   └── bop.tfvars
				├── dev
				│   └── bla.tfvars
				└── defaults.tfvars
	*/

	infraDir := root.AddDirectory("infra")
	componentsDir := infraDir.AddDirectory("components")
	fooDir := componentsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	bazDir := componentsDir.AddDirectory("baz")
	bazDir.AddTerraformFileWithProviderBlock("main.tf")

	variablesDir := infraDir.AddDirectory("variables")
	stagDir := variablesDir.AddDirectory("stag")
	stagDir.AddTFVarsFile("bop.tfvars")
	devDir := variablesDir.AddDirectory("dev")
	devDir.AddTFVarsFile("bla.tfvars")
	variablesDir.AddTFVarsFile("defaults.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "infra-components-baz-dev",
			Path:    "infra/components/baz",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/defaults.tfvars",
					"../../variables/dev/bla.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-components-baz-stag",
			Path:    "infra/components/baz",
			EnvName: "stag",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/defaults.tfvars",
					"../../variables/stag/bop.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-components-foo-dev",
			Path:    "infra/components/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/defaults.tfvars",
					"../../variables/dev/bla.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-components-foo-stag",
			Path:    "infra/components/foo",
			EnvName: "stag",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/defaults.tfvars",
					"../../variables/stag/bop.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_EnvNames(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── apps
			├── bar
			│   └── main.tf
			├── foo
			│   └── main.tf
			├── baz.tfvars
			├── bat.tfvars
			├── network-baz.tfvars
			├── network-bat.tfvars
			├── dev.tfvars
			├── prod.tfvars
			└── default.tfvars
	*/

	appsDir := root.AddDirectory("apps")
	barDir := appsDir.AddDirectory("bar")
	barDir.AddTerraformFileWithProviderBlock("main.tf")
	fooDir := appsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	appsDir.AddTFVarsFile("baz.tfvars")
	appsDir.AddTFVarsFile("bat.tfvars")
	appsDir.AddTFVarsFile("network-baz.tfvars")
	appsDir.AddTFVarsFile("network-bat.tfvars")
	appsDir.AddTFVarsFile("dev.tfvars")
	appsDir.AddTFVarsFile("prod.tfvars")
	appsDir.AddTFVarsFile("default.tfvars")

	template := `version: 0.1
autodetect:
  env_names:
    - baz
    - bat

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-bar-bat",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../bat.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
					"../network-bat.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-bar-baz",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../baz.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
					"../network-baz.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-bat",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../bat.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
					"../network-bat.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-baz",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../baz.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
					"../network-baz.tfvars",
					"../prod.tfvars",
				},
			},
		},
	})
}

func Test_Generate_ParentAndGrandparent(t *testing.T) {

	/*
			.
		└── apps
		    ├── foo
		    │   ├── us1
		    │   │   └── main.tf
		    │   ├── us2
		    │   │   └── main.tf
		    │   ├── dev.tfvars
		    │   ├── prod.tfvars
		    │   └── default.tfvars
		    ├── bar
		    │   ├── us1
		    │   │   └── main.tf
		    │   ├── us2
		    │   │   └── main.tf
		    │   ├── dev.tfvars
		    │   ├── prod.tfvars
		    │   └── default.tfvars
		    ├── dev.tfvars
		    ├── prod.tfvars
		    └── default.tfvars
	*/

	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	// Create foo structure
	foo := apps.AddDirectory("foo")
	fooUs1 := foo.AddDirectory("us1")
	fooUs1.AddTerraformFileWithProviderBlock("main.tf")
	fooUs2 := foo.AddDirectory("us2")
	fooUs2.AddTerraformFileWithProviderBlock("main.tf")
	foo.AddTFVarsFile("dev.tfvars")
	foo.AddTFVarsFile("prod.tfvars")
	foo.AddTFVarsFile("default.tfvars")

	// Create bar structure
	bar := apps.AddDirectory("bar")
	barUs1 := bar.AddDirectory("us1")
	barUs1.AddTerraformFileWithProviderBlock("main.tf")
	barUs2 := bar.AddDirectory("us2")
	barUs2.AddTerraformFileWithProviderBlock("main.tf")
	bar.AddTFVarsFile("dev.tfvars")
	bar.AddTFVarsFile("prod.tfvars")
	bar.AddTFVarsFile("default.tfvars")

	// Add grandparent tfvars
	apps.AddTFVarsFile("dev.tfvars")
	apps.AddTFVarsFile("prod.tfvars")
	apps.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "apps-bar-us1-dev",
			Path: "apps/bar/us1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-us1-prod",
			Path: "apps/bar/us1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
					"../default.tfvars",
					"../prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-us2-dev",
			Path: "apps/bar/us2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-us2-prod",
			Path: "apps/bar/us2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
					"../default.tfvars",
					"../prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-us1-dev",
			Path: "apps/foo/us1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-us1-prod",
			Path: "apps/foo/us1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
					"../default.tfvars",
					"../prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-us2-dev",
			Path: "apps/foo/us2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-us2-prod",
			Path: "apps/foo/us2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
					"../default.tfvars",
					"../prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
	})
}

func Test_Generate_PathOverrides(t *testing.T) {
	root := NewFilesystem(t)
	infra := root.AddDirectory("infra")
	components := infra.AddDirectory("components")

	// Create foo directory with auto tfvars
	foo := components.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	foo.AddTFVarsFile("var.auto.tfvars")

	// Create blah directory
	blah := components.AddDirectory("blah")
	blah.AddTerraformFileWithProviderBlock("main.tf")

	// Create bar directory
	bar := components.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")

	// Add tfvars files to infra directory
	infra.AddTFVarsFile("baz.tfvars")
	infra.AddTFVarsFile("bat.tfvars")
	infra.AddTFVarsFile("bip.tfvars")
	infra.AddTFVarsFile("network-baz.tfvars")
	infra.AddTFVarsFile("network-bat.tfvars")
	infra.AddTFVarsFile("default.tfvars")

	template := `version: 0.1
autodetect:
  env_names:
    - baz
    - bat
    - bip
  path_overrides:
    - path: "**/**"
      exclude:
        - baz
    - path: infra/components/foo
      only:
        - baz
    - path: infra/**/bar
      exclude:
        - bat
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "infra-components-bar-bip",
			Path: "infra/components/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../bip.tfvars",
					"../../default.tfvars",
				},
				Workspace: "bip",
			},
			EnvName: "bip",
			Type:    "terraform",
		},
		{
			Name: "infra-components-blah-bat",
			Path: "infra/components/blah",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../bat.tfvars",
					"../../default.tfvars",
					"../../network-bat.tfvars",
				},
				Workspace: "bat",
			},
			EnvName: "bat",
			Type:    "terraform",
		},
		{
			Name: "infra-components-blah-bip",
			Path: "infra/components/blah",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../bip.tfvars",
					"../../default.tfvars",
				},
				Workspace: "bip",
			},
			EnvName: "bip",
			Type:    "terraform",
		},
		{
			Name: "infra-components-foo-baz",
			Path: "infra/components/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../baz.tfvars",
					"../../default.tfvars",
					"../../network-baz.tfvars",
					"var.auto.tfvars",
				},
				Workspace: "baz",
			},
			EnvName: "baz",
			Type:    "terraform",
		},
	})
}

func Test_Generate_PreferFolderName(t *testing.T) {
	root := NewFilesystem(t)
	infra := root.AddDirectory("infra")
	infra.AddTerraformFileWithProviderBlock("main.tf")

	envs := infra.AddDirectory("envs")
	stg := envs.AddDirectory("stg")
	stg.AddTFVarsFile("dev.tfvars")

	qa := envs.AddDirectory("qa")
	qa.AddTFVarsFile("prod.tfvars")

	template := `version: 0.1
autodetect:
  env_names:
    - qa
    - dev
    - prod
    - stg
  prefer_folder_name_for_env: true
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "infra-qa",
			Path: "infra",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/qa/prod.tfvars",
				},
				Workspace: "qa",
			},
			EnvName: "qa",
			Type:    "terraform",
		},
		{
			Name: "infra-stg",
			Path: "infra",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/stg/dev.tfvars",
				},
				Workspace: "stg",
			},
			EnvName: "stg",
			Type:    "terraform",
		},
	})
}

func Test_Generate_RepoName(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	bar := apps.AddDirectory("bar")
	bar.AddFile("backend.tf").Content(`terraform {
  backend "s3" {}
}`)
	bar.AddTFVarsFile("terraform.tfvars")

	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	foo.AddTFVarsFile("dev.tfvars")
	foo.AddTFVarsFile("prod.tfvars")
	foo.AddTFVarsFile("terraform.tfvars")

	template := `version: 0.1

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_vars:
    {{- if eq $.RepoName "infracost/infracost"}}
      environment: prod
    {{- end}}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-bar",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				Vars: map[string]interface{}{
					"environment": "prod",
				},
			},
		},
		{
			Name: "apps-foo-dev",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				Vars: map[string]interface{}{
					"environment": "prod",
				},
			},
		},
		{
			Name: "apps-foo-prod",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				Vars: map[string]interface{}{
					"environment": "prod",
				},
			},
		},
	}, config.WithRepoName("infracost/infracost"))
}

func Test_Generate_RootPaths(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	bar := apps.AddDirectory("bar")
	bar.AddFile("backend.tf").Content(`terraform {
  backend "s3" {}
}`)
	bar.AddTFVarsFile("terraform.tfvars")

	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	foo.AddTFVarsFile("dev.tfvars")
	foo.AddTFVarsFile("prod.tfvars")
	foo.AddTFVarsFile("terraform.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "apps-bar",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"terraform.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name: "apps-foo-dev",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"dev.tfvars",
					"terraform.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-prod",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"prod.tfvars",
					"terraform.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
	})
}

func Test_Generate_RootWithLeafs(t *testing.T) {
	root := NewFilesystem(t)
	terraform := root.AddDirectory("terraform")
	terraform.AddTerraformFileWithProviderBlock("main.tf")

	configDir := terraform.AddDirectory("config")
	dev := configDir.AddDirectory("dev")
	dev.AddTFVarsFile("terraform.tfvars")
	prod := configDir.AddDirectory("prod")
	prod.AddTFVarsFile("terraform.tfvars")

	foo := terraform.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	fooDev := foo.AddDirectory("dev")
	fooDev.AddTFVarsFile("terraform.tfvars")
	fooProd := foo.AddDirectory("prod")
	fooProd.AddTFVarsFile("terraform.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "terraform-dev",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"config/dev/terraform.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "terraform-prod",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"config/prod/terraform.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "terraform-foo-dev",
			Path: "terraform/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"dev/terraform.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "terraform-foo-prod",
			Path: "terraform/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"prod/terraform.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
	})
}

func Test_Generate_Sibling(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")

	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	envs := apps.AddDirectory("envs")
	envs.AddTFVarsFile("prod.tfvars")
	envs.AddTFVarsFile("dev.tfvars")
	envs.AddTFVarsFile("shared.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "apps-bar-dev",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/dev.tfvars",
					"../envs/shared.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-prod",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/prod.tfvars",
					"../envs/shared.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-dev",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/dev.tfvars",
					"../envs/shared.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-prod",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/prod.tfvars",
					"../envs/shared.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
	})
}

func Test_Generate_SingleNestedProject(t *testing.T) {
	root := NewFilesystem(t)
	app := root.AddDirectory("app")
	foo := app.AddDirectory("foo")
	foo.AddFile("test.tf").Content(`resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}`)

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "app-foo",
			Path: "app/foo",
			Type: "terraform",
		},
	})
}

func Test_Generate_SingleProject(t *testing.T) {
	root := NewFilesystem(t)
	root.AddTerraformFileWithProviderBlock("main.tf")
	root.AddTFVarsFile("prod.tfvars")
	root.AddTFVarsFile("dev.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "main-dev",
			Path: ".",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "main-prod",
			Path: ".",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
	})
}

func Test_Generate_SingleRootProject(t *testing.T) {
	root := NewFilesystem(t)
	root.AddFile("test.tf").Content(`resource "aws_instance" "example" {
  ami           = "ami-0c55b159cbfafe1d0"
  instance_type = "t2.micro"
}`)

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "main",
			Path: ".",
			Type: "terraform",
		},
	})
}

func Test_Generate_Terragrunt(t *testing.T) {
	root := NewFilesystem(t)
	root.AddTerragruntFile()

	apps := root.AddDirectory("apps")

	bar := apps.AddDirectory("bar")
	bar.AddTerragruntFile()

	foo := apps.AddDirectory("foo")
	foo.AddTerragruntFileIncludingParentDir()

	baz := apps.AddDirectory("baz")
	baz.AddTerragruntFile()

	bip := baz.AddDirectory("bip")
	bip.AddTerragruntFileIncludingParentDir()

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "apps-bar",
			Path: "apps/bar",
			Type: "terragrunt",
		},
		{
			Name: "apps-baz-bip",
			Path: "apps/baz/bip",
			Type: "terragrunt",
			DependencyPaths: []string{
				"apps/baz/terragrunt.hcl",
			},
		},
		{
			Name: "apps-foo",
			Path: "apps/foo",
			Type: "terragrunt",
			DependencyPaths: []string{
				"terragrunt.hcl",
			},
		},
	})
}

func Test_Generate_SharedEnvVarFiles(t *testing.T) {

	/*
		└── apps
		    ├── foo
		    │   ├── main.tf
		    ├── bar
		    │   ├── main.tf
		    │   ├── staging.tfvars
		    │   └── dev.tfvars
		    ├── prod-default.tfvars
		    ├── staging-default.tfvars
		    ├── dev-default.tfvars
		    └── default.tfvars
	*/

	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	bar.AddTFVarsFile("staging.tfvars")
	bar.AddTFVarsFile("dev.tfvars")

	// Add shared env var files at apps level
	apps.AddTFVarsFile("prod-default.tfvars")
	apps.AddTFVarsFile("staging-default.tfvars")
	apps.AddTFVarsFile("dev-default.tfvars")
	apps.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "apps-bar-dev",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev-default.tfvars",
					"dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-staging",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../staging-default.tfvars",
					"staging.tfvars",
				},
				Workspace: "staging",
			},
			EnvName: "staging",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-dev",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev-default.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-prod",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod-default.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-staging",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../staging-default.tfvars",
				},
				Workspace: "staging",
			},
			EnvName: "staging",
			Type:    "terraform",
		},
	})
}

func Test_Generate_SiblingAndAunt(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	// Add sibling envs (at apps level)
	appsEnvs := apps.AddDirectory("envs")
	appsEnvs.AddTFVarsFile("dev.tfvars")
	appsEnvs.AddTFVarsFile("prod.tfvars")
	appsEnvs.AddTFVarsFile("default.tfvars")

	// Create foo structure with aunt envs
	foo := apps.AddDirectory("foo")
	fooEnvs := foo.AddDirectory("envs")
	fooEnvs.AddTFVarsFile("dev.tfvars")
	fooEnvs.AddTFVarsFile("prod.tfvars")
	fooEnvs.AddTFVarsFile("default.tfvars")

	fooUs1 := foo.AddDirectory("us1")
	fooUs1.AddTerraformFileWithProviderBlock("main.tf")
	fooUs2 := foo.AddDirectory("us2")
	fooUs2.AddTerraformFileWithProviderBlock("main.tf")

	// Create bar structure with aunt envs
	bar := apps.AddDirectory("bar")
	barEnvs := bar.AddDirectory("envs")
	barEnvs.AddTFVarsFile("dev.tfvars")
	barEnvs.AddTFVarsFile("prod.tfvars")
	barEnvs.AddTFVarsFile("default.tfvars")

	barUs1 := bar.AddDirectory("us1")
	barUs1.AddTerraformFileWithProviderBlock("main.tf")
	barUs2 := bar.AddDirectory("us2")
	barUs2.AddTerraformFileWithProviderBlock("main.tf")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "apps-bar-us1-dev",
			Path: "apps/bar/us1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
					"../envs/default.tfvars",
					"../envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-us1-prod",
			Path: "apps/bar/us1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
					"../envs/default.tfvars",
					"../envs/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-us2-dev",
			Path: "apps/bar/us2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
					"../envs/default.tfvars",
					"../envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-us2-prod",
			Path: "apps/bar/us2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
					"../envs/default.tfvars",
					"../envs/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-us1-dev",
			Path: "apps/foo/us1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
					"../envs/default.tfvars",
					"../envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-us1-prod",
			Path: "apps/foo/us1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
					"../envs/default.tfvars",
					"../envs/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-us2-dev",
			Path: "apps/foo/us2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev.tfvars",
					"../envs/default.tfvars",
					"../envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-us2-prod",
			Path: "apps/foo/us2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod.tfvars",
					"../envs/default.tfvars",
					"../envs/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
	})
}

func Test_Generate_SiblingAndChildDefault(t *testing.T) {
	/*
	   # If there is ambiguity whether to link root modules to var files in sibling
	   # or child directories we should default to child. In the future we could
	   # update this logic to read the var file contents and see if the vars match
	   # the root module
	   .
	   └── apps
	       ├── main.tf
	       ├── foo
	       │   └── main.tf
	       ├── bar
	       │   └── main.tf
	       └── envs
	           ├── dev.tfvars
	           └── prod.tfvars

	*/

	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")
	apps.AddTerraformFileWithProviderBlock("main.tf")

	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")

	envs := apps.AddDirectory("envs")
	envs.AddTFVarsFile("dev.tfvars")
	envs.AddTFVarsFile("prod.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "apps-dev",
			Path: "apps",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-prod",
			Path: "apps",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-bar",
			Path: "apps/bar",
			Type: "terraform",
		},
		{
			Name: "apps-foo",
			Path: "apps/foo",
			Type: "terraform",
		},
	})
}

func Test_Generate_SiblingAndChildPreferChild(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")
	apps.AddTerraformFileWithProviderBlock("main.tf")

	// Add sibling envs at apps level
	appsEnvs := apps.AddDirectory("envs")
	appsEnvs.AddTFVarsFile("dev.tfvars")
	appsEnvs.AddTFVarsFile("prod.tfvars")

	// Add foo with child envs
	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	fooEnvs := foo.AddDirectory("envs")
	fooEnvs.AddTFVarsFile("dev.tfvars")

	// Add bar with child envs
	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	barEnvs := bar.AddDirectory("envs")
	barEnvs.AddTFVarsFile("prod.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "apps-dev",
			Path: "apps",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-prod",
			Path: "apps",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-prod",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-dev",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
	})
}

func Test_Generate_SiblingAndChildPreferSibling(t *testing.T) {

	/*
	   # We should link root modules with var files in sibling directories if further
	   # up the hierarchy there are more var files in sibling directories
	   .
	   ├── apps
	   │   ├── main.tf
	   │   ├── foo
	   │   │   └── main.tf
	   │   ├── bar
	   │   │   └── main.tf
	   │   └── envs
	   │       ├── dev.tfvars
	   │       └── staging.tfvars
	   └── envs
	       ├── dev.tfvars
	       └── prod.tfvars
	*/

	root := NewFilesystem(t)

	// Add root-level envs (sibling to apps)
	rootEnvs := root.AddDirectory("envs")
	rootEnvs.AddTFVarsFile("dev.tfvars")
	rootEnvs.AddTFVarsFile("prod.tfvars")

	apps := root.AddDirectory("apps")
	apps.AddTerraformFileWithProviderBlock("main.tf")

	// Add sibling envs at apps level
	appsEnvs := apps.AddDirectory("envs")
	appsEnvs.AddTFVarsFile("dev.tfvars")
	appsEnvs.AddTFVarsFile("staging.tfvars")

	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "apps-dev",
			Path: "apps",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-prod",
			Path: "apps",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-dev",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-bar-staging",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/staging.tfvars",
				},
				Workspace: "staging",
			},
			EnvName: "staging",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-dev",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-foo-staging",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/staging.tfvars",
				},
				Workspace: "staging",
			},
			EnvName: "staging",
			Type:    "terraform",
		},
	})
}

func Test_Generate_WithNoDirectoriesWithProviders(t *testing.T) {
	root := NewFilesystem(t)

	modules := root.AddDirectory("modules")

	foo := modules.AddDirectory("foo")
	foo.AddTerraformWithNoBackendOrProvider("test.tf")

	bar := modules.AddDirectory("bar")
	bar.AddTerraformWithNoBackendOrProvider("test.tf")

	baz := modules.AddDirectory("baz")
	baz.AddTerraformWithNoBackendOrProvider("test.tf")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "modules-bar",
			Path: "modules/bar",
			Type: "terraform",
		},
		{
			Name: "modules-baz",
			Path: "modules/baz",
			Type: "terraform",
		},
		{
			Name: "modules-foo",
			Path: "modules/foo",
			Type: "terraform",
		},
	})
}

func Test_Generate_ParentFilenameMatchesProject(t *testing.T) {
	root := NewFilesystem(t)

	infra := root.AddDirectory("infra")
	infra.AddTFVarsFile("baz.tfvars")
	infra.AddTFVarsFile("foo.tfvars")

	components := infra.AddDirectory("components")
	components.AddTFVarsFile("defaults.tfvars")
	components.AddTFVarsFile("foo.tfvars")
	components.AddTFVarsFile("bar.tfvars")

	foo := components.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	bar := components.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")

	baz := components.AddDirectory("baz")
	baz.AddTerraformFileWithProviderBlock("main.tf")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "infra-components-bar",
			Path: "infra/components/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../bar.tfvars",
					"../defaults.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name: "infra-components-baz",
			Path: "infra/components/baz",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../baz.tfvars",
					"../defaults.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name: "infra-components-foo",
			Path: "infra/components/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../foo.tfvars",
					"../defaults.tfvars",
					"../foo.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_TerragruntAndTerraformMixed(t *testing.T) {
	/*
		└── apps
		    ├── bar
		    │   └── terragrunt.hcl
		    ├── foo
		    │   └── terragrunt.hcl
		    ├── baz
		    │   ├── terragrunt.hcl
		    │   └── bip
		    │       └── terragrunt.hcl.json
		    ├── _module
		    │   └── main.tf
		    ├── fez
		    │   └── main.tf
		    └── envs
		        ├── prod.tfvars
		        └── dev.tfvars
	*/

	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	// Terragrunt projects
	bar := apps.AddDirectory("bar")
	bar.AddTerragruntFile()

	foo := apps.AddDirectory("foo")
	foo.AddTerragruntFile()

	baz := apps.AddDirectory("baz")
	baz.AddTerragruntFile()

	bip := baz.AddDirectory("bip")
	bip.AddTerragruntFileIncludingParentDir()

	// Terraform module (should be ignored due to _module name)
	module := apps.AddDirectory("_module")
	module.AddTerraformFileWithProviderBlock("main.tf")

	// Terraform project (should be included due to include_dirs filter)
	fez := apps.AddDirectory("fez")
	fez.AddTerraformFileWithProviderBlock("main.tf")

	// Environment vars
	envs := apps.AddDirectory("envs")
	envs.AddTFVarsFile("prod.tfvars")
	envs.AddTFVarsFile("dev.tfvars")

	template := `version: 0.1
autodetect:
  include_dirs:
    - '**/fez'
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-bar",
			Path: "apps/bar",
			Type: "terragrunt",
		},
		{
			Name: "apps-baz-bip",
			Path: "apps/baz/bip",
			Type: "terragrunt",
			DependencyPaths: []string{
				"apps/baz/terragrunt.hcl",
			},
		},
		{
			Name: "apps-fez-dev",
			Path: "apps/fez",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/dev.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "apps-fez-prod",
			Path: "apps/fez",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "apps-foo",
			Path: "apps/foo",
			Type: "terragrunt",
		},
	})
}

func Test_Generate_TfvarJson(t *testing.T) {
	root := NewFilesystem(t)

	terraform := root.AddDirectory("terraform")
	terraform.AddTerraformFileWithProviderBlock("main.tf")

	variables := root.AddDirectory("variables")
	env := variables.AddDirectory("env")

	prod := env.AddDirectory("prod")
	prod.AddTFVarsJSONFile("tfvars.json")

	dev := env.AddDirectory("dev")
	dev.AddTFVarsJSONFile("tfvars.json")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "terraform-dev",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../variables/env/dev/tfvars.json",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "terraform-prod",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../variables/env/prod/tfvars.json",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
	})
}

func Test_Generate_WildcardEnvNames(t *testing.T) {
	root := NewFilesystem(t)
	terraform := root.AddDirectory("terraform")
	terraform.AddTerraformFileWithProviderBlock("main.tf")

	env := terraform.AddDirectory("env")
	env.AddTFVarsFile("ops-prod-foo.tfvars")
	env.AddTFVarsFile("ops-prod-bar.tfvars")
	env.AddTFVarsFile("ops-dev.tfvars")
	env.AddTFVarsFile("conf-dev-foo.tfvars")
	env.AddTFVarsFile("dev.tfvars")
	env.AddTFVarsFile("uk-uat.tfvars")
	env.AddTFVarsFile("us-uat.tfvars")
	env.AddTFVarsFile("other-uat.tfvars")
	env.AddTFVarsFile("conf-prod-foo.tfvars")

	template := `version: 0.1
autodetect:
  env_names:
    - ops-*
    - conf-*
    - dev
    - ??-uat
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "terraform-conf-dev-foo",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/conf-dev-foo.tfvars",
					"env/other-uat.tfvars",
				},
				Workspace: "conf-dev-foo",
			},
			EnvName: "conf-dev-foo",
			Type:    "terraform",
		},
		{
			Name: "terraform-conf-prod-foo",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/conf-prod-foo.tfvars",
					"env/other-uat.tfvars",
				},
				Workspace: "conf-prod-foo",
			},
			EnvName: "conf-prod-foo",
			Type:    "terraform",
		},
		{
			Name: "terraform-dev",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/dev.tfvars",
					"env/other-uat.tfvars",
				},
				Workspace: "dev",
			},
			EnvName: "dev",
			Type:    "terraform",
		},
		{
			Name: "terraform-ops-dev",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/ops-dev.tfvars",
					"env/other-uat.tfvars",
				},
				Workspace: "ops-dev",
			},
			EnvName: "ops-dev",
			Type:    "terraform",
		},
		{
			Name: "terraform-ops-prod-bar",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/ops-prod-bar.tfvars",
					"env/other-uat.tfvars",
				},
				Workspace: "ops-prod-bar",
			},
			EnvName: "ops-prod-bar",
			Type:    "terraform",
		},
		{
			Name: "terraform-ops-prod-foo",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/ops-prod-foo.tfvars",
					"env/other-uat.tfvars",
				},
				Workspace: "ops-prod-foo",
			},
			EnvName: "ops-prod-foo",
			Type:    "terraform",
		},
		{
			Name: "terraform-uk-uat",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/other-uat.tfvars",
					"env/uk-uat.tfvars",
				},
				Workspace: "uk-uat",
			},
			EnvName: "uk-uat",
			Type:    "terraform",
		},
		{
			Name: "terraform-us-uat",
			Path: "terraform",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/other-uat.tfvars",
					"env/us-uat.tfvars",
				},
				Workspace: "us-uat",
			},
			EnvName: "us-uat",
			Type:    "terraform",
		},
	})
}

func Test_Generate_EnvNamesCaseInsensitive(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── apps
			├── bar
			│   └── main.tf
			├── foo
			│   └── main.tf
			├── baz.tfvars
			├── bat.tfvars
			├── network-Baz.tfvars
			├── network-Bat.tfvars
			├── dev.tfvars
			├── prod.tfvars
			└── default.tfvars
	*/

	appsDir := root.AddDirectory("apps")
	barDir := appsDir.AddDirectory("bar")
	barDir.AddTerraformFileWithProviderBlock("main.tf")
	fooDir := appsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	appsDir.AddTFVarsFile("baz.tfvars")
	appsDir.AddTFVarsFile("bat.tfvars")
	appsDir.AddTFVarsFile("network-Baz.tfvars")
	appsDir.AddTFVarsFile("network-Bat.tfvars")
	appsDir.AddTFVarsFile("dev.tfvars")
	appsDir.AddTFVarsFile("prod.tfvars")
	appsDir.AddTFVarsFile("default.tfvars")

	template := `version: 0.1
autodetect:
  env_names:
    - baz
    - bat

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-bar-bat",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../bat.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
					"../network-Bat.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-bar-baz",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../baz.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
					"../network-Baz.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-bat",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../bat.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
					"../network-Bat.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-baz",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../baz.tfvars",
					"../default.tfvars",
					"../dev.tfvars",
					"../network-Baz.tfvars",
					"../prod.tfvars",
				},
			},
		},
	})
}

func Test_Generate_EnvNamesInPath(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── infra
			├── components
			│   ├── foo-dev
			│   │   └── main.tf
			│   └── foo-prod
			│       └── main.tf
			└── variables
				├── dev
				│   └── foo-dev.tfvars
				├── prod
				│   └── foo-prod.tfvars
				├── qa
				│   └── bar.tfvars
				└── defaults.tfvars
	*/

	infraDir := root.AddDirectory("infra")
	componentsDir := infraDir.AddDirectory("components")
	fooDevDir := componentsDir.AddDirectory("foo-dev")
	fooDevDir.AddTerraformFileWithProviderBlock("main.tf")
	fooProdDir := componentsDir.AddDirectory("foo-prod")
	fooProdDir.AddTerraformFileWithProviderBlock("main.tf")

	variablesDir := infraDir.AddDirectory("variables")
	devDir := variablesDir.AddDirectory("dev")
	devDir.AddTFVarsFile("foo-dev.tfvars")
	prodDir := variablesDir.AddDirectory("prod")
	prodDir.AddTFVarsFile("foo-prod.tfvars")
	qaDir := variablesDir.AddDirectory("qa")
	qaDir.AddTFVarsFile("bar.tfvars")
	variablesDir.AddTFVarsFile("defaults.tfvars")

	template := `version: 0.1
autodetect:
  env_names:
   - dev
   - prod
   - qa

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "infra-components-foo-dev",
			Path: "infra/components/foo-dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/defaults.tfvars",
					"../../variables/dev/foo-dev.tfvars",
				},
			},
		},
		{
			Name: "infra-components-foo-prod",
			Path: "infra/components/foo-prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../variables/defaults.tfvars",
					"../../variables/prod/foo-prod.tfvars",
				},
			},
		},
	})
}

func Test_Generate_EnvNamesPartials(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── apps
			├── bar
			│   └── main.tf
			├── foo
			│   └── main.tf
			├── network-baz.tfvars
			├── network-bat.tfvars
			├── dev.tfvars
			├── prod.tfvars
			└── default.tfvars
	*/

	appsDir := root.AddDirectory("apps")
	barDir := appsDir.AddDirectory("bar")
	barDir.AddTerraformFileWithProviderBlock("main.tf")
	fooDir := appsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	appsDir.AddTFVarsFile("network-baz.tfvars")
	appsDir.AddTFVarsFile("network-bat.tfvars")
	appsDir.AddTFVarsFile("dev.tfvars")
	appsDir.AddTFVarsFile("prod.tfvars")
	appsDir.AddTFVarsFile("default.tfvars")

	template := `version: 0.1
autodetect:
  env_names:
    - baz
    - bat

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-bar-bat",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
					"../network-bat.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-bar-baz",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
					"../network-baz.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-bat",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
					"../network-bat.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-baz",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
					"../network-baz.tfvars",
					"../prod.tfvars",
				},
			},
		},
	})
}

func Test_Generate_EnvVarDotExtensions(t *testing.T) {
	root := NewFilesystem(t)

	/*
		.
		└── apps
			├── bar
			│   └── main.tf
			├── foo
			│   └── main.tf
			├── .dev-custom-ext
			├── .prod-custom-ext
			├── .config.prod.env.tfvars
			├── .config.dev.env.tfvars
			└── default.tfvars
	*/

	appsDir := root.AddDirectory("apps")
	barDir := appsDir.AddDirectory("bar")
	barDir.AddTerraformFileWithProviderBlock("main.tf")
	fooDir := appsDir.AddDirectory("foo")
	fooDir.AddTerraformFileWithProviderBlock("main.tf")
	appsDir.AddTFVarsFile(".dev-custom-ext")
	appsDir.AddTFVarsFile(".prod-custom-ext")
	appsDir.AddTFVarsFile(".config.prod.env.tfvars")
	appsDir.AddTFVarsFile(".config.dev.env.tfvars")
	appsDir.AddTFVarsFile("default.tfvars")

	template := `version: 0.1
autodetect:
  env_names:
    - baz
    - dev
    - prod
  terraform_var_file_extensions:
    - ".tfvars"
    - ".env.tfvars"
    - ""

`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name:    "apps-bar-dev",
			Path:    "apps/bar",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../.config.dev.env.tfvars",
					"../.dev-custom-ext",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-prod",
			Path:    "apps/bar",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../.config.prod.env.tfvars",
					"../.prod-custom-ext",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-dev",
			Path:    "apps/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../.config.dev.env.tfvars",
					"../.dev-custom-ext",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-prod",
			Path:    "apps/foo",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../.config.prod.env.tfvars",
					"../.prod-custom-ext",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_EnvVarDots(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")
	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	apps.AddFile(".network-baz.tfvars")
	apps.AddFile(".network-bat.tfvars")
	apps.AddTFVarsFile("config.dev.tfvars")
	apps.AddTFVarsFile("config.prod.tfvars")
	apps.AddFile(".dev.tfvars")
	apps.AddFile(".prod.tfvars")
	apps.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "apps-bar-dev",
			Path:    "apps/bar",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../.dev.tfvars",
					"../.network-bat.tfvars",
					"../.network-baz.tfvars",
					"../config.dev.tfvars",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-prod",
			Path:    "apps/bar",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../.network-bat.tfvars",
					"../.network-baz.tfvars",
					"../.prod.tfvars",
					"../config.prod.tfvars",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-dev",
			Path:    "apps/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../.dev.tfvars",
					"../.network-bat.tfvars",
					"../.network-baz.tfvars",
					"../config.dev.tfvars",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-prod",
			Path:    "apps/foo",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../.network-bat.tfvars",
					"../.network-baz.tfvars",
					"../.prod.tfvars",
					"../config.prod.tfvars",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_EmptyExtensions(t *testing.T) {
	root := NewFilesystem(t)
	// we add a TF Vars file but don't expect it to be scanned.
	root.AddTFVarsFile("program.dll")
	root.AddTFVarsFile("prod")
	root.AddTerraformFileWithProviderBlock("main.tf")

	template := `version: 0.1
autodetect:
  env_names:
    - prod
  terraform_var_file_extensions:
    - ""
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name:    "main-prod",
			Path:    ".",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{"prod"},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_EnvVarExtensions(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")
	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	apps.AddTFVarsFile("baz-custom-ext")
	apps.AddFile("Jenkinsfile")
	apps.AddTFVarsFile("dev.tfvars")
	apps.AddTFVarsJSONFile("common.tfvars.json")
	apps.AddFile("empty-file")
	apps.AddTFVarsFile("prod-custom-ext")
	apps.AddTFVarsFile("prod.env.tfvars")
	apps.AddTFVarsFile("default.tfvars")

	template := `version: 0.1
autodetect:
  env_names:
    - baz
    - dev
    - prod
  terraform_var_file_extensions:
    - ".tfvars"
    - ".env.tfvars"
    - ""

`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name:    "apps-bar-baz",
			Path:    "apps/bar",
			EnvName: "baz",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../baz-custom-ext",
					"../common.tfvars.json",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-dev",
			Path:    "apps/bar",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../common.tfvars.json",
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-prod",
			Path:    "apps/bar",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../common.tfvars.json",
					"../default.tfvars",
					"../prod-custom-ext",
					"../prod.env.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-baz",
			Path:    "apps/foo",
			EnvName: "baz",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../baz-custom-ext",
					"../common.tfvars.json",
					"../default.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-dev",
			Path:    "apps/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../common.tfvars.json",
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-prod",
			Path:    "apps/foo",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../common.tfvars.json",
					"../default.tfvars",
					"../prod-custom-ext",
					"../prod.env.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_ExcludedDirs(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")
	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	baz := apps.AddDirectory("baz")
	baz.AddTerraformWithNoBackendOrProvider("foo.tf")
	bat := apps.AddDirectory("bat")
	bat.AddTerraformWithNoBackendOrProvider("bip.tf")

	apps.AddTFVarsFile("dev.tfvars")
	apps.AddTFVarsFile("prod.tfvars")
	apps.AddTFVarsFile("default.tfvars")

	template := `version: 0.1
autodetect:
  exclude_dirs:
    - apps/foo

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-bar-dev",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-bar-prod",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
		},
	})
}

func Test_Generate_ExcludedDirsGlob(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")
	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	fooBar := foo.AddDirectory("bar")
	fooBar.AddTerraformFileWithProviderBlock("main.tf")
	fooBat := fooBar.AddDirectory("bat")
	fooBat.AddTerraformFileWithProviderBlock("main.tf")

	template := `version: 0.1
autodetect:
  exclude_dirs:
    - "apps/foo/**"
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-bar",
			Path: "apps/bar",
			Type: "terraform",
		},
		{
			Name: "apps-foo",
			Path: "apps/foo",
			Type: "terraform",
		},
	})
}

func Test_Generate_ExcludesExamples(t *testing.T) {
	root := NewFilesystem(t)
	examples := root.AddDirectory("examples")
	examplesFoo := examples.AddDirectory("foo")
	examplesFoo.AddTerraformFileWithProviderBlock("main.tf")

	infra := root.AddDirectory("infra")
	infra.AddTerraformWithNoBackendOrProvider("blank.tf")
	infraExamples := infra.AddDirectory("examples")
	infraExamplesBar := infraExamples.AddDirectory("bar")
	infraExamplesBar.AddTerraformFileWithProviderBlock("main.tf")
	infraExamplesBaz := infraExamples.AddDirectory("baz")
	infraExamplesBaz.AddTerraformFileWithProviderBlock("main.tf")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "infra",
			Path: "infra",
			Type: "terraform",
		},
	})
}

func Test_Generate_ExternalTfvars(t *testing.T) {
	/*
	   .
	   └── ./
	       ├── apps/
	       │   └── foo/
	       │       └── main.tf
	       ├── infra/
	       │   └── db/
	       │       └── main.tf
	       └── envs/
	           ├── default.tfvars
	           ├── dev/
	           │   └── dev.tfvars
	           ├── test/
	           │   └── test.tfvars
	           └── prod/
	               └── prod.tfvars
	*/

	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")
	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	infra := root.AddDirectory("infra")
	db := infra.AddDirectory("db")
	db.AddTerraformFileWithProviderBlock("main.tf")

	envs := root.AddDirectory("envs")
	envs.AddTFVarsFile("default.tfvars")
	dev := envs.AddDirectory("dev")
	dev.AddTFVarsFile("dev.tfvars")
	test := envs.AddDirectory("test")
	test.AddTFVarsFile("test.tfvars")
	prod := envs.AddDirectory("prod")
	prod.AddTFVarsFile("prod.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "apps-foo-dev",
			Path:    "apps/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-prod",
			Path:    "apps/foo",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-test",
			Path:    "apps/foo",
			EnvName: "test",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/test/test.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-db-dev",
			Path:    "infra/db",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/dev/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-db-prod",
			Path:    "infra/db",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/prod/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-db-test",
			Path:    "infra/db",
			EnvName: "test",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"../../envs/test/test.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_ForceProjectType(t *testing.T) {
	root := NewFilesystem(t)
	dup := root.AddDirectory("dup")
	dup.AddTerragruntFileIncludingParentDir()
	dup.AddTerraformFileWithProviderBlock("main.tf")
	dup.AddTFVarsFile("prod.tfvars")
	dup.AddTFVarsFile("dev.tfvars")

	nondup := root.AddDirectory("nondup")
	nondup.AddTerragruntFileIncludingParentDir()

	template := `version: 0.1
autodetect:
  force_project_type: terraform
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name:    "dup-dev",
			Path:    "dup",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"dev.tfvars",
				},
			},
			Type: "terragrunt",
		},
		{
			Name:    "dup-prod",
			Path:    "dup",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"prod.tfvars",
				},
			},
			Type: "terragrunt",
		},
		{
			Name: "nondup",
			Path: "nondup",
			Type: "terragrunt",
		},
	})
}

func Test_Generate_ForceProjectTypeNoValidProject(t *testing.T) {
	root := NewFilesystem(t)
	dup := root.AddDirectory("dup")
	dup.AddTerragruntFile()
	dup.AddFile("blank.tf")
	dup.AddTFVarsFile("prod.tfvars")
	dup.AddTFVarsFile("dev.tfvars")

	template := `version: 0.1
autodetect:
  force_project_type: terraform
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name:    "dup-dev",
			Path:    "dup",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"dev.tfvars",
				},
				Workspace: "dev",
			},
			Type: "terragrunt",
		},
		{
			Name:    "dup-prod",
			Path:    "dup",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"prod.tfvars",
				},
				Workspace: "prod",
			},
			Type: "terragrunt",
		},
	})
}

func Test_Generate_Grandchild(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	barConfig := bar.AddDirectory("config")
	barEnvs := barConfig.AddDirectory("envs")
	barEnvs.AddTFVarsFile("prod.tfvars")
	barEnvs.AddTFVarsFile("dev.tfvars")
	barEnvs.AddTFVarsFile("default.tfvars")

	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	fooConfig := foo.AddDirectory("config")
	fooEnvs := fooConfig.AddDirectory("envs")
	fooEnvs.AddTFVarsFile("staging.tfvars")
	fooEnvs.AddTFVarsFile("dev.tfvars")
	fooEnvs.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "apps-bar-dev",
			Path:    "apps/bar",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"config/envs/default.tfvars",
					"config/envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-prod",
			Path:    "apps/bar",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"config/envs/default.tfvars",
					"config/envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-dev",
			Path:    "apps/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"config/envs/default.tfvars",
					"config/envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-staging",
			Path:    "apps/foo",
			EnvName: "staging",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"config/envs/default.tfvars",
					"config/envs/staging.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_Grandparent(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	foo := apps.AddDirectory("foo")
	fooUs1 := foo.AddDirectory("us1")
	fooUs1.AddTerraformFileWithProviderBlock("main.tf")
	fooUs2 := foo.AddDirectory("us2")
	fooUs2.AddTerraformFileWithProviderBlock("main.tf")

	bar := apps.AddDirectory("bar")
	barUs1 := bar.AddDirectory("us1")
	barUs1.AddTerraformFileWithProviderBlock("main.tf")
	barUs2 := bar.AddDirectory("us2")
	barUs2.AddTerraformFileWithProviderBlock("main.tf")

	apps.AddTFVarsFile("dev.tfvars")
	apps.AddTFVarsFile("prod.tfvars")
	apps.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "apps-bar-us1-dev",
			Path:    "apps/bar/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-us1-prod",
			Path:    "apps/bar/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-us2-dev",
			Path:    "apps/bar/us2",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-us2-prod",
			Path:    "apps/bar/us2",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us1-dev",
			Path:    "apps/foo/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us1-prod",
			Path:    "apps/foo/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us2-dev",
			Path:    "apps/foo/us2",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us2-prod",
			Path:    "apps/foo/us2",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_GreatAunt(t *testing.T) {
	root := NewFilesystem(t)
	infra := root.AddDirectory("infra")

	apps := infra.AddDirectory("apps")
	bar := apps.AddDirectory("bar")
	barUs1 := bar.AddDirectory("us1")
	barUs1.AddTerraformFileWithProviderBlock("main.tf")
	barUs2 := bar.AddDirectory("us2")
	barUs2.AddTerraformFileWithProviderBlock("main.tf")

	foo := apps.AddDirectory("foo")
	fooUs1 := foo.AddDirectory("us1")
	fooUs1.AddTerraformFileWithProviderBlock("main.tf")
	fooUs2 := foo.AddDirectory("us2")
	fooUs2.AddTerraformFileWithProviderBlock("main.tf")

	envs := infra.AddDirectory("envs")
	envs.AddTFVarsFile("dev.tfvars")
	envs.AddTFVarsFile("prod.tfvars")
	envs.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "infra-apps-bar-us1-dev",
			Path:    "infra/apps/bar/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-bar-us1-prod",
			Path:    "infra/apps/bar/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-bar-us2-dev",
			Path:    "infra/apps/bar/us2",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-bar-us2-prod",
			Path:    "infra/apps/bar/us2",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-foo-us1-dev",
			Path:    "infra/apps/foo/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-foo-us1-prod",
			Path:    "infra/apps/foo/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-foo-us2-dev",
			Path:    "infra/apps/foo/us2",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "infra-apps-foo-us2-prod",
			Path:    "infra/apps/foo/us2",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../../envs/default.tfvars",
					"../../../envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_IncludeDirs(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	baz := apps.AddDirectory("baz")
	baz.AddTerraformFileWithProviderBlock("foo.tf")
	bat := apps.AddDirectory("bat")
	bat.AddTerraformFileWithProviderBlock("bip.tf")
	hidden := apps.AddDirectory(".hidden")
	hidden.AddTerraformFileWithProviderBlock("main.tf")

	wildcard := apps.AddDirectory("wildcard")
	one := wildcard.AddDirectory("one")
	one.AddTerraformFileWithProviderBlock("fip.tf")
	two := wildcard.AddDirectory("two")
	two.AddTerraformFileWithProviderBlock("fip.tf")

	apps.AddTFVarsFile("dev.tfvars")
	apps.AddTFVarsFile("prod.tfvars")
	apps.AddTFVarsFile("default.tfvars")

	template := `version: 0.1
autodetect:
  include_dirs:
    - apps/bat
    - apps/baz
    - apps/.hidden
    - apps/wildcard/**

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "apps-.hidden-dev",
			Path: "apps/.hidden",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-.hidden-prod",
			Path: "apps/.hidden",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-bar-dev",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-bar-prod",
			Path: "apps/bar",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-bat-dev",
			Path: "apps/bat",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-bat-prod",
			Path: "apps/bat",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-baz-dev",
			Path: "apps/baz",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-baz-prod",
			Path: "apps/baz",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-dev",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-foo-prod",
			Path: "apps/foo",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-wildcard-one-dev",
			Path: "apps/wildcard/one",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-wildcard-one-prod",
			Path: "apps/wildcard/one",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
				},
			},
		},
		{
			Name: "apps-wildcard-two-dev",
			Path: "apps/wildcard/two",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../dev.tfvars",
				},
			},
		},
		{
			Name: "apps-wildcard-two-prod",
			Path: "apps/wildcard/two",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../prod.tfvars",
				},
			},
		},
	})
}

func Test_Generate_IncludesHiddenDirs(t *testing.T) {
	root := NewFilesystem(t)

	infracost := root.AddDirectory(".infracost")
	foo := infracost.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	git := root.AddDirectory(".git")
	git.AddFile("main.ft")

	infra := root.AddDirectory(".infra")
	baz := infra.AddDirectory("baz")
	baz.AddTerraformFileWithProviderBlock("main.tf")
	bat := infra.AddDirectory("bat")
	bat.AddTerraformFileWithProviderBlock("main.tf")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: ".infra-bat",
			Path: ".infra/bat",
			Type: "terraform",
		},
		{
			Name: ".infra-baz",
			Path: ".infra/baz",
			Type: "terraform",
		},
	})
}

func Test_Generate_MaxSearchDepth(t *testing.T) {
	root := NewFilesystem(t)
	infra := root.AddDirectory("infra")
	foo := infra.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	nested := infra.AddDirectory("nested")
	bar := nested.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")

	template := `version: 0.1
autodetect:
  max_search_depth: 3

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "infra-foo",
			Path: "infra/foo",
		},
	})
}

func Test_Generate_ModuleCalls(t *testing.T) {
	root := NewFilesystem(t)
	infra := root.AddDirectory("infra")

	modules := infra.AddDirectory("modules")
	isCalled := modules.AddDirectory("is_called")
	isCalled.AddTerraformFileWithProviderBlock("main.tf")
	isAlsoCalled := modules.AddDirectory("is_also_called")
	isAlsoCalled.AddTerraformFileWithProviderBlock("main.tf")
	isAProject := modules.AddDirectory("is_a_project")
	isAProject.AddTerraformFileWithProviderBlock("main.tf")

	dev := infra.AddDirectory("dev")
	dev.AddTerraformWithModuleCallToSource("../modules/is_called")
	dev.AddTerraformWithModuleCallToSource("../modules/is_also_called")

	prod := infra.AddDirectory("prod")
	prod.AddTerraformWithModuleCallToSource("../modules/is_called")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "infra-dev",
			Path: "infra/dev",
			DependencyPaths: []string{
				"infra/modules/is_also_called/**",
				"infra/modules/is_called/**",
			},
			Type: "terraform",
		},
		{
			Name: "infra-modules-is_a_project",
			Path: "infra/modules/is_a_project",
			Type: "terraform",
		},
		{
			Name: "infra-prod",
			Path: "infra/prod",
			DependencyPaths: []string{
				"infra/modules/is_called/**",
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_ModuleCallsWithTemplate(t *testing.T) {
	root := NewFilesystem(t)
	infra := root.AddDirectory("infra")

	modules := infra.AddDirectory("modules")
	isCalled := modules.AddDirectory("is_called")
	isCalled.AddTerraformFileWithProviderBlock("main.tf")
	isAlsoCalled := modules.AddDirectory("is_also_called")
	isAlsoCalled.AddTerraformFileWithProviderBlock("main.tf")
	isAProject := modules.AddDirectory("is_a_project")
	isAProject.AddTerraformFileWithProviderBlock("main.tf")

	dev := infra.AddDirectory("dev")
	dev.AddTerraformWithModuleCallToSource("../modules/is_called")
	dev.AddTerraformWithModuleCallToSource("../modules/is_also_called")

	prod := infra.AddDirectory("prod")
	prod.AddTerraformWithModuleCallToSource("../modules/is_called")

	template := `version: 0.1

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
    dependency_paths:
    {{- range $dep := $project.DependencyPaths }}
      - {{ $dep }}
    {{- end }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "infra-dev",
			Path: "infra/dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{},
			},
			DependencyPaths: []string{
				"infra/modules/is_also_called/**",
				"infra/modules/is_called/**",
			},
		},
		{
			Name: "infra-modules-is_a_project",
			Path: "infra/modules/is_a_project",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{},
			},
			DependencyPaths: []string{},
		},
		{
			Name: "infra-prod",
			Path: "infra/prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{},
			},
			DependencyPaths: []string{
				"infra/modules/is_called/**",
			},
		},
	})
}

func Test_Generate_ModulesAndExternalTfvars(t *testing.T) {
	root := NewFilesystem(t)

	db := root.AddDirectory("db")
	modules := db.AddDirectory("modules")
	dbConfig := modules.AddDirectory("db_config")
	dbConfig.AddTerraformFileWithProviderBlock("main.tf")

	dev := db.AddDirectory("dev")
	dev.AddTerraformWithModuleCallToSource("../modules/db_config")

	prod := db.AddDirectory("prod")
	prod.AddTerraformWithModuleCallToSource("../modules/db_config")

	envs := root.AddDirectory("envs")
	envs.AddTFVarsFile("dev.tfvars")
	envs.AddTFVarsFile("prod.tfvars")

	root.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "db-dev",
			Path:    "db/dev",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../envs/dev.tfvars",
				},
			},
			DependencyPaths: []string{
				"db/modules/db_config/**",
			},
			Type: "terraform",
		},
		{
			Name:    "db-prod",
			Path:    "db/prod",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../default.tfvars",
					"../../envs/prod.tfvars",
				},
			},
			DependencyPaths: []string{
				"db/modules/db_config/**",
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_MultiDescendents(t *testing.T) {

	/*
		If a var file is a descendent of multiple root modules, it should only
		be linked with the nearest one in the hierarchy
		.
		└── apps
		    ├── foo
		    │   ├── main.tf
		    │   ├── envs
		    │   │   ├── dev.tfvars
		    │   │   └── staging.tfvars
		    │   └── us1
		    │       ├── main.tf
		    │       └── envs
		    │           ├── dev.tfvars
		    │           └── prod.tfvars
		    ├── bar
		    │   ├── main.tf
		    │   ├── envs
		    │   │   ├── dev.tfvars
		    │   │   └── staging.tfvars
		    │   └── us1
		    │       ├── main.tf
		    │       └── envs
		    │           ├── dev.tfvars
		    │           └── prod.tfvars
		    └── envs
		        └── default.tfvars
	*/

	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")
	fooEnvs := foo.AddDirectory("envs")
	fooEnvs.AddTFVarsFile("dev.tfvars")
	fooEnvs.AddTFVarsFile("staging.tfvars")
	fooUs1 := foo.AddDirectory("us1")
	fooUs1.AddTerraformFileWithProviderBlock("main.tf")
	fooUs1Envs := fooUs1.AddDirectory("envs")
	fooUs1Envs.AddTFVarsFile("dev.tfvars")
	fooUs1Envs.AddTFVarsFile("prod.tfvars")

	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	barEnvs := bar.AddDirectory("envs")
	barEnvs.AddTFVarsFile("dev.tfvars")
	barEnvs.AddTFVarsFile("staging.tfvars")
	barUs1 := bar.AddDirectory("us1")
	barUs1.AddTerraformFileWithProviderBlock("main.tf")
	barUs1Envs := barUs1.AddDirectory("envs")
	barUs1Envs.AddTFVarsFile("dev.tfvars")
	barUs1Envs.AddTFVarsFile("prod.tfvars")

	appsEnvs := apps.AddDirectory("envs")
	appsEnvs.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "apps-bar-dev",
			Path:    "apps/bar",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/default.tfvars",
					"envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-staging",
			Path:    "apps/bar",
			EnvName: "staging",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/default.tfvars",
					"envs/staging.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-us1-dev",
			Path:    "apps/bar/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-us1-prod",
			Path:    "apps/bar/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-dev",
			Path:    "apps/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/default.tfvars",
					"envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-staging",
			Path:    "apps/foo",
			EnvName: "staging",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../envs/default.tfvars",
					"envs/staging.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us1-dev",
			Path:    "apps/foo/us1",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"envs/dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-us1-prod",
			Path:    "apps/foo/us1",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../../envs/default.tfvars",
					"envs/prod.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_Parent(t *testing.T) {
	root := NewFilesystem(t)
	apps := root.AddDirectory("apps")

	bar := apps.AddDirectory("bar")
	bar.AddTerraformFileWithProviderBlock("main.tf")
	foo := apps.AddDirectory("foo")
	foo.AddTerraformFileWithProviderBlock("main.tf")

	apps.AddTFVarsFile("dev.tfvars")
	apps.AddTFVarsFile("prod.tfvars")
	apps.AddTFVarsFile("default.tfvars")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name:    "apps-bar-dev",
			Path:    "apps/bar",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-bar-prod",
			Path:    "apps/bar",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-dev",
			Path:    "apps/foo",
			EnvName: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../dev.tfvars",
				},
			},
			Type: "terraform",
		},
		{
			Name:    "apps-foo-prod",
			Path:    "apps/foo",
			EnvName: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"../default.tfvars",
					"../prod.tfvars",
				},
			},
			Type: "terraform",
		},
	})
}

func Test_Generate_SkipEnvInTemplate(t *testing.T) {

	root := NewFilesystem(t)
	env := root.AddDirectory("environment")
	env.AddDirectory("legacy").AddTFVarsFile("terraform.tfvars")
	env.AddDirectory("dev").AddTFVarsFile("terraform.tfvars")
	env.AddDirectory("prod").AddTFVarsFile("terraform.tfvars")
	root.AddTerraformWithNoBackendOrProvider("main.tf")

	template := `version: 0.1

projects:
{{- range $project := matchPaths "environment/:env/terraform.tfvars" }}
  {{- if ne $project.env "legacy"}}
    - path: .
      name: {{ $project.env }}
      terraform_var_files:
        - environment/{{ $project.env }}/terraform.tfvars
  {{- end}}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Name: "dev",
			Path: ".",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"environment/dev/terraform.tfvars",
				},
			},
		},
		{
			Name: "prod",
			Path: ".",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"environment/prod/terraform.tfvars",
				},
			},
		},
	})

}

func Test_Generate_AutodetectWithNestedTfvars(t *testing.T) {

	/*
		/
			app1
				app3
					qa.tfvars
					main.tf
				env
					prod.tfvars
					test.tfvars
				defaults.tfvars
				main.tf
			app2
				env
					defaults.tfvars
					prod.tfvars
					staging.tfvars
				main.tf

	*/

	root := NewFilesystem(t)
	app1 := root.AddDirectory("app1")

	app3 := app1.AddDirectory("app3")
	app3.AddTerraformFileWithProviderBlock("main.tf")
	app3.AddTFVarsFile("qa.tfvars")

	app1env := app1.AddDirectory("env")
	app1env.AddTFVarsFile("prod.tfvars")
	app1env.AddTFVarsFile("test.tfvars")

	app1.AddTFVarsFile("defaults.tfvars")
	app1.AddTerraformFileWithProviderBlock("main.tf")

	app2 := root.AddDirectory("app2")
	app2env := app2.AddDirectory("env")
	app2env.AddTFVarsFile("defaults.tfvars")
	app2env.AddTFVarsFile("prod.tfvars")
	app2env.AddTFVarsFile("staging.tfvars")
	app2.AddTerraformFileWithProviderBlock("main.tf")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "app1-prod",
			Path: "app1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"defaults.tfvars",
					"env/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "app1-test",
			Path: "app1",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"defaults.tfvars",
					"env/test.tfvars",
				},
				Workspace: "test",
			},
			EnvName: "test",
			Type:    "terraform",
		},
		{
			Name: "app1-app3-qa",
			Path: "app1/app3",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"qa.tfvars",
				},
				Workspace: "qa",
			},
			EnvName: "qa",
			Type:    "terraform",
		},
		{
			Name: "app2-prod",
			Path: "app2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/defaults.tfvars",
					"env/prod.tfvars",
				},
				Workspace: "prod",
			},
			EnvName: "prod",
			Type:    "terraform",
		},
		{
			Name: "app2-staging",
			Path: "app2",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"env/defaults.tfvars",
					"env/staging.tfvars",
				},
				Workspace: "staging",
			},
			EnvName: "staging",
			Type:    "terraform",
		},
	})
}

func Test_Generate_TemplatedDetectedProjects(t *testing.T) {

	root := NewFilesystem(t)
	dev := root.AddDirectory("dev")
	dev.AddTerraformFileWithProviderBlock("main.tf")
	dev.AddTFVarsFile("terraform.tfvars")

	prod := root.AddDirectory("prod")
	prod.AddTerraformFileWithProviderBlock("main.tf")
	prod.AddTFVarsFile("terraform.tfvars")

	template := `version: 0.1

projects:
{{- range $project := .DetectedProjects }}
  - path: {{ $project.Path }}
    name: {{ $project.Name }}
    terraform_var_files:
    {{- range $varFile := $project.TerraformVarFiles }}
      - {{ $varFile }}
    {{- end }}
{{- end }}
`

	testConfigGenerationWithTemplate(t, root.Path(), template, []*config.Project{
		{
			Path: "dev",
			Name: "dev",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"terraform.tfvars",
				},
			},
		},
		{
			Path: "prod",
			Name: "prod",
			Terraform: config.ProjectTerraform{
				VarFiles: []string{
					"terraform.tfvars",
				},
			},
		},
	})

}

func Test_Generate_CFNYAML(t *testing.T) {

	root := NewFilesystem(t)
	root.AddCFNYAML()

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "template",
			Path: "template.yml",
			Type: "cloudformation",
		},
	})
}

func Test_Generate_CFNJSON(t *testing.T) {

	root := NewFilesystem(t)
	root.AddCFNJSON()

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "template",
			Path: "template.json",
			Type: "cloudformation",
		},
	})
}

func Test_Generate_CFNYAML_InsideTFProject(t *testing.T) {

	root := NewFilesystem(t)
	tfDir := root.AddDirectory("tf")
	tfDir.AddCFNYAML()
	tfDir.AddTerraformFileWithProviderBlock("main.tf")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "tf",
			Path: "tf",
			Type: "terraform",
		},
	},
	)
}

func Test_Generate_CDK_DetectsAppsButDoesNotSynth(t *testing.T) {
	root := NewFilesystem(t)
	cdkDir := root.AddDirectory("cdk-app")

	cdkDir.AddFile("cdk.json").Content(`{"app": "node bin/app.js"}`)
	cdkDir.AddFile("package-lock.json").Content(`{}`)
	cdkDir.AddFile("package.json").Content(`{}`)

	synthCalled := false

	generated, err := config.Generate(root.Path())

	require.NoError(t, err)
	require.NotNil(t, generated)

	require.Len(t, generated.CDK.Projects, 1)
	require.Equal(t, "cdk-app/cdk.json", generated.CDK.Projects[0].CdkConfigPath)

	require.False(t, synthCalled, "Synth should not be called during Generate()")

	require.Len(t, generated.Projects, 0)
}

func Test_TerragruntDependenciesTracked(t *testing.T) {
	root := NewFilesystem(t)
	tf := root.AddDirectory("tf")
	tf.AddTerraformFileWithProviderBlock("main.tf")

	tg := root.AddDirectory("tg")
	tg.AddFile("terragrunt.hcl").Content(`
terraform {
  source = "../tf"
}
`)

	generated, err := config.Generate(root.Path())

	require.NoError(t, err)
	require.Len(t, generated.Projects, 1, "should only have the Terragrunt project")
	assert.Equal(t, config.ProjectTypeTerragrunt, generated.Projects[0].Type, "the detected project should be a terragrunt one")
	assert.Equal(t, "tg", generated.Projects[0].Path, "the detected project should be at the terragrunt directory (tg)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tf/**", "the detected project should have a dependency on the terraform directory (tf)")
}

func Test_TerragruntNestedDependenciesTracked(t *testing.T) {
	root := NewFilesystem(t)
	tf := root.AddDirectory("tf")
	tf.AddTerraformFileWithProviderBlock("main.tf")

	tg := root.AddDirectory("tg")
	tg.AddFile("terragrunt.hcl").Content(`
terraform {
  source = "../tf"
}
`)
	nested := tg.AddDirectory("nested")
	nested.AddFile("terragrunt.hcl").Content(`
include {
  path = find_in_parent_folders()
}
`)

	generated, err := config.Generate(root.Path())

	require.NoError(t, err)
	require.Len(t, generated.Projects, 1, "should only have the Terragrunt project")
	assert.Equal(t, config.ProjectTypeTerragrunt, generated.Projects[0].Type, "the detected project should be a terragrunt one")
	assert.Equal(t, "tg/nested", generated.Projects[0].Path, "the detected project should be at the nested terragrunt directory (tg/nested)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tf/**", "the detected project should have a dependency on the terraform directory (tf)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tg/terragrunt.hcl", "the detected project should have a dependency on the terragrunt directory (tg)")
}

func Test_TerragruntNestedDependenciesTracked_NonDefaultName(t *testing.T) {
	root := NewFilesystem(t)
	tf := root.AddDirectory("tf")
	tf.AddTerraformFileWithProviderBlock("main.tf")

	tg := root.AddDirectory("tg")
	tg.AddFile("whatever.hcl").Content(`
terraform {
  source = "../tf"
}
`)
	nested := tg.AddDirectory("nested")
	nested.AddFile("terragrunt.hcl").Content(`
include {
  path = find_in_parent_folders("whatever.hcl")
}
`)

	generated, err := config.Generate(root.Path())

	require.NoError(t, err)
	require.Len(t, generated.Projects, 1, "should only have the Terragrunt project")
	assert.Equal(t, config.ProjectTypeTerragrunt, generated.Projects[0].Type, "the detected project should be a terragrunt one")
	assert.Equal(t, "tg/nested", generated.Projects[0].Path, "the detected project should be at the nested terragrunt directory (tg/nested)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tf/**", "the detected project should have a dependency on the terraform directory (tf)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tg/whatever.hcl", "the detected project should have a dependency on the terragrunt directory (tg)")
}

func Test_TerragruntNestedDependenciesTracked_Template(t *testing.T) {
	root := NewFilesystem(t)
	tf := root.AddDirectory("tf")
	tf.AddTerraformFileWithProviderBlock("main.tf")

	tg := root.AddDirectory("tg")
	tg.AddFile("terragrunt.hcl").Content(`
terraform {
  source = "../tf"
}
`)
	nested := tg.AddDirectory("nested")
	nested.AddFile("terragrunt.hcl").Content(`
include {
  path = "${find_in_parent_folders()}"
}
`)

	generated, err := config.Generate(root.Path())

	require.NoError(t, err)
	require.Len(t, generated.Projects, 1, "should only have the Terragrunt project")
	assert.Equal(t, config.ProjectTypeTerragrunt, generated.Projects[0].Type, "the detected project should be a terragrunt one")
	assert.Equal(t, "tg/nested", generated.Projects[0].Path, "the detected project should be at the nested terragrunt directory (tg/nested)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tf/**", "the detected project should have a dependency on the terraform directory (tf)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tg/terragrunt.hcl", "the detected project should have a dependency on the terragrunt directory (tg)")
}

func Test_TerragruntNestedDependenciesTracked_HarcodedPath(t *testing.T) {
	root := NewFilesystem(t)
	tf := root.AddDirectory("tf")
	tf.AddTerraformFileWithProviderBlock("main.tf")

	tg := root.AddDirectory("tg")
	tg.AddFile("terragrunt.hcl").Content(`
terraform {
  source = "../tf"
}
`)
	nested := tg.AddDirectory("nested")
	nested.AddFile("terragrunt.hcl").Content(`
include {
  path = "../terragrunt.hcl"
}
`)

	generated, err := config.Generate(root.Path())

	require.NoError(t, err)
	require.Len(t, generated.Projects, 1, "should only have the Terragrunt project")
	assert.Equal(t, config.ProjectTypeTerragrunt, generated.Projects[0].Type, "the detected project should be a terragrunt one")
	assert.Equal(t, "tg/nested", generated.Projects[0].Path, "the detected project should be at the nested terragrunt directory (tg/nested)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tf/**", "the detected project should have a dependency on the terraform directory (tf)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tg/terragrunt.hcl", "the detected project should have a dependency on the terragrunt directory (tg)")
}

func Test_TerragruntNestedDependenciesTracked_TemplateWithInterpolation(t *testing.T) {
	root := NewFilesystem(t)
	tf := root.AddDirectory("tf")
	tf.AddTerraformFileWithProviderBlock("main.tf")

	tg := root.AddDirectory("tg")
	tg.AddFile("something.hcl").Content(`
terraform {
  source = "../tf"
}
`)
	nested := tg.AddDirectory("nested")
	nested.AddFile("terragrunt.hcl").Content(`
include {
  path = "${get_path_to_repo_root()}/tg/something.hcl"
}
`)

	generated, err := config.Generate(root.Path())

	require.NoError(t, err)
	require.Len(t, generated.Projects, 1, "should only have the Terragrunt project")
	assert.Equal(t, config.ProjectTypeTerragrunt, generated.Projects[0].Type, "the detected project should be a terragrunt one")
	assert.Equal(t, "tg/nested", generated.Projects[0].Path, "the detected project should be at the nested terragrunt directory (tg/nested)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tf/**", "the detected project should have a dependency on the terraform directory (tf)")
	assert.Contains(t, generated.Projects[0].DependencyPaths, "tg/something.hcl", "the detected project should have a dependency on the terragrunt directory (tg)")
}

func Test_Generate_CiscoStacks(t *testing.T) {
	root := NewFilesystem(t)

	// Create a cisco stacks structure: stacks/<stack>/base + stacks/<stack>/layers/<layer>
	stacksDir := root.AddDirectory("stacks")
	myStack := stacksDir.AddDirectory("my-stack")
	base := myStack.AddDirectory("base")
	base.AddTerraformFileWithProviderBlock("main.tf")
	layers := myStack.AddDirectory("layers")
	layers.AddDirectory("dev")
	layers.AddDirectory("prod")

	testConfigGeneration(t, root.Path(), []*config.Project{
		{
			Name: "stacks/my-stack/layer/dev",
			Path: ".",
			Type: config.ProjectTypeCiscoStacks,
		},
		{
			Name: "stacks/my-stack/layer/prod",
			Path: ".",
			Type: config.ProjectTypeCiscoStacks,
		},
	})
}

func Test_TerraformDuplicateDependenciesRemoved(t *testing.T) {
	root := NewFilesystem(t)
	tf := root.AddDirectory("tf")
	tf.AddTerraformFileWithProviderBlock("main.tf")

	project1 := root.AddDirectory("project1")

	project1.AddTerraformWithModuleCallToSource("../tf")
	project1.AddTerraformWithModuleCallToSource("../tf")

	generated, err := config.Generate(root.Path())

	require.NoError(t, err)
	require.Len(t, generated.Projects, 1, "should only have one project")
	assert.Equal(t, config.ProjectTypeTerraform, generated.Projects[0].Type, "the detected project should be a terraform one")
	assert.Equal(t, 1, len(generated.Projects[0].DependencyPaths), "the detected project should have one dependency")
	assert.Equal(t, "tf/**", generated.Projects[0].DependencyPaths[0], "the detected project should have a dependency on the terraform directory (tf)")
}
