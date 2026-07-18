package config

import (
	"testing"

	"github.com/infracost/config/configfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func headingNode(t *testing.T, yamlBody string) yaml.Node {
	t.Helper()
	var n yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(yamlBody), &n))
	// unwrap the document node to the mapping node
	return *n.Content[0]
}

// fromConfigFile maps the known headings (terraform, aws, cdk) into their typed API fields and routes
// any other heading into the opaque Plugins blob.
func TestFromConfigFile_MapsHeadingsToTypedFields(t *testing.T) {
	fc := &configfile.Config{
		Version:   "0.3",
		Currency:  "USD",
		UsageFile: "usage.yml",
		Headings: map[string]yaml.Node{
			"terraform": headingNode(t, `
source_map:
  - match: "^MOD$"
    replace: "github.com/x//mod"
defaults:
  workspace: prod
`),
			"cdk": headingNode(t, `
defaults:
  context:
    key: val
`),
			// repo-level plugin defaults live under a defaults: sub-key (consistent with terraform.defaults)
			"kubernetes": headingNode(t, `
defaults:
  some_default: true
`),
		},
		Projects: []configfile.Project{
			{
				Name: "main",
				Type: "terragrunt",
				Path: "app",
				Headings: map[string]yaml.Node{
					"terraform": headingNode(t, `
workspace: prod
cloud:
  host: app.terraform.io
  org: acme
var_files:
  - prod.tfvars
vars:
  region: us-east-1
`),
				},
			},
			{
				Name: "stack",
				Type: "cloudformation",
				Path: "stack",
				Headings: map[string]yaml.Node{
					"aws": headingNode(t, `
region: us-east-1
stack_name: my-stack
`),
					"kubernetes": headingNode(t, `
opt: true
`),
				},
			},
		},
	}

	var target Config
	require.NoError(t, fromConfigFile(fc, &target))

	// structural
	assert.Equal(t, "0.3", target.Version)
	assert.Equal(t, "USD", target.Currency)
	assert.Equal(t, "usage.yml", target.UsageFilePath)

	// repo-level terraform heading -> typed Terraform
	require.Len(t, target.Terraform.SourceMap, 1)
	assert.Equal(t, "^MOD$", target.Terraform.SourceMap[0].Match)
	assert.Equal(t, "prod", target.Terraform.Defaults.Workspace)

	// repo-level cdk heading -> typed CDK (not a plugin blob)
	assert.Equal(t, "val", target.CDK.Defaults.Context["key"])
	assert.NotContains(t, target.Plugins, "cdk")

	// repo-level unknown heading -> repo Plugins blob
	require.Contains(t, target.Plugins, "kubernetes")
	assert.Equal(t, true, target.Plugins["kubernetes"]["some_default"])

	// project terraform heading splits: workspace/cloud stay typed; var_files/vars go to the blob
	// keyed by the exact project type (terragrunt).
	require.Len(t, target.Projects, 2)
	tg := target.Projects[0]
	assert.Equal(t, "main", tg.Name)
	assert.Equal(t, ProjectTypeTerragrunt, tg.Type)
	assert.Equal(t, "prod", tg.Terraform.Workspace)
	assert.Equal(t, "app.terraform.io", tg.Terraform.Cloud.Host)
	assert.Equal(t, []any{"prod.tfvars"}, tg.Plugins["terragrunt"]["var_files"], "canonical source is the blob")
	assert.Equal(t, map[string]any{"region": "us-east-1"}, tg.Plugins["terragrunt"]["vars"])
	// the deprecated typed fields are populated as a backwards-compat mirror of the blob
	assert.Equal(t, []string{"prod.tfvars"}, tg.Terraform.VarFiles, "mirrored from the blob")
	assert.Equal(t, map[string]any{"region": "us-east-1"}, tg.Terraform.Vars, "mirrored from the blob")

	// project aws heading -> cloudformation blob (canonical); the deprecated typed AWS mirrors it.
	cfn := target.Projects[1]
	assert.Equal(t, "us-east-1", cfn.Plugins["cloudformation"]["region"])
	assert.Equal(t, "us-east-1", cfn.AWS.Region, "mirrored from the cloudformation blob")
	assert.Equal(t, "my-stack", cfn.AWS.StackName, "mirrored from the cloudformation blob")
	assert.Equal(t, "my-stack", cfn.Plugins["cloudformation"]["stack_name"])
	require.Contains(t, cfn.Plugins, "kubernetes")
	assert.Equal(t, true, cfn.Plugins["kubernetes"]["opt"])
}

