package autodetect

import (
	"fmt"
	"os"
	"path/filepath"
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
			projects = append(projects, Project{
				Name: fmt.Sprintf("stacks/%s/layer/%s", stack.Name(), layer.Name()),
				Path: ".",
				Type: ProjectTypeCiscoStacks,
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
