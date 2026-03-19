package autodetect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DetectCiscoStacksProjects scans for Cisco Stacks projects by looking for
// stacks/*/base/ directories and enumerating their layers.
// Returns the projects and a set of stack names that produced at least one
// layer project (for filtering terraform autodetect projects that overlap).
func DetectCiscoStacksProjects(rootDir string) ([]Project, map[string]bool) {
	var projects []Project
	stackNames := make(map[string]bool)

	stacksDir := filepath.Join(rootDir, "stacks")
	stacks, err := os.ReadDir(stacksDir)
	if err != nil {
		return []Project{}, map[string]bool{}
	}

	for _, stack := range stacks {
		if !stack.IsDir() {
			continue
		}

		baseDir := filepath.Join(stacksDir, stack.Name(), "base")
		if _, err := os.Stat(baseDir); err != nil {
			continue
		}

		layersDir := filepath.Join(stacksDir, stack.Name(), "layers")
		layers, err := os.ReadDir(layersDir)
		if err != nil {
			continue
		}

		hasLayers := false
		for _, layer := range layers {
			if !layer.IsDir() {
				continue
			}
			hasLayers = true

			deps := []string{
				fmt.Sprintf("stacks/%s/base/**", stack.Name()),
				fmt.Sprintf("stacks/%s/*.tfvars.jinja", stack.Name()),
				"stacks/*.tf",
				"stacks/*.tfvars.jinja",
				fmt.Sprintf("environments/%s/**", envFromLayerName(layer.Name())),
			}

			projects = append(projects, Project{
				Name:            fmt.Sprintf("stacks/%s/layer/%s", stack.Name(), layer.Name()),
				Path:            fmt.Sprintf("stacks/%s/layers/%s", stack.Name(), layer.Name()),
				Type:            ProjectTypeCiscoStacks,
				DependencyPaths: deps,
				Metadata: map[string]string{
					"cisco_stacks_stack": stack.Name(),
					"cisco_stacks_layer": layer.Name(),
				},
			})
		}

		if hasLayers {
			stackNames[stack.Name()] = true
		}
	}

	return projects, stackNames
}

// envFromLayerName extracts the environment from a Cisco Stacks layer name.
// Layer names follow the format: env[@subenv][_instance]
func envFromLayerName(name string) string {
	env := name
	if idx := strings.IndexByte(env, '_'); idx >= 0 {
		env = env[:idx]
	}
	if idx := strings.IndexByte(env, '@'); idx >= 0 {
		env = env[:idx]
	}
	return env
}
