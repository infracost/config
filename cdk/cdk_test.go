package cdk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineCDKLanguage(t *testing.T) {
	tests := []struct {
		name    string
		app     string
		want    Language
		wantErr string
	}{
		// TypeScript
		{name: "ts-node", app: "npx ts-node --prefer-ts-exts bin/app.ts", want: LanguageTypeScript},
		{name: "ts extension", app: "node bin/app.ts", want: LanguageTypeScript},
		{name: "ts preamble semicolon", app: ". .nvm/nvm.sh; npx ts-node app.ts", want: LanguageTypeScript},

		// JavaScript
		{name: "node", app: "node bin/app.js", want: LanguageJavaScript},
		{name: "npx", app: "npx cdk-app", want: LanguageJavaScript},
		{name: "npm", app: "npm run synth", want: LanguageJavaScript},
		{name: "yarn", app: "yarn synth", want: LanguageJavaScript},

		// Python
		{name: "python3", app: "python3 app.py", want: LanguagePython},
		{name: "python", app: "python app.py", want: LanguagePython},
		{name: "pipenv", app: "pipenv run python app.py", want: LanguagePython},
		{name: "poetry", app: "poetry run python app.py", want: LanguagePython},
		{name: "uv", app: "uv run python app.py", want: LanguagePython},

		// Shell preamble stripping
		{name: "activate semicolon", app: ". .venv/bin/activate; python app.py", want: LanguagePython},
		{name: "source activate semicolon", app: "source .venv/bin/activate; python3 app.py", want: LanguagePython},
		{name: "activate double ampersand", app: ". .venv/bin/activate && python app.py", want: LanguagePython},

		// Path-based tool names
		{name: "relative venv python", app: "../.venv/bin/python app.py", want: LanguagePython},
		{name: "absolute python3", app: "/usr/bin/python3 app.py", want: LanguagePython},
		{name: "venv with preamble", app: ". .venv/bin/activate && .venv/bin/python app.py", want: LanguagePython},

		// Errors
		{name: "preamble only", app: ". .venv/bin/activate;", wantErr: "invalid app"},
		{name: "unsupported tool", app: "java -jar app.jar", wantErr: "unsupported tool"},
		{name: "empty app", app: "", wantErr: "invalid app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			content := `{"app": "` + tt.app + `"}`
			require.NoError(t, os.WriteFile(filepath.Join(dir, "cdk.json"), []byte(content), 0600))

			lang, err := DetermineCDKLanguage(dir, "cdk.json")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, lang)
			}
		})
	}
}
