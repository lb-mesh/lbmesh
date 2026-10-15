package recipe

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
)

// DefaultCatalogIndexURL is the upstream CDN endpoint for the official lbmesh
// recipe index.  It can be overridden at build time or via configuration.
const DefaultCatalogIndexURL = "https://raw.githubusercontent.com/lbmesh/catalog/main/index.json"

// CatalogEntry is a single record in the upstream catalog index JSON.
type CatalogEntry struct {
	// Name is the short service identifier (e.g. "postgresql").
	Name string `json:"name"`
	// Category classifies the service.
	Category string `json:"category"`
	// Description is a human-readable summary.
	Description string `json:"description"`
	// RecipeURL is the raw URL of the recipe.v1.yaml file.
	RecipeURL string `json:"recipe_url"`
	// Version is the recipe template version string.
	Version string `json:"version"`
}

// CatalogIndex is the root structure of the upstream index.json.
type CatalogIndex struct {
	// SchemaVersion identifies the index format version.
	SchemaVersion string         `json:"schema_version"`
	// Entries lists all available recipes in the upstream catalog.
	Entries       []CatalogEntry `json:"entries"`
}

// httpClient is the shared HTTP client used for catalog downloads.  A
// reasonable timeout prevents the CLI from hanging indefinitely on slow
// or unreachable upstream endpoints.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// CatalogSync downloads the upstream recipe index JSON from indexURL and
// persists it to ~/.lbmesh/catalog/index.json.  It also fetches each
// referenced recipe.v1.yaml and stores it under
// ~/.lbmesh/catalog/templates/<category>/<name>/recipe.v1.yaml so that
// the cascading resolver can find them offline after the first sync.
//
// Progress is reported through the optional progressFn callback; pass nil to
// suppress all progress output.
func CatalogSync(indexURL string, progressFn func(msg string)) error {
	if indexURL == "" {
		indexURL = DefaultCatalogIndexURL
	}
	if progressFn == nil {
		progressFn = func(string) {}
	}

	root, err := dirs.LocalRoot()
	if err != nil {
		return err
	}
	catalogDir := filepath.Join(root, "catalog")

	// ------------------------------------------------------------------
	// 1. Download and persist the index JSON.
	// ------------------------------------------------------------------
	progressFn(fmt.Sprintf("Fetching catalog index from %s …", indexURL))

	indexData, err := downloadBytes(indexURL)
	if err != nil {
		return fmt.Errorf("downloading catalog index: %w", err)
	}

	var idx CatalogIndex
	if err := json.Unmarshal(indexData, &idx); err != nil {
		return fmt.Errorf("parsing catalog index JSON: %w", err)
	}

	indexPath := filepath.Join(catalogDir, "index.json")
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		return fmt.Errorf("creating catalog directory: %w", err)
	}
	if err := os.WriteFile(indexPath, indexData, 0o644); err != nil {
		return fmt.Errorf("writing catalog index to %s: %w", indexPath, err)
	}
	progressFn(fmt.Sprintf("Catalog index saved (%d entries).", len(idx.Entries)))

	// ------------------------------------------------------------------
	// 2. Fetch each recipe YAML and store it in the template tree.
	// ------------------------------------------------------------------
	var fetchErrs []string
	for _, entry := range idx.Entries {
		if entry.RecipeURL == "" {
			continue
		}

		destDir := filepath.Join(catalogDir, "templates", entry.Category, entry.Name)
		destPath := filepath.Join(destDir, "recipe.v1.yaml")

		progressFn(fmt.Sprintf("  ↓ %s/%s", entry.Category, entry.Name))

		data, err := downloadBytes(entry.RecipeURL)
		if err != nil {
			fetchErrs = append(fetchErrs, fmt.Sprintf("    %s/%s: %v", entry.Category, entry.Name, err))
			continue
		}

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			fetchErrs = append(fetchErrs, fmt.Sprintf("    %s/%s: creating dir: %v", entry.Category, entry.Name, err))
			continue
		}

		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			fetchErrs = append(fetchErrs, fmt.Sprintf("    %s/%s: writing file: %v", entry.Category, entry.Name, err))
			continue
		}
	}

	if len(fetchErrs) > 0 {
		// Surface fetch failures but do not abort — a partial sync is better
		// than no sync.
		progressFn("Warning: some recipe files could not be fetched:")
		for _, e := range fetchErrs {
			progressFn(e)
		}
	}

	progressFn("Catalog sync complete.")
	return nil
}

// LoadCatalogIndex reads the locally cached catalog index from
// ~/.lbmesh/catalog/index.json and returns the parsed structure.
// Returns an error when no sync has been performed yet.
func LoadCatalogIndex() (*CatalogIndex, error) {
	root, err := dirs.LocalRoot()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(root, "catalog", "index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no local catalog index found — run `lbmesh catalog sync` first")
		}
		return nil, fmt.Errorf("reading catalog index: %w", err)
	}

	var idx CatalogIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing catalog index: %w", err)
	}

	return &idx, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// downloadBytes performs an HTTP GET to url and returns the response body.
func downloadBytes(url string) ([]byte, error) {
	resp, err := httpClient.Get(url) //nolint:noctx // CLI tool; context not plumbed here.
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body from %s: %w", url, err)
	}

	return data, nil
}
