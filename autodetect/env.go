package autodetect

import (
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var (
	defaultEnvs = []string{
		"prd",
		"prod",
		"production",
		"preprod",
		"staging",
		"stage",
		"stg",
		"stag",
		"development",
		"dev",
		"release",
		"testing",
		"test",
		"tst",
		"qa",
		"uat",
		"live",
		"sandbox",
		"sbx",
		"sbox",
		"demo",
		"integration",
		"int",
		"experimental",
		"experiments",
		"trial",
		"validation",
		"perf",
		"sec",
		"dr",
		"load",
		"management",
		"mgmt",
		"playground",
	}

	defaultExcludedDirs = []string{
		".infracost",
		".git",
		".terraform",
		".terragrunt-cache",
		"example",
		"examples",
	}
)

// EnvFileMatcher is used to match environment specific var files.
type EnvFileMatcher struct {
	envNames   []string
	envLookup  map[string]struct{}
	extensions []string
	wildcards  []string
}

func CreateEnvFileMatcher(names []string, extensions []string) *EnvFileMatcher {
	if extensions == nil {
		// create a matcher with the .json extensions as well so that we support
		// tfars-env.json use case.
		extensions = append(defaultTFVarExtensions, ".json") // nolint
	}

	if len(names) == 0 {
		return CreateEnvFileMatcher(defaultEnvs, extensions)
	}

	// Sort the extensions by length so that we always prefer the longest extension
	// when matching a file.
	sort.Slice(extensions, func(i, j int) bool {
		return len(extensions[i]) > len(extensions[j])
	})

	envNames := make([]string, 0, len(names))
	var wildcards []string
	for _, name := range names {
		// envNames can contain wildcards, we need to handle them separately. e.g: dev-*
		// will create separate envs for dev-staging and dev-legacy. We don't want these
		// wildcards to appear in the envNames list as this will create unwanted env
		// grouping.
		if strings.Contains(name, "*") || strings.Contains(name, "?") {
			wildcards = append(wildcards, strings.ToLower(name))
			continue
		}

		// ensure all env names to lowercase, so we can match case insensitively.
		envNames = append(envNames, strings.ToLower(name))
	}

	lookup := make(map[string]struct{}, len(names))
	for _, name := range envNames {
		lookup[name] = struct{}{}
	}

	return &EnvFileMatcher{
		envNames:   envNames,
		envLookup:  lookup,
		extensions: extensions,
		wildcards:  wildcards,
	}
}

// IsAutoVarFile checks if the var file is an auto.tfvars or terraform.tfvars.
// These are special Terraform var files that are applied to every project
// automatically.
func IsAutoVarFile(file string) bool {
	withoutJSONSuffix := strings.TrimSuffix(file, ".json")

	return strings.HasSuffix(withoutJSONSuffix, ".auto.tfvars") || withoutJSONSuffix == "terraform.tfvars"
}

// IsGlobalVarFile checks if the var file is a global var file.
func (e *EnvFileMatcher) IsGlobalVarFile(file string) bool {
	return !e.IsEnvName(file)
}

// IsEnvName checks if the var file is an environment specific var file.
func (e *EnvFileMatcher) IsEnvName(file string) bool {
	clean := e.clean(file)
	_, ok := e.envLookup[clean]
	if ok {
		return true
	}

	for _, name := range e.envNames {
		if e.hasEnvPrefix(clean, name) || e.hasEnvSuffix(clean, name) {
			return true
		}
	}

	for _, wildcard := range e.wildcards {
		if isMatch, _ := path.Match(wildcard, clean); isMatch {
			return true
		}
	}

	return false
}

// clean removes the extension from the file name and converts it to lowercase.
// This should be used to clean the file name before matching it to an env name.
func (e *EnvFileMatcher) clean(name string) string {
	base := filepath.Base(name)

	for _, ext := range e.extensions {
		base = strings.TrimSuffix(base, ext)
	}

	// remove the leading . from the stem as this will affect env name matching
	return strings.TrimPrefix(strings.ToLower(base), ".")
}

// EnvName returns the environment name for the given var file.
func (e *EnvFileMatcher) EnvName(file string) string {
	// if we have a direct match to an env name, return it.
	clean := e.clean(file)
	_, ok := e.envLookup[clean]
	if ok {
		return clean
	}

	// if we have a wildcard match to an env name return the clean name now
	// as the partial match logic can collide with wildcard matches.
	for _, wildcard := range e.wildcards {
		if isMatch, _ := path.Match(wildcard, clean); isMatch {
			return clean
		}
	}

	// if we have a partial suffix match to an env name return the partial match
	// which is the longest match. This is likely to be the better match. e.g: if we
	// have both dev and legacy-dev as defined envNames, given a tfvar named
	// legacy-dev-staging legacy-dev should be the env name returned.
	var match string
	for _, name := range e.envNames {
		if e.hasEnvSuffix(clean, name) {
			if len(name) > len(match) {
				match = name
			}
		}
	}

	if match != "" {
		return match
	}

	// repeat the same process for suffixes but with prefix matches.
	for _, name := range e.envNames {
		if e.hasEnvPrefix(clean, name) {
			if len(name) > len(match) {
				match = name
			}
		}
	}

	if match != "" {
		return match
	}

	return clean
}

// PathEnv returns the env name detected in file path when it matches defined envs. Otherwise returns an empty string.
func (e *EnvFileMatcher) PathEnv(file string) string {
	env := e.EnvName(file)

	_, ok := e.envLookup[env]
	if ok {
		return env
	}

	return ""
}

func (e *EnvFileMatcher) hasEnvPrefix(clean string, name string) bool {
	return strings.HasPrefix(clean, name+"-") || strings.HasPrefix(clean, name+"_") || strings.HasPrefix(clean, name+".")
}

func (e *EnvFileMatcher) hasEnvSuffix(clean string, name string) bool {
	return strings.HasSuffix(clean, "_"+name) || strings.HasSuffix(clean, "-"+name) || strings.HasSuffix(clean, "."+name)
}
