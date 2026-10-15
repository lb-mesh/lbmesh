// Package recipe implements the lbmesh recipe.v1.yaml format: a typed,
// validated representation of a local service declaration plus a cascading
// resolver that locates recipe definitions across the local workspace, user
// custom templates, git taps, cached catalog, and Go-embedded defaults.
package recipe

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RecipeVersion is the supported recipe manifest schema version.
const RecipeVersion = "v1"

// ValidCategories is the set of recognized recipe metadata categories.
var ValidCategories = map[string]struct{}{
	"database":  {},
	"backend":   {},
	"auth":      {},
	"ai-agent":  {},
	"cache":     {},
	"webserver": {},
}

// ValidScopes is the set of recognized execution scopes.
var ValidScopes = map[string]struct{}{
	"system":  {},
	"project": {},
	"custom":  {},
}

// ---------------------------------------------------------------------------
// Top-level recipe model
// ---------------------------------------------------------------------------

// Recipe is the fully parsed representation of a recipe.v1.yaml file.
type Recipe struct {
	// RecipeVersion declares the manifest schema version ("v1").
	RecipeVersion string `yaml:"recipe"`

	// Metadata identifies the recipe (name, category, description, scope).
	Metadata Metadata `yaml:"metadata"`

	// Resources declares host reservations required to run the service.
	Resources Resources `yaml:"resources"`

	// Container describes the container image, environment, ports, and volumes.
	Container Container `yaml:"container"`

	// Outputs describes generated .env variables and post-start instructions.
	Outputs Outputs `yaml:"outputs,omitempty"`
}

// Metadata identifies a recipe.
type Metadata struct {
	// Name is the short service identifier (e.g. "postgresql").
	Name string `yaml:"name"`
	// Category classifies the service (database, backend, auth, ai-agent, …).
	Category string `yaml:"category"`
	// Description is a human-readable summary of the service.
	Description string `yaml:"description"`
	// Scope is the execution scope: system, project, or custom.
	Scope string `yaml:"scope"`
	// Version is the recipe template version.
	Version string `yaml:"version"`
}

// Resources declares host reservations required to run the service.
type Resources struct {
	// MinMemoryMB is the minimum memory reservation in MiB.
	MinMemoryMB int `yaml:"min_memory_mb"`
	// RecommendedMemoryMB is the suggested comfortable memory reservation in MiB.
	RecommendedMemoryMB int `yaml:"recommended_memory_mb"`
	// MinCPUs is the minimum CPU reservation.
	MinCPUs int `yaml:"min_cpus"`
}

// Container describes the container to run.
type Container struct {
	// Image is the container image reference.
	Image string `yaml:"image"`
	// Command is the entrypoint command override.
	Command []string `yaml:"command,omitempty"`
	// Args are additional arguments passed to the entrypoint.
	Args []string `yaml:"args,omitempty"`
	// Environment defines default environment variables.
	Environment map[string]string `yaml:"environment,omitempty"`
	// Ports maps container ports to host ports.
	Ports []PortBinding `yaml:"ports,omitempty"`
	// Volumes lists container volume mount paths.
	Volumes []string `yaml:"volumes,omitempty"`
}

// PortBinding maps a container port to a host port.
type PortBinding struct {
	// Container is the port inside the container.
	Container int `yaml:"container"`
	// Host is the port on the host.
	Host int `yaml:"host"`
	// Protocol is the network protocol (tcp, udp).
	Protocol string `yaml:"protocol"`
}

// Outputs describes generated .env variables and post-start instructions.
type Outputs struct {
	// Env is a set of environment variables to emit into the project .env.
	Env map[string]string `yaml:"env,omitempty"`
	// Instructions are post-start console instructions for the user.
	Instructions string `yaml:"instructions,omitempty"`
}

// ---------------------------------------------------------------------------
// Loading & validation
// ---------------------------------------------------------------------------

// LoadFile reads and unmarshals a recipe.v1.yaml file from path, then validates
// it against the structural schema.
func LoadFile(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading recipe file %s: %w", path, err)
	}

	var r Recipe
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing recipe file %s: %w", path, err)
	}

	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("validating recipe file %s: %w", path, err)
	}

	return &r, nil
}

// Validate checks the recipe against structural constraints and returns an
// error describing the first problem found, or nil when the recipe is well
// formed.
func (r *Recipe) Validate() error {
	if r.RecipeVersion != RecipeVersion {
		return fmt.Errorf("unsupported recipe version %q, expected %q", r.RecipeVersion, RecipeVersion)
	}

	if r.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if _, ok := ValidCategories[r.Metadata.Category]; !ok {
		return fmt.Errorf("metadata.category %q is not one of %v", r.Metadata.Category, sortedKeys(ValidCategories))
	}
	if _, ok := ValidScopes[r.Metadata.Scope]; !ok {
		return fmt.Errorf("metadata.scope %q is not one of %v", r.Metadata.Scope, sortedKeys(ValidScopes))
	}

	if r.Container.Image == "" {
		return fmt.Errorf("container.image is required")
	}

	if r.Resources.MinMemoryMB < 0 {
		return fmt.Errorf("resources.min_memory_mb must not be negative")
	}
	if r.Resources.RecommendedMemoryMB < 0 {
		return fmt.Errorf("resources.recommended_memory_mb must not be negative")
	}
	if r.Resources.MinCPUs < 0 {
		return fmt.Errorf("resources.min_cpus must not be negative")
	}

	for i, p := range r.Container.Ports {
		if p.Container <= 0 {
			return fmt.Errorf("container.ports[%d].container must be a positive port number", i)
		}
		if p.Host <= 0 {
			return fmt.Errorf("container.ports[%d].host must be a positive port number", i)
		}
		if p.Protocol != "" && p.Protocol != "tcp" && p.Protocol != "udp" {
			return fmt.Errorf("container.ports[%d].protocol must be tcp or udp", i)
		}
	}

	return nil
}

// sortedKeys returns the keys of a map in sorted order for stable error output.
func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Insertion-order stability is not guaranteed; sort for deterministic output.
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
