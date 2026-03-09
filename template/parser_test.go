package template

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestParser_Debug(t *testing.T) {
	p := NewParser(".", Variables{}, nil)

	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{
			name:     "string",
			input:    "hello world",
			expected: "DEBUG: \"hello world\"\n",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "DEBUG: \"\"\n",
		},
		{
			name:     "int",
			input:    42,
			expected: "DEBUG: 42\n",
		},
		{
			name:     "float",
			input:    3.14,
			expected: "DEBUG: 3.14\n",
		},
		{
			name:     "bool",
			input:    true,
			expected: "DEBUG: true\n",
		},
		{
			name:     "nil",
			input:    nil,
			expected: "DEBUG: <nil>\n",
		},
		{
			name:     "string slice",
			input:    []string{"a", "b", "c"},
			expected: "DEBUG: []string{\"a\", \"b\", \"c\"}\n",
		},
		{
			name:     "int slice",
			input:    []int{1, 2, 3},
			expected: "DEBUG: []int{1, 2, 3}\n",
		},
		{
			name:     "map",
			input:    map[string]string{"key": "value"},
			expected: "DEBUG: map[string]string{\"key\":\"value\"}\n",
		},
		{
			name: "struct",
			input: struct {
				Name string
				Age  int
			}{Name: "test", Age: 30},
			expected: "DEBUG: struct { Name string; Age int }{Name:\"test\", Age:30}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			require.NoError(t, err)

			origStderr := os.Stderr
			os.Stderr = w

			p.debug(tt.input)

			os.Stderr = origStderr
			_ = w.Close()

			var buf bytes.Buffer
			_, err = buf.ReadFrom(r)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestParser_Compile(t *testing.T) {
	tests := []struct {
		name      string
		variables Variables
		isProd    func(string) bool
	}{
		{
			name: "different env dirs",
		},
		{
			name: "different env files",
		},
		{
			name: "external dirs",
		},
		{
			name: "include directory based on file",
		},
		{
			name: "with string matching functions",
		},
		{
			name: "with string manipulation functions",
		},
		{
			name: "with top level template data",
			variables: Variables{
				Branch:     "test",
				BaseBranch: "master",
			},
		},
		{
			name: "with rel paths",
		},
		{
			name: "with list",
		},
		{
			name: "with is dir",
		},
		{
			name: "with parse functions",
		},
		{
			name: "with is production",
			isProd: func(name string) bool {
				return !slices.Contains([]string{
					"test2",
					"foo1",
				}, name)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDataPath := filepath.Join("./testdata", strings.ReplaceAll(tt.name, " ", "_"))
			input := filepath.Join(testDataPath, "infracost.yml.tmpl")
			golden := filepath.Join(testDataPath, "infracost.golden.yml")

			f, err := os.Open(golden)
			require.NoError(t, err)

			p := NewParser(testDataPath, tt.variables, tt.isProd)

			wr := &bytes.Buffer{}
			err = p.CompileFromFile(input, wr)
			require.NoError(t, err)

			contents, err := os.ReadFile(input)
			require.NoError(t, err)

			wrr := &bytes.Buffer{}
			err = p.Compile(string(contents), wrr)
			require.NoError(t, err)

			var actualFileOutput any
			err = yaml.NewDecoder(wr).Decode(&actualFileOutput)
			require.NoError(t, err)

			var actualStringOutput any
			err = yaml.NewDecoder(wrr).Decode(&actualStringOutput)
			require.NoError(t, err)

			var expected any
			err = yaml.NewDecoder(f).Decode(&expected)
			require.NoError(t, err)
			assert.Equal(t, expected, actualFileOutput)
			assert.Equal(t, expected, actualStringOutput)
		})
	}
}
