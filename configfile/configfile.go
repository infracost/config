// Package configfile is the file-syntax layer for the Infracost config file. It models the on-disk
// shape only: a project (or the repo root) is a set of structural fields plus an arbitrary set of
// named "headings" (terraform:, aws:, kubernetes:, ...) captured verbatim as opaque nodes.
//
// It deliberately does NOT understand what any heading means - the typed/blob split, the
// aws->cloudformation mapping, the terraform-family collapse, etc. all live in the semantic layer
// (package config), which imports this package and converts between configfile.* and the config API
// types. Keeping the semantics out of here is what lets this package stay a pure, dependency-light
// representation of the file and avoids an import cycle with package config.
//
// The file is split by type: this file holds the whole-file (Config) representation, configproject.go
// holds a single project entry.
package configfile

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// captureHeadings stores every non-structural top-level key from all (keyed by structural) into
// headings. A heading is a plugin blob, which is always a mapping (raw_options is a JSON object), so
// a non-mapping value under an unknown key is a typo of a structural field, not a blob, and is
// rejected with a clear error rather than silently captured. An explicitly empty/null value is
// allowed (an empty heading).
func captureHeadings(all map[string]yaml.Node, structural map[string]bool, headings *map[string]yaml.Node) error {
	for key, n := range all {
		if structural[key] {
			continue
		}
		if !isHeadingNode(n) {
			return fmt.Errorf("unknown field %q", key)
		}
		if *headings == nil {
			*headings = map[string]yaml.Node{}
		}
		(*headings)[key] = n
	}
	return nil
}

// isHeadingNode reports whether a captured value can be a plugin blob: a mapping, or an explicitly
// empty/null value. Scalars and sequences cannot be a raw_options blob.
func isHeadingNode(n yaml.Node) bool {
	switch n.Kind {
	case yaml.MappingNode:
		return true
	case yaml.ScalarNode:
		return n.Tag == "!!null" || n.Value == ""
	default:
		return false
	}
}

// Config is the on-disk representation of the whole config file. The structural fields are the keys
// config passes through untouched; every other top-level key is an opaque heading captured in
// Headings (e.g. terraform for repo-level defaults, cdk, or repo-level plugin defaults). The semantic
// layer (package config) decides what those headings mean.
type Config struct {
	Version    string    `yaml:"version"`
	Currency   string    `yaml:"currency,omitempty"`
	UsageFile  string    `yaml:"usage_file,omitempty"`
	Projects   []Project `yaml:"projects"`
	Autodetect yaml.Node `yaml:"autodetect,omitempty"`

	// Headings holds every top-level key that is not one of the structural fields above (terraform,
	// cdk, and future repo-level plugin defaults), captured raw. Never serialized directly
	// (yaml:"-"); MarshalYAML re-emits its entries as top-level keys.
	Headings map[string]yaml.Node `yaml:"-"`
}

// structuralConfigKeys is the set of top-level file keys that map to the structural fields above. Any
// other key is captured as a heading. Kept in sync with the yaml tags on Config by the test
// TestConfig_StructuralKeysMatchTags.
var structuralConfigKeys = map[string]bool{
	"version":    true,
	"currency":   true,
	"usage_file": true,
	"projects":   true,
	"autodetect": true,
}

// UnmarshalYAML decodes the structural fields normally, then captures every remaining top-level key
// as an opaque heading. A local alias type avoids recursing back into this method.
func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	type structural Config
	var s structural
	if err := node.Decode(&s); err != nil {
		return err
	}
	*c = Config(s)

	var all map[string]yaml.Node
	if err := node.Decode(&all); err != nil {
		return err
	}
	return captureHeadings(all, structuralConfigKeys, &c.Headings)
}

// MarshalYAML emits the structural fields followed by the captured headings as top-level keys (sorted
// for deterministic output), so the file round-trips to one heading per plugin with no `plugins:`
// block.
func (c Config) MarshalYAML() (any, error) {
	type structural Config
	var out yaml.Node
	if err := out.Encode(structural(c)); err != nil {
		return nil, err
	}

	for _, key := range sortedKeys(c.Headings) {
		n := c.Headings[key]
		out.Content = append(out.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			&n,
		)
	}

	return &out, nil
}

// sortedKeys returns the keys of a heading map in deterministic (sorted) order, used when re-emitting
// captured headings so marshalled output is stable.
func sortedKeys(m map[string]yaml.Node) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
