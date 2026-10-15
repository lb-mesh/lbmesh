package recipe

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
)

//go:embed default_catalog/*
var defaultCatalog embed.FS

// defaultCatalogDir is the embed root for the built-in recipe templates.
const defaultCatalogDir = "default_catalog"

// Source identifies where a resolved recipe originated.
type Source int

const (
	// SourceUnknown is returned when the origin could not be determined.
	SourceUnknown Source = iota
	// SourceLocalWorkspace is the project-local ./.lbmesh/recipe.yaml.
	SourceLocalWorkspace
	// SourceCustom is a user-defined template under ~/.lbmesh/custom/templates.
	SourceCustom
	// SourceTap is a recipe from a connected git tap under ~/.lbmesh/taps.
	SourceTap
	// SourceCatalog is a recipe from the cached upstream catalog under
	// ~/.lbmesh/catalog.
	SourceCatalog
	// SourceEmbedded is a Go-embedded default template.
	SourceEmbedded
)

// String returns a human-readable label for the source.
func (s Source) String() string {
	switch s {
	case SourceLocalWorkspace:
		return "local workspace"
	case SourceCustom:
		return "custom template"
	case SourceTap:
		return "git tap"
	case SourceCatalog:
		return "cached catalog"
	case SourceEmbedded:
		return "embedded default"
	default:
		return "unknown"
	}
}

// ResolvedRecipe is a loaded recipe together with the source it came from and
// the path it was read from.
type ResolvedRecipe struct {
	Recipe Recipe
	Source Source
	Path   string
}

// Resolve searches the cascading hierarchy for a recipe named `name`, returning
// the first match found. Priority order:
//
//  1. Local workspace: ./.lbmesh/recipe.yaml
//  2. User custom: ~/.lbmesh/custom/templates/<name>/recipe.v1.yaml
//  3. Git taps: ~/.lbmesh/taps/<org>/templates/<name>/recipe.v1.yaml
//  4. Cached catalog: ~/.lbmesh/catalog/templates/<category>/<name>/recipe.v1.yaml
//  5. Embedded defaults: default_catalog/<name>/recipe.v1.yaml
func Resolve(name string) (*ResolvedRecipe, error) {
	if name == "" {
		return nil, fmt.Errorf("recipe name must not be empty")
	}

	// 1. Local workspace project recipe.
	if path := filepath.Join(".lbmesh", "recipe.yaml"); fileExists(path) {
		rec, err := LoadFile(path)
		if err == nil {
			return &ResolvedRecipe{Recipe: *rec, Source: SourceLocalWorkspace, Path: path}, nil
		}
	}

	root, err := dirs.LocalRoot()
	if err != nil {
		return nil, fmt.Errorf("resolving lbmesh home directory: %w", err)
	}

	// 2. User custom template.
	if path := filepath.Join(root, "custom", "templates", name, "recipe.v1.yaml"); fileExists(path) {
		rec, err := LoadFile(path)
		if err == nil {
			return &ResolvedRecipe{Recipe: *rec, Source: SourceCustom, Path: path}, nil
		}
	}

	// 3. Connected git taps: ~/.lbmesh/taps/<org>/templates/<name>/recipe.v1.yaml
	if path, err := firstMatch(filepath.Join(root, "taps", "*", "templates", name, "recipe.v1.yaml")); err == nil {
		rec, err2 := LoadFile(path)
		if err2 == nil {
			return &ResolvedRecipe{Recipe: *rec, Source: SourceTap, Path: path}, nil
		}
		_ = err
	}

	// 4. Cached upstream catalog: ~/.lbmesh/catalog/templates/<category>/<name>/recipe.v1.yaml
	if path, err := firstMatch(filepath.Join(root, "catalog", "templates", "*", name, "recipe.v1.yaml")); err == nil {
		rec, err2 := LoadFile(path)
		if err2 == nil {
			return &ResolvedRecipe{Recipe: *rec, Source: SourceCatalog, Path: path}, nil
		}
		_ = err
	}

	// 5. Go-embedded default catalog.
	embedPath := filepath.ToSlash(filepath.Join(defaultCatalogDir, name, "recipe.v1.yaml"))
	data, err := defaultCatalog.ReadFile(embedPath)
	if err == nil {
		var r Recipe
		if err := yaml.Unmarshal(data, &r); err == nil {
			if err := r.Validate(); err == nil {
				return &ResolvedRecipe{Recipe: r, Source: SourceEmbedded, Path: embedPath}, nil
			}
		}
	}

	return nil, fmt.Errorf("recipe %q not found in any source (workspace, custom, taps, catalog, embedded)", name)
}

// firstMatch returns the first existing file matching the glob pattern, or an
// error when none matches.
func firstMatch(pattern string) (string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	for _, m := range matches {
		if fileExists(m) {
			return m, nil
		}
	}
	return "", fmt.Errorf("no match for pattern %q", pattern)
}

// fileExists returns true when path points to a readable file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// ---------------------------------------------------------------------------
// Catalog listing helpers
// ---------------------------------------------------------------------------

// EmbeddedNames returns the sorted list of service names available in the
// Go-embedded default catalog.
func EmbeddedNames() ([]string, error) {
	entries, err := defaultCatalog.ReadDir(defaultCatalogDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sortStrings(names)
	return names, nil
}

// sortStrings sorts a string slice in place.
func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
