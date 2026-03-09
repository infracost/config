package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/infracost/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func testConfigGeneration(t *testing.T, dir string, wantProjects []*config.Project, opts ...config.GenerationOption) {
	testConfigGenerationWithTemplate(t, dir, "", wantProjects, opts...)
}

func testConfigGenerationWithTemplate(t *testing.T, dir, template string, wantProjects []*config.Project, opts ...config.GenerationOption) {
	if template != "" {
		opts = append(opts, config.WithTemplate(template))
	}
	generated, err := config.Generate(dir, opts...)
	require.NoError(t, err, "expected nil error, got %v", err)
	assertProjectsMatch(t, wantProjects, generated.Projects, "generated config does not match expected")
}

func NewFilesystem(t *testing.T) *Directory {
	dir := filepath.Join(t.TempDir(), uuid.NewString())
	err := os.MkdirAll(dir, 0700)
	if err != nil {
		t.Fatalf("failed to create directory %s: %v", dir, err)
	}
	return &Directory{
		t:    t,
		name: dir,
	}
}

type Directory struct {
	t      *testing.T
	parent *Directory
	name   string
}

type File struct {
	t      *testing.T
	name   string
	parent *Directory
}

func (d *Directory) AddTerragruntFile() {
	d.AddFile("terragrunt.hcl").Content(`input = {
	}`)
}

func (d *Directory) AddCFNYAML() {
	d.AddFile("template.yml").Content(`Resources:
  MyResource:
    Type: AWS::EC2::Instance
    Properties:
      InstanceType: t2.micro
`)
}

func (d *Directory) AddCFNJSON() {
	d.AddFile("template.json").Content(`{
	"Resources": {
		"MyResource": {
			"Type": "AWS::EC2::Instance",
			"Properties": {
				"InstanceType": "t2.micro"
			}
		}
	}
}`)
}

func (d *Directory) AddFile(name string) *File {
	f := &File{
		t:      d.t,
		parent: d,
		name:   name,
	}
	if err := os.WriteFile(f.Path(), nil, 0600); err != nil {
		d.t.Fatalf("failed to create file %s: %v", f.Path(), err)
	}
	return f
}

func (d *Directory) AddTerraformFileWithProviderBlock(name string) {
	d.AddFile(name).Content(`provider "aws" {
  region                      = "us-east-1"
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  access_key                  = "mock_access_key"
  secret_key                  = "mock_secret_key"
}`)
}

func (d *Directory) AddTerraformWithNoBackendOrProvider(name string) {
	d.AddFile(name).Content(`
variable "region" {
	type = string
}`)
}

func (d *Directory) AddTFVarsJSONFile(name string) {
	d.AddFile(name).Content(`{
	"region": "us-east-1"
}`)
}

func (d *Directory) AddTFVarsFile(name string) {
	d.AddFile(name).Content(`instance_type = "m5.4xlarge"
`)
}

func (d *Directory) AddTerraformWithModuleCallToSource(source string) {
	name := uuid.NewString()
	d.AddFile(name + ".tf").Content(fmt.Sprintf(`
provider "aws" {
  region                      = "us-east-1"
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  access_key                  = "mock_access_key"
  secret_key                  = "mock_secret_key"
}

module "%s" {
	source = %q
}
`, name, source))
}

func (d *Directory) AddTerragruntFileIncludingParentDir() {
	d.AddFile("terragrunt.hcl").Content(`include {
	path = find_in_parent_folders()
}`)
}

func (d *Directory) AddDirectory(name string) *Directory {
	subdir := &Directory{
		t:      d.t,
		parent: d,
		name:   name,
	}
	err := os.MkdirAll(subdir.Path(), 0700)
	if err != nil {
		d.t.Fatalf("failed to create directory %s: %v", subdir.Path(), err)
	}
	return subdir
}

func (d *Directory) Path() string {
	if d.parent == nil {
		return d.name
	}
	return filepath.Join(d.parent.Path(), d.name)
}

func (f *File) Path() string {
	return filepath.Join(f.parent.Path(), f.name)
}

func (f *File) Content(content string) *File {
	if err := os.WriteFile(f.Path(), []byte(content), 0600); err != nil {
		f.t.Fatalf("failed to write file content %s: %v", f.Path(), err)
	}
	return f
}

func assertProjectsMatch(t *testing.T, want, got []*config.Project, prefix string) {
	t.Helper()
	var wantedProjectNames []string
	for _, project := range want {
		wantedProjectNames = append(wantedProjectNames, project.Name)
	}
	ok := true
	var gotProjectNames []string
	dupeCheck := map[string]struct{}{}
	for _, project := range got {
		if !slices.Contains(wantedProjectNames, project.Name) {
			t.Logf("%s: unexpected project %q", prefix, project.Name)
			ok = false
		}
		if _, dupe := dupeCheck[project.Name]; dupe {
			t.Logf("%s: unexpected duplicate project %q", prefix, project.Name)
			ok = false
		}
		dupeCheck[project.Name] = struct{}{}
		gotProjectNames = append(gotProjectNames, project.Name)
	}
	assert.Equal(t, len(want), len(got), "%s: expected %d projects, got %d", prefix, len(want), len(got))
	for _, wantedName := range wantedProjectNames {
		if !slices.Contains(gotProjectNames, wantedName) {
			t.Logf("%s: missing expected project %q", prefix, wantedName)
			ok = false
		}
	}
	if !ok {
		t.Fail()
	}
	for _, wantedProject := range want {
		for _, gotProject := range got {
			if wantedProject.Name == gotProject.Name {
				assertProjectMatches(t, wantedProject, gotProject, fmt.Sprintf("%s: project %q", prefix, wantedProject.Name))
			}
		}
	}
	if t.Failed() {
		wantYml, err := yaml.Marshal(want)
		if err != nil {
			t.Logf("%s: failed to marshal want projects: %v", prefix, err)
		} else {
			t.Logf("%s: want projects:\n%s", prefix, string(wantYml))
		}
		yml, err := yaml.Marshal(got)
		if err != nil {
			t.Logf("%s: failed to marshal got projects: %v", prefix, err)
		} else {
			t.Logf("%s: got projects:\n%s", prefix, string(yml))
		}
	}
}

func assertProjectMatches(t *testing.T, want, got *config.Project, prefix string) {
	t.Helper()
	assert.Equal(t, want.Name, got.Name, "%s: expected project name %q, got %q", prefix, want.Name, got.Name)
	assert.Equal(t, want.Path, got.Path, "%s: expected project path %q, got %q", prefix, want.Path, got.Path)
	assert.Equal(t, want.EnvName, got.EnvName, "%s: expected project env name %q, got %q", prefix, want.EnvName, got.EnvName)
	if len(want.Terraform.VarFiles) > 0 || len(got.Terraform.VarFiles) > 0 {
		assert.EqualValues(t, want.Terraform.VarFiles, got.Terraform.VarFiles, "%s: expected project terraform var files %q, got %q", prefix, want.Terraform.VarFiles, got.Terraform.VarFiles)
	}
	if len(want.DependencyPaths) > 0 || len(got.DependencyPaths) > 0 {
		assert.EqualValues(t, want.DependencyPaths, got.DependencyPaths, "%s: expected project dependency paths %q, got %q", prefix, want.DependencyPaths, got.DependencyPaths)
	}
	assert.Equal(t, want.Type, got.Type, "%s: expected project type %q, got %q", prefix, want.Type, got.Type)
}
