package autodetect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentifyCloudFormationPath(t *testing.T) {
	dir := t.TempDir()
	write := func(t *testing.T, name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0600))
		return path
	}

	tests := []struct {
		name    string
		file    string
		content string
		want    bool
	}{
		{
			name:    "json template with typed resources",
			file:    "template.json",
			content: `{"Resources": {"Bucket": {"Type": "AWS::S3::Bucket"}}}`,
			want:    true,
		},
		{
			name:    "yaml template with typed resources",
			file:    "template.yaml",
			content: "Resources:\n  Instance:\n    Type: AWS::EC2::Instance\n",
			want:    true,
		},
		{
			name:    "yaml template with format version only",
			file:    "version.yml",
			content: "AWSTemplateFormatVersion: '2010-09-09'\n",
			want:    true,
		},
		{
			// encoding/json matches keys case-insensitively, so the byte
			// pre-filter must not reject a template with unusually-cased keys.
			name:    "json template with lower-case keys",
			file:    "lower.json",
			content: `{"awstemplateformatversion": "2010-09-09"}`,
			want:    true,
		},
		{
			name:    "package.json is rejected",
			file:    "package.json",
			content: `{"name": "not-a-template", "version": "1.0.0"}`,
			want:    false,
		},
		{
			name:    "ci yaml is rejected",
			file:    "ci.yml",
			content: "jobs:\n  build:\n    steps: []\n",
			want:    false,
		},
		{
			// "resources" without a "::" type separator must not pass the
			// pre-filter (nor IsCF).
			name:    "k8s-style resources without type separator is rejected",
			file:    "limits.yaml",
			content: "resources:\n  limits:\n    cpu: 100m\n",
			want:    false,
		},
		{
			name:    "non json/yaml extension is rejected",
			file:    "template.txt",
			content: `{"AWSTemplateFormatVersion": "2010-09-09"}`,
			want:    false,
		},
		{
			name:    "template under node_modules is rejected as CDK origin",
			file:    filepath.Join("node_modules", "pkg", "template.json"),
			content: `{"Resources": {"Bucket": {"Type": "AWS::S3::Bucket"}}}`,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := write(t, tt.file, tt.content)
			assert.Equal(t, tt.want, IdentifyCloudFormationPath(path))
		})
	}
}
