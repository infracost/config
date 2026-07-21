package autodetect

import (
	"testing"

	"github.com/infracost/config/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttributeVarFiles(t *testing.T) {
	files := []TFVarsFile{
		{AbsolutePath: "/repo/apps/envs/blue.tfvars", Env: "blue"},
		{AbsolutePath: "/repo/apps/envs/green.tfvars", Env: "green"},
		{AbsolutePath: "/repo/apps/envs/default.tfvars", IsGlobal: true},
	}

	tests := []struct {
		name             string
		projectAbsPath   string
		projectBaseIsEnv bool
		projectBaseEnv   string
		want             []plugin.AttributedVarFile
	}{
		{
			// A non-env project directory (e.g. a component) keeps every attributed file, so the
			// plugin fans it out into one project per environment.
			name:             "non-env project dir keeps all envs",
			projectAbsPath:   "/repo/apps/component",
			projectBaseIsEnv: false,
			want: []plugin.AttributedVarFile{
				{Path: "../envs/blue.tfvars", Env: "blue"},
				{Path: "../envs/green.tfvars", Env: "green"},
				{Path: "../envs/default.tfvars", IsGlobal: true},
			},
		},
		{
			// An env-named project directory only keeps its own environment's files (plus globals),
			// so it does not fan out into a project per sibling environment (FIX-453). "blue" is a
			// custom env the plugin's default matcher would not recognise, which is exactly why this
			// collapse has to happen caller-side.
			name:             "env-named project dir collapses to its own env",
			projectAbsPath:   "/repo/apps/blue",
			projectBaseIsEnv: true,
			projectBaseEnv:   "blue",
			want: []plugin.AttributedVarFile{
				{Path: "../envs/blue.tfvars", Env: "blue"},
				{Path: "../envs/default.tfvars", IsGlobal: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := attributeVarFiles(tt.projectAbsPath, files, tt.projectBaseIsEnv, tt.projectBaseEnv)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
