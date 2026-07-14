package config

import (
	"fmt"

	"github.com/infracost/config/configfile"
	"gopkg.in/yaml.v3"
)

// This file is the semantic bridge between the file-syntax layer (package configfile) and the config
// API types. It interprets the opaque headings the file layer captures.
//
// NOTE: for now headings map to the current typed fields (terraform -> Terraform, aws ->
// ProjectAWSConfig, cdk -> CDK) so behaviour is unchanged. Routing the opaque remainder into the
// Plugins blob (the typed/blob split) lands with the read-path work; this bridge is the seam where
// that change will happen.

// fromConfigFile populates target from the neutral file representation, interpreting each heading.
// Structural fields copy across directly; known headings decode into their typed API fields; any
// other heading is an opaque plugin blob.
// MarshalYAML renders the API Config in the on-disk heading shape (one heading per plugin, no
// `plugins:` block) by going through the file layer. Every yaml.Marshal of a Config - generation's
// re-parse, env-var replacement, callers saving the file - therefore emits the file-syntax form.
func (c Config) MarshalYAML() (any, error) {
	return toConfigFile(&c)
}

// UnmarshalYAML decodes the on-disk heading shape into the API Config via the file layer, so every
// yaml.Unmarshal into a Config interprets headings (typed/blob split) consistently. It decodes into
// the receiver, preserving any defaults it was seeded with (see fromConfigFile).
func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	var fc configfile.Config
	if err := node.Decode(&fc); err != nil {
		return err
	}
	return fromConfigFile(&fc, c)
}

func fromConfigFile(fc *configfile.Config, target *Config) error {
	if fc.Version != "" {
		target.Version = fc.Version
	}
	// Only overwrite scalars the file actually set, so a Config seeded with defaults (e.g. currency
	// USD) keeps them when the file omits the key.
	if fc.Currency != "" {
		target.Currency = fc.Currency
	}
	if fc.UsageFile != "" {
		target.UsageFilePath = fc.UsageFile
	}

	for name, node := range fc.Headings {
		switch name {
		case "terraform":
			// repo-level terraform is the typed {source_map, defaults} section. Its defaults: sub-block
			// splits like a project's terraform: heading - the typed keys (cloud/spacelift/workspace)
			// decode into TerraformDefaults, and everything else is a repo-level terraform blob default
			// forwarded to the plugin. Because the split extracts the typed keys, the blob can never hold
			// them, so there is no collision when they recombine under defaults: on write.
			if err := decodeInto(node, &target.Terraform); err != nil {
				return fmt.Errorf("%w: terraform: %s", ErrInvalidConfigYAML, err)
			}
			if defaultsNode, ok := mappingChild(node, "defaults"); ok {
				_, blob, err := splitMapping(defaultsNode, terraformTypedKeys)
				if err != nil {
					return fmt.Errorf("%w: terraform defaults: %s", ErrInvalidConfigYAML, err)
				}
				if len(blob) > 0 {
					if target.Plugins == nil {
						target.Plugins = map[string]map[string]any{}
					}
					target.Plugins[string(ProjectTypeTerraform)] = blob
				}
			}
		case "cdk":
			// cdk is a typed section, not a plugin blob.
			if err := decodeInto(node, &target.CDK); err != nil {
				return fmt.Errorf("%w: cdk: %s", ErrInvalidConfigYAML, err)
			}
		default:
			// Repo-level plugin defaults live under a defaults: sub-key (consistent with
			// terraform.defaults). Only that sub-block is a repo default merged into projects; any other
			// direct key is ignored - config is plugin-agnostic and only understands the defaults:
			// convention it owns. Key like a project blob (aws -> cloudformation) so the fold in
			// normalize matches it to the projects it applies to.
			blob, err := decodeBlob(node)
			if err != nil {
				return fmt.Errorf("%w: heading %q: %s", ErrInvalidConfigYAML, name, err)
			}
			defaults, ok := blob["defaults"].(map[string]any)
			if !ok || len(defaults) == 0 {
				continue
			}
			if target.Plugins == nil {
				target.Plugins = map[string]map[string]any{}
			}
			target.Plugins[blobKeyForHeading(name, ProjectTypeUnknown)] = defaults
		}
	}

	// Only replace the target's projects when the file actually specified them (nil => key omitted,
	// so keep whatever defaults the caller seeded; non-nil => honour it, even if empty).
	if fc.Projects != nil {
		projects := make([]*Project, 0, len(fc.Projects))
		for i := range fc.Projects {
			p, err := projectFromConfigFile(&fc.Projects[i])
			if err != nil {
				return err
			}
			projects = append(projects, p)
		}
		target.Projects = projects
	}

	return nil
}

