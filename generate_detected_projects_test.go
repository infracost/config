package config_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/infracost/config"
	"github.com/infracost/config/autodetect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func Test_GenerateDetectedProjects_TemplateTestdata(t *testing.T) {
	isProd := func(name string) bool { return !slices.Contains([]string{"test2", "foo1"}, name) }
	dirs := []string{
		"different_env_dirs",
		"different_env_files",
		"include_directory_based_on_file",
		"with_is_dir",
		"with_is_production",
		"with_list",
		"with_parse_functions",
		"with_rel_paths",
		"with_string_manipulation_functions",
		"with_string_matching_functions",
	}
	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			path, err := filepath.Abs(filepath.Join("template", "testdata", dir))
			require.NoError(t, err)
			tmpl, err := os.ReadFile(filepath.Join(path, "infracost.yml.tmpl"))
			require.NoError(t, err)
			golden, err := os.ReadFile(filepath.Join(path, "infracost.golden.yml"))
			require.NoError(t, err)

			var rendered bytes.Buffer
			generated, err := config.Generate(t.Context(), path,
				config.WithDetectedProjects(nil, nil),
				config.WithTemplate(string(tmpl)),
				config.WithIsProjectProductionFunc(isProd),
				config.WithWriter(&rendered),
				config.WithSkipCDK(true),
			)
			require.NoError(t, err)

			var want, got any
			require.NoError(t, yaml.Unmarshal(golden, &want))
			require.NoError(t, yaml.Unmarshal(rendered.Bytes(), &got))
			assert.Equal(t, want, got)

			var goldenCfg struct {
				Projects []any `yaml:"projects"`
			}
			require.NoError(t, yaml.Unmarshal(golden, &goldenCfg))
			assert.Len(t, generated.Projects, len(goldenCfg.Projects))
		})
	}
}

func sampleRepo(t *testing.T) string {
	t.Helper()
	root := NewFilesystem(t)
	app := root.AddDirectory("infra").AddDirectory("app")
	app.AddFile("main.tf")
	app.AddFile("dev.tfvars")
	return root.Path()
}

func sampleProjects() ([]autodetect.Project, []autodetect.RootModule) {
	p := autodetect.Project{
		Name:              "infra-app-dev",
		Path:              "infra/app",
		Env:               "dev",
		Type:              config.ProjectTypeTerraform,
		TerraformVarFiles: []string{"dev.tfvars"},
		DependencyPaths:   []string{"modules/**"},
	}
	return []autodetect.Project{p}, []autodetect.RootModule{{Path: "infra/app", Type: config.ProjectTypeTerraform, Projects: []autodetect.Project{p}}}
}

func generateSample(t *testing.T, tmpl string, rendered *bytes.Buffer) (*config.Config, error) {
	t.Helper()
	projects, rootModules := sampleProjects()
	opts := []config.GenerationOption{
		config.WithDetectedProjects(projects, rootModules),
		config.WithRepoName("example/repo"),
		config.WithBranch("main"),
		config.WithBaseBranch("main"),
		config.WithTemplate(tmpl),
	}
	if rendered != nil {
		opts = append(opts, config.WithWriter(rendered))
	}
	return config.Generate(t.Context(), sampleRepo(t), opts...)
}

func Test_GenerateDetectedProjects_DetectedProjectsLoop(t *testing.T) {
	tmpl := `version: 0.1
projects:
{{- range .DetectedProjects }}
  - name: {{ .Name }}-{{ $.Branch }}
    path: {{ .Path }}
    terraform_var_files:
    {{- range .TerraformVarFiles }}
      - {{ . }}
    {{- end }}
{{- end }}
{{- range .DetectedRootModules }}
  - name: root-{{ .Path }}
    path: {{ .Path }}
{{- end }}
`
	var rendered bytes.Buffer
	generated, err := generateSample(t, tmpl, &rendered)
	require.NoError(t, err)
	require.Len(t, generated.Projects, 2)
	assert.Equal(t, "infra-app-dev-main", generated.Projects[0].Name)
	assert.Equal(t, "infra/app", generated.Projects[0].Path)
	assert.Equal(t, "root-infra/app", generated.Projects[1].Name)
	assert.Contains(t, rendered.String(), "- dev.tfvars")
}

func Test_GenerateDetectedProjects_DefaultProjects(t *testing.T) {
	generated, err := generateSample(t, "version: 0.1\n", nil)
	require.NoError(t, err)
	require.Len(t, generated.Projects, 1)
	assert.Equal(t, "infra-app-dev", generated.Projects[0].Name)
	assert.Equal(t, config.ProjectTypeTerraform, generated.Projects[0].Type)
}

func Test_GenerateDetectedProjects_BackfillsDetectedTypes(t *testing.T) {
	p := autodetect.Project{Name: "stack", Path: "infra/app", Type: config.ProjectTypeCloudFormation}
	generated, err := config.Generate(t.Context(), sampleRepo(t),
		config.WithDetectedProjects([]autodetect.Project{p}, nil),
		config.WithTemplate("version: 0.1\nprojects:\n  - name: stack\n    path: infra/app\n"),
	)
	require.NoError(t, err)
	require.Len(t, generated.Projects, 1)
	assert.Equal(t, config.ProjectTypeCloudFormation, generated.Projects[0].Type)
}

