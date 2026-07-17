package configfile

import "gopkg.in/yaml.v3"

// Project is the on-disk representation of a single entry in the config file's projects list. The
// structural fields are the keys config passes through untouched; every other top-level key is an
// opaque heading captured in Headings (e.g. terraform, aws, kubernetes). The semantic layer decides
// what those headings mean.
type Project struct {
	Name            string            `yaml:"name,omitempty"`
	Type            string            `yaml:"type,omitempty"`
	Path            string            `yaml:"path"`
	Env             map[string]string `yaml:"env,omitempty"`
	Metadata        map[string]string `yaml:"metadata,omitempty"`
	EnvName         string            `yaml:"env_name,omitempty"`
	UsageFile       string            `yaml:"usage_file,omitempty"`
	YorConfigPath   string            `yaml:"yor_config_path,omitempty"`
	ExcludePaths    []string          `yaml:"exclude_paths,omitempty"`
	DependencyPaths []string          `yaml:"dependency_paths,omitempty"`
	CDKSynthError   string            `yaml:"cdk_synth_error,omitempty"`

	// Headings holds every top-level key that is not one of the structural fields above, keyed by the
	// heading name (terraform, aws, kubernetes, ...) and captured as a raw node so the semantic layer
	// can interpret or forward it without this package understanding its contents. Never serialized
	// directly (yaml:"-"); MarshalYAML re-emits its entries as top-level keys.
	Headings map[string]yaml.Node `yaml:"-"`
}

// structuralProjectKeys is the set of top-level project keys that map to the structural fields above.
// Any other key is captured as a heading. Kept in sync with the yaml tags on Project by the test
// TestProject_StructuralKeysMatchTags.
var structuralProjectKeys = map[string]bool{
	"name":             true,
	"type":             true,
	"path":             true,
	"env":              true,
	"metadata":         true,
	"env_name":         true,
	"usage_file":       true,
	"yor_config_path":  true,
	"exclude_paths":    true,
	"dependency_paths": true,
	"cdk_synth_error":  true,
}

// UnmarshalYAML decodes the structural fields normally, then captures every remaining top-level key
// as an opaque heading. Using a local alias type for the structural decode avoids recursing back into
// this method.
func (p *Project) UnmarshalYAML(node *yaml.Node) error {
	type structural Project
	var s structural
	if err := node.Decode(&s); err != nil {
		return err
	}
	*p = Project(s)

	var all map[string]yaml.Node
	if err := node.Decode(&all); err != nil {
		return err
	}
	return captureHeadings(all, structuralProjectKeys, &p.Headings)
}

// MarshalYAML emits the structural fields followed by the captured headings as top-level keys, so a
// project round-trips to one heading per plugin with no separate `plugins:` block. Heading keys are
// emitted in sorted order for deterministic output.
func (p Project) MarshalYAML() (any, error) {
	type structural Project
	var out yaml.Node
	if err := out.Encode(structural(p)); err != nil {
		return nil, err
	}

	for _, key := range sortedKeys(p.Headings) {
		n := p.Headings[key]
		out.Content = append(out.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: key},
			&n,
		)
	}

	return &out, nil
}