// projectFromConfigFile converts a single file-layer project into the API Project.
func projectFromConfigFile(fp *configfile.Project) (*Project, error) {
	p := &Project{
		Name:            fp.Name,
		Type:            ProjectType(fp.Type),
		Path:            fp.Path,
		Env:             fp.Env,
		Metadata:        fp.Metadata,
		EnvName:         fp.EnvName,
		UsageFile:       fp.UsageFile,
		YorConfigPath:   fp.YorConfigPath,
		ExcludePaths:    fp.ExcludePaths,
		DependencyPaths: fp.DependencyPaths,
		CDKSynthError:   fp.CDKSynthError,
	}

	for name, node := range fp.Headings {
		switch name {
		case "terraform":
			// split: the typed keys config reads outside the plugin stay on ProjectTerraform; the rest
			// (var_files, vars, and anything unrecognised) is the opaque blob forwarded to the plugin.
			typedNode, blob, err := splitMapping(node, terraformTypedKeys)
			if err != nil {
				return nil, fmt.Errorf("%w: project %q terraform: %s", ErrInvalidConfigYAML, p.Path, err)
			}
			if len(typedNode.Content) > 0 {
				if err := typedNode.Decode(&p.Terraform); err != nil {
					return nil, fmt.Errorf("%w: project %q terraform: %s", ErrInvalidConfigYAML, p.Path, err)
				}
			}
			setProjectPlugin(p, blobKeyForHeading("terraform", p.Type), blob)
		default:
			// aws and any other heading are opaque blobs; aws keys under cloudformation.
			blob, err := decodeBlob(node)
			if err != nil {
				return nil, fmt.Errorf("%w: project %q heading %q: %s", ErrInvalidConfigYAML, p.Path, name, err)
			}
			setProjectPlugin(p, blobKeyForHeading(name, p.Type), blob)
		}
	}

	mirrorBlobToDeprecated(p)

	return p, nil
}

// mirrorBlobToDeprecated populates the deprecated typed fields (Terraform.VarFiles/.Vars, AWS) from
// the canonical Plugins blob, so consumers still reading them get real data during the deprecation
// window. The blob remains the source of truth and the only representation serialized.
func mirrorBlobToDeprecated(p *Project) {
	if blob := p.Plugins[terraformBlobKey(p.Type)]; blob != nil {
		if v, ok := blob["var_files"]; ok {
			p.Terraform.VarFiles = anyToStringSlice(v)
		}
		if v, ok := blob["vars"].(map[string]any); ok {
			// deep-copy so the deprecated mirror never aliases the canonical blob's map.
			if cp, ok := deepCopyAny(v).(map[string]any); ok {
				p.Terraform.Vars = cp
			}
		}
	}
	if blob := p.Plugins[string(ProjectTypeCloudFormation)]; blob != nil {
		p.AWS = ProjectAWSConfig{
			Region:    anyToString(blob["region"]),
			AccountID: anyToString(blob["account_id"]),
			StackID:   anyToString(blob["stack_id"]),
			StackName: anyToString(blob["stack_name"]),
		}
	}
}

// anyToStringSlice coerces a decoded blob value into []string. Values that have been through a YAML
// round-trip are []any of strings; ones built in-process may be []string already.
func anyToStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func anyToString(v any) string {
	s, _ := v.(string)
	return s
}

// setProjectPlugin stores a non-empty blob under p.Plugins[key], allocating the maps as needed.
func setProjectPlugin(p *Project, key string, blob map[string]any) {
	if len(blob) == 0 {
		return
	}
	if p.Plugins == nil {
		p.Plugins = map[string]map[string]any{}
	}
	p.Plugins[key] = blob
}

