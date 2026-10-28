package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
)

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
//  1. Local workspace: ./.lbmesh/recipe.yaml (only when its metadata.name is name)
//  2. User custom: ~/.lbmesh/custom/templates/<name>/recipe.v1.yaml
//  3. Git taps: ~/.lbmesh/taps/<host>/<owner>/<repo>/templates/<name>/recipe.v1.yaml
//  4. Cached catalog: ~/.lbmesh/catalog/templates/<category>/<name>/recipe.v1.yaml
//
// No recipes ship inside the binary; run `lbmesh catalog sync` to populate the
// cached catalog.
func Resolve(name string) (*ResolvedRecipe, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}

	// 1. Local workspace project recipe.
	if path := filepath.Join(".lbmesh", "recipe.yaml"); fileExists(path) {
		rec, err := LoadFile(path)
		if err == nil && rec.Metadata.Name == name {
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

	// loadErr remembers the first recipe file that was found but could not be
	// loaded, so the final error explains why instead of claiming "not found".
	var loadErr error

	// 3. Connected git taps, laid out by TapAdd as
	//    ~/.lbmesh/taps/<host>/<owner>/<repo>/templates/<name>/recipe.v1.yaml
	if path, err := firstMatch(filepath.Join(root, "taps", "*", "*", "*", "templates", name, "recipe.v1.yaml")); err == nil {
		rec, err2 := LoadFile(path)
		if err2 == nil {
			return &ResolvedRecipe{Recipe: *rec, Source: SourceTap, Path: path}, nil
		}
		loadErr = err2
	}

	// 4. Cached upstream catalog: ~/.lbmesh/catalog/templates/<category>/<name>/recipe.v1.yaml
	if path, err := firstMatch(filepath.Join(root, "catalog", "templates", "*", name, "recipe.v1.yaml")); err == nil {
		rec, err2 := LoadFile(path)
		if err2 == nil {
			return &ResolvedRecipe{Recipe: *rec, Source: SourceCatalog, Path: path}, nil
		}
		if loadErr == nil {
			loadErr = err2
		}
	}

	if loadErr != nil {
		return nil, fmt.Errorf("recipe %q was found but is invalid: %w", name, loadErr)
	}
	return nil, fmt.Errorf("recipe %q not found in any source (workspace, custom, taps, catalog); run `lbmesh catalog sync` to fetch the upstream catalog", name)
}

// ValidateName reports whether name can be used as a recipe (or catalog
// category) directory name: non-empty, a single path element, and free of glob
// metacharacters. Names reach the filesystem from CLI arguments, LLM plans and
// the downloaded catalog index, so "../" and friends must be rejected.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("recipe name must not be empty")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\*?[]`) {
		return fmt.Errorf("invalid recipe name %q", name)
	}
	return nil
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
