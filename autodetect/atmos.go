package autodetect

import (
	"fmt"
	"path/filepath"

	"github.com/infracost/config/atmos"
	"github.com/infracost/config/types"
)

// DetectAtmosProjects discovers Atmos stack-components in rootDir, emitting one
// project per deployable (stack, component) pair. Atmos expands a single component
// directory into N projects (one per stack), so each project gets a unique synthetic
// Name/Path and carries the stack/component in Metadata and Env; the parser plugin
// re-resolves them from atmos.yaml at parse time.
//
// It returns the projects, the set of directories Atmos covers (the Terraform
// components base — so overlapping plain-Terraform autodetect projects can be filtered
// out), and any detection error. If rootDir is not an Atmos repo (no resolvable
// atmos.yaml), it returns nothing without an error. An error indicates a valid Atmos
// repo was found but could not be fully processed (e.g. unsupported stack naming).
func DetectAtmosProjects(rootDir string) ([]Project, map[string]bool, error) {
	cfg, err := atmos.LoadConfig(rootDir)
	if err != nil {
		// Distinguish "no atmos.yaml" (not an Atmos repo — silent) from "atmos.yaml
		// present but malformed" (surfaced as an error so the user gets feedback
		// instead of silently falling through to plain-Terraform detection).
		if !atmos.IsAtmosRepo(rootDir) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("atmos: failed to load config in %s: %w", rootDir, err)
	}

	stackComponents, err := atmos.Enumerate(rootDir)
	if err != nil {
		return nil, nil, fmt.Errorf("atmos detection in %s: %w", rootDir, err)
	}
	if len(stackComponents) == 0 {
		return nil, nil, nil
	}

	tfBaseRel, err := filepath.Rel(rootDir, cfg.TerraformBasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("atmos: could not resolve terraform base path: %w", err)
	}
	stacksRel, err := filepath.Rel(rootDir, cfg.StacksBasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("atmos: could not resolve stacks base path: %w", err)
	}

	// Watch the whole Terraform components base (a component may reference sibling
	// local modules, e.g. "../shared") and the configured stacks base (a stack's
	// vars/providers come from manifests there), plus atmos.yaml. This conservatively
	// over-invalidates rather than risk missing a relevant change; per-component
	// module-graph precision is a future optimization.
	dependencyPaths := []string{
		filepath.ToSlash(tfBaseRel) + "/**",
		filepath.ToSlash(stacksRel) + "/**",
		"atmos.yaml",
	}

	projects := make([]Project, 0, len(stackComponents))
	for _, sc := range stackComponents {
		projects = append(projects, Project{
			Name: fmt.Sprintf("atmos/%s/%s", sc.Stack, sc.Component),
			Path: fmt.Sprintf("atmos/%s/%s", sc.Stack, sc.Component),
			Type: types.ProjectTypeAtmos,
			Env:  sc.Stack,
			Metadata: map[string]string{
				"atmos_stack":     sc.Stack,
				"atmos_component": sc.Component,
			},
			DependencyPaths: dependencyPaths,
		})
	}

	return projects, map[string]bool{filepath.ToSlash(tfBaseRel): true}, nil
}