// toConfigFile is the inverse of fromConfigFile: an API Config -> configfile -> API Config round-trip
// preserves the typed sections and plugin blobs.
func TestToConfigFile_RoundTripsThroughAPI(t *testing.T) {
	original := &Config{
		ConfigBase:    ConfigBase{Version: "0.3"},
		Currency:      "USD",
		UsageFilePath: "usage.yml",
		Terraform: Terraform{
			SourceMap: []TerraformRegexSource{{Match: "^MOD$", Replace: "github.com/x//mod"}},
			Defaults:  TerraformDefaults{Workspace: "prod"},
		},
		Plugins: map[string]map[string]any{
			"kubernetes": {"some_default": true},
		},
		Projects: []*Project{
			{
				Name: "main",
				Type: ProjectTypeTerragrunt,
				Path: "app",
				// post-split API shape: workspace/cloud typed, var_files/vars in the blob keyed by type
				Terraform: ProjectTerraform{
					Workspace: "prod",
					Cloud:     TerraformCloud{Host: "app.terraform.io", Org: "acme"},
				},
				Plugins: map[string]map[string]any{
					"terragrunt": {
						"var_files": []any{"prod.tfvars"},
						"vars":      map[string]any{"region": "us-east-1"},
					},
				},
			},
			{
				Name: "stack",
				Type: ProjectTypeCloudFormation,
				Path: "stack",
				Plugins: map[string]map[string]any{
					"cloudformation": {"region": "us-east-1", "stack_name": "my-stack"},
					"kubernetes":     {"opt": true},
				},
			},
		},
	}

	fc, err := toConfigFile(original)
	require.NoError(t, err)

	// the file layer must carry one heading per plugin - no `plugins:` block anywhere
	out, err := yaml.Marshal(fc)
	require.NoError(t, err)
	rendered := string(out)
	t.Logf("\n--- MARSHALLED ---\n%s", rendered)
	assert.NotContains(t, rendered, "plugins:")
	// the cloudformation blob renders under the historical aws: heading
	assert.Contains(t, rendered, "aws:")

	var back Config
	require.NoError(t, fromConfigFile(fc, &back))

	assert.Equal(t, original.Version, back.Version)
	assert.Equal(t, original.Currency, back.Currency)
	assert.Equal(t, original.UsageFilePath, back.UsageFilePath)
	assert.Equal(t, original.Terraform, back.Terraform)
	assert.Equal(t, original.Plugins, back.Plugins)
	require.Len(t, back.Projects, 2)
	// the canonical blob and the typed (non-mirrored) fields round-trip exactly; back also gains the
	// deprecated mirror (VarFiles/Vars/AWS), which is derived on load, so compare those explicitly
	// rather than the whole Terraform struct.
	assert.Equal(t, original.Projects[0].Plugins, back.Projects[0].Plugins)
	assert.Equal(t, original.Projects[0].Terraform.Workspace, back.Projects[0].Terraform.Workspace)
	assert.Equal(t, original.Projects[0].Terraform.Cloud, back.Projects[0].Terraform.Cloud)
	assert.Equal(t, original.Projects[0].Name, back.Projects[0].Name)
	assert.Equal(t, original.Projects[0].Type, back.Projects[0].Type)
	assert.Equal(t, original.Projects[1].Plugins, back.Projects[1].Plugins)
}

// A nil Projects slice (key omitted) leaves the target's existing projects untouched; a non-nil slice
// replaces them.
func TestFromConfigFile_ProjectsSeeding(t *testing.T) {
	seeded := []*Project{{Path: "."}}

	// omitted -> keep seeded
	target := Config{Projects: seeded}
	require.NoError(t, fromConfigFile(&configfile.Config{Version: "0.3"}, &target))
	assert.Equal(t, seeded, target.Projects, "omitted projects must keep the seeded defaults")

	// explicit empty -> replace with empty
	target = Config{Projects: seeded}
	require.NoError(t, fromConfigFile(&configfile.Config{Version: "0.3", Projects: []configfile.Project{}}, &target))
	assert.Empty(t, target.Projects, "explicit empty projects must clear the defaults")
}