func Test_GenerateDetectedProjects_RenderedOutputOnRunError(t *testing.T) {
	var rendered bytes.Buffer
	_, err := generateSample(t, "version: 0.1\nbefore: yes\n{{ .NoSuchField }}\n", &rendered)
	require.Error(t, err)
	assert.Equal(t, "version: 0.1\nbefore: yes\n", rendered.String())
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func Test_GenerateDetectedProjects_TemplateErrorBeatsWriteError(t *testing.T) {
	projects, rootModules := sampleProjects()
	_, err := config.Generate(t.Context(), sampleRepo(t),
		config.WithDetectedProjects(projects, rootModules),
		config.WithTemplate("version: 0.1\n{{ if }}\n"),
		config.WithWriter(failingWriter{}),
	)
	var te *config.TemplateError
	require.True(t, errors.As(err, &te), "want TemplateError, got %v", err)
	assert.Equal(t, config.TemplateStageSyntax, te.Stage)
	assert.ErrorContains(t, err, "disk full")
}

func Test_GenerateDetectedProjects_RenderedOutputWriteError(t *testing.T) {
	projects, rootModules := sampleProjects()
	_, err := config.Generate(t.Context(), sampleRepo(t),
		config.WithDetectedProjects(projects, rootModules),
		config.WithTemplate("version: 0.1\n"),
		config.WithWriter(failingWriter{}),
	)
	require.ErrorContains(t, err, "disk full")
}

func Test_GenerateDetectedProjects_TemplateErrors(t *testing.T) {
	tests := []struct {
		name         string
		tmpl         string
		stage        config.TemplateStage
		templateLine int
		renderedLine int
	}{
		{
			name:         "syntax error",
			tmpl:         "version: 0.1\nprojects:\n{{ range .DetectedProjects }\n",
			stage:        config.TemplateStageSyntax,
			templateLine: 3,
		},
		{
			name:         "run error",
			tmpl:         "version: 0.1\n\n{{ .NoSuchField }}\n",
			stage:        config.TemplateStageRun,
			templateLine: 3,
		},
		{
			name:         "bad yaml after lines the template removes",
			tmpl:         "{{- /* one */ -}}\n{{- /* two */ -}}\nversion: 0.1\nprojects: []\nfoo: bar\n  baz: qux\n",
			stage:        config.TemplateStageYAML,
			renderedLine: 4,
		},
		{
			name:  "unsupported version",
			tmpl:  "version: 0.9\n",
			stage: config.TemplateStageConfig,
		},
		{
			name:         "unknown field",
			tmpl:         "version: 0.1\nprojects:\n  - path: infra/app\n    bogus: 1\n",
			stage:        config.TemplateStageConfig,
			renderedLine: 4,
		},
		{
			name:  "project with no path",
			tmpl:  "version: 0.1\nprojects:\n  - name: nopath\n",
			stage: config.TemplateStageConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := generateSample(t, tt.tmpl, nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, config.ErrInvalidConfigTemplate)

			var te *config.TemplateError
			require.True(t, errors.As(err, &te), "want TemplateError, got %v", err)
			assert.Equal(t, tt.stage, te.Stage, te.Message)
			assert.Equal(t, tt.templateLine, te.TemplateLine, te.Message)
			assert.Equal(t, tt.renderedLine, te.RenderedLine, te.Message)
			assert.NotEmpty(t, te.Message)
			assert.NotContains(t, te.Message, "version: 0.1\n", "message should not repeat the template")
			assert.False(t, strings.HasPrefix(te.Message, "invalid config"), te.Message)
			assert.False(t, strings.HasPrefix(te.Message, "template:"), te.Message)
		})
	}
}

func Test_GenerateDetectedProjects_RunErrorColumn(t *testing.T) {
	_, err := generateSample(t, "version: 0.1\nfoo: {{ readFile \"missing.yml\" }}\n", nil)
	var te *config.TemplateError
	require.True(t, errors.As(err, &te), "want TemplateError, got %v", err)
	assert.Equal(t, config.TemplateStageRun, te.Stage)
	assert.Equal(t, 2, te.TemplateLine)
	assert.Equal(t, 9, te.TemplateColumn, "the column of readFile")
	assert.True(t, strings.HasPrefix(te.Message, "at <readFile"), te.Message)
}

func Test_GenerateDetectedProjects_SyntaxErrorHasNoColumn(t *testing.T) {
	_, err := generateSample(t, "version: 0.1\n{{ if }}\n", nil)
	var te *config.TemplateError
	require.True(t, errors.As(err, &te), "want TemplateError, got %v", err)
	assert.Equal(t, 2, te.TemplateLine)
	assert.Equal(t, 0, te.TemplateColumn)
}

func Test_TemplateError_ZeroValue(t *testing.T) {
	te := &config.TemplateError{Stage: config.TemplateStageConfig, Message: "bad"}
	assert.Equal(t, "bad", te.Error())
	assert.NoError(t, te.Unwrap())
}

func Test_GenerateDetectedProjects_MaxRenderedBytes(t *testing.T) {
	projects, rootModules := sampleProjects()
	var rendered bytes.Buffer
	_, err := config.Generate(t.Context(), sampleRepo(t),
		config.WithDetectedProjects(projects, rootModules),
		config.WithTemplate("{{ range 1000000 }}xxxxxxxxxx{{ end }}"),
		config.WithMaxRenderedBytes(1000),
		config.WithWriter(&rendered),
	)
	assert.ErrorIs(t, err, config.ErrInvalidConfigTemplate)
	var te *config.TemplateError
	require.True(t, errors.As(err, &te), "want TemplateError, got %v", err)
	assert.Equal(t, config.TemplateStageRun, te.Stage)
	assert.Contains(t, te.Message, config.ErrRenderedTooLarge.Error())
	assert.Equal(t, 1000, rendered.Len())
}