// decodeInto decodes a captured heading node into out. The node is copied to a local first because
// yaml.Node.Decode has a pointer receiver and map-indexed values aren't addressable.
func decodeInto(node yaml.Node, out any) error {
	n := node
	return n.Decode(out)
}

func decodeBlob(node yaml.Node) (map[string]any, error) {
	var blob map[string]any
	if err := decodeInto(node, &blob); err != nil {
		return nil, err
	}
	return blob, nil
}

// toConfigFile is the inverse of fromConfigFile: it renders the API Config back into the neutral file
// representation, emitting each typed section as its heading. It mirrors fromConfigFile so a
// Config -> configfile -> Config round-trip is an identity (which is what the generate marshal /
// re-parse cycle relies on).
func toConfigFile(c *Config) (*configfile.Config, error) {
	fc := &configfile.Config{
		Version:   c.Version,
		Currency:  c.Currency,
		UsageFile: c.UsageFilePath,
	}

	// Repo-level plugin defaults render under their heading (cloudformation -> aws) wrapped in a
	// defaults: sub-key - the convention every plugin uses for repo-level defaults, matching
	// terraform.defaults. (terraform's own defaults live in its typed section below.)
	for key, blob := range c.Plugins {
		if len(blob) == 0 || key == string(ProjectTypeTerraform) {
			// terraform's repo blob defaults are merged into its typed section below.
			continue
		}
		if err := setHeading(&fc.Headings, headingForBlobKey(key), map[string]any{"defaults": blob}); err != nil {
			return nil, err
		}
	}

	// Repo-level typed terraform ({source_map, defaults}), with any repo-level terraform blob defaults
	// merged under defaults: alongside the typed ones (their keys are disjoint by construction).
	// source_map stays here as the deprecated typed field; it is folded into each project as
	// regex_source_map during normalize.
	var tfNode yaml.Node
	if err := tfNode.Encode(c.Terraform); err != nil {
		return nil, fmt.Errorf("%w: encode terraform: %s", ErrInvalidConfigYAML, err)
	}
	if blob := c.Plugins[string(ProjectTypeTerraform)]; len(blob) > 0 {
		if err := mergeUnderDefaults(&tfNode, blob); err != nil {
			return nil, fmt.Errorf("%w: encode terraform defaults: %s", ErrInvalidConfigYAML, err)
		}
	}
	if len(tfNode.Content) > 0 {
		if fc.Headings == nil {
			fc.Headings = map[string]yaml.Node{}
		}
		fc.Headings["terraform"] = tfNode
	}

	if err := setHeading(&fc.Headings, "cdk", c.CDK); err != nil {
		return nil, err
	}

	if c.Projects != nil {
		fc.Projects = make([]configfile.Project, 0, len(c.Projects))
		for _, p := range c.Projects {
			fp, err := toConfigFileProject(p)
			if err != nil {
				return nil, err
			}
			fc.Projects = append(fc.Projects, *fp)
		}
	}

	return fc, nil
}

func toConfigFileProject(p *Project) (*configfile.Project, error) {
	fp := &configfile.Project{
		Name:            p.Name,
		Type:            string(p.Type),
		Path:            p.Path,
		Env:             p.Env,
		Metadata:        p.Metadata,
		EnvName:         p.EnvName,
		UsageFile:       p.UsageFile,
		YorConfigPath:   p.YorConfigPath,
		ExcludePaths:    p.ExcludePaths,
		DependencyPaths: p.DependencyPaths,
		CDKSynthError:   p.CDKSynthError,
	}

	// Each plugin blob renders under its heading (terraform-family -> terraform:, cloudformation ->
	// aws:, others keep their name).
	for key, blob := range p.Plugins {
		if len(blob) == 0 {
			continue
		}
		var n yaml.Node
		if err := n.Encode(blob); err != nil {
			return nil, fmt.Errorf("%w: encode blob %q: %s", ErrInvalidConfigYAML, key, err)
		}
		mergeHeading(&fp.Headings, headingForBlobKey(key), n)
	}

	// The typed terraform fields (workspace/cloud/spacelift) merge into the terraform: heading ahead of
	// the blob keys, so they read first.
	typedNode, err := filterEncode(p.Terraform, terraformTypedKeys)
	if err != nil {
		return nil, fmt.Errorf("%w: encode terraform: %s", ErrInvalidConfigYAML, err)
	}
	if len(typedNode.Content) > 0 {
		if existing, ok := fp.Headings["terraform"]; ok {
			fp.Headings["terraform"] = concatMappings(typedNode, existing)
		} else {
			if fp.Headings == nil {
				fp.Headings = map[string]yaml.Node{}
			}
			fp.Headings["terraform"] = typedNode
		}
	}

	return fp, nil
}

// mergeHeading stores node under headings[name], concatenating if that heading already has content
// (so multiple plugin keys that map to the same heading don't clobber each other).
func mergeHeading(headings *map[string]yaml.Node, name string, node yaml.Node) {
	if *headings == nil {
		*headings = map[string]yaml.Node{}
	}
	if existing, ok := (*headings)[name]; ok {
		(*headings)[name] = concatMappings(existing, node)
		return
	}
	(*headings)[name] = node
}

// splitMapping partitions a mapping node into a node holding the given keys and a generic map holding
// the rest.
func splitMapping(node yaml.Node, keys map[string]bool) (yaml.Node, map[string]any, error) {
	kept := yaml.Node{Kind: yaml.MappingNode}
	rest := map[string]any{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k, v := node.Content[i], node.Content[i+1]
		if keys[k.Value] {
			kept.Content = append(kept.Content, k, v)
			continue
		}
		var val any
		if err := v.Decode(&val); err != nil {
			return kept, nil, err
		}
		rest[k.Value] = val
	}
	return kept, rest, nil
}

// filterEncode encodes v to a mapping node and keeps only the given keys.
func filterEncode(v any, keys map[string]bool) (yaml.Node, error) {
	var full yaml.Node
	if err := full.Encode(v); err != nil {
		return full, err
	}
	out := yaml.Node{Kind: yaml.MappingNode}
	for i := 0; i+1 < len(full.Content); i += 2 {
		if keys[full.Content[i].Value] {
			out.Content = append(out.Content, full.Content[i], full.Content[i+1])
		}
	}
	return out, nil
}

// concatMappings returns a mapping node with a's key/value pairs followed by b's.
func concatMappings(a, b yaml.Node) yaml.Node {
	out := yaml.Node{Kind: yaml.MappingNode}
	out.Content = append(out.Content, a.Content...)
	out.Content = append(out.Content, b.Content...)
	return out
}

// mappingChild returns the value node for key in a mapping node.
func mappingChild(node yaml.Node, key string) (yaml.Node, bool) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return *node.Content[i+1], true
		}
	}
	return yaml.Node{}, false
}

// mergeUnderDefaults appends blob's key/value pairs to the defaults: child of a terraform heading
// node (creating defaults: if absent). Callers guarantee the blob's keys are disjoint from the typed
// defaults (the read split extracts the typed keys), so no key is emitted twice.
func mergeUnderDefaults(node *yaml.Node, blob map[string]any) error {
	var blobNode yaml.Node
	if err := blobNode.Encode(blob); err != nil {
		return err
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "defaults" {
			node.Content[i+1].Content = append(node.Content[i+1].Content, blobNode.Content...)
			return nil
		}
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "defaults"},
		&blobNode,
	)
	return nil
}

// setHeading encodes a typed value into a heading node and stores it, skipping values that encode to
// an empty mapping so we never emit a bare `heading: {}`.
func setHeading(headings *map[string]yaml.Node, name string, v any) error {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return fmt.Errorf("%w: encode heading %q: %s", ErrInvalidConfigYAML, name, err)
	}
	if len(n.Content) == 0 {
		return nil
	}
	if *headings == nil {
		*headings = map[string]yaml.Node{}
	}
	(*headings)[name] = n
	return nil
}
