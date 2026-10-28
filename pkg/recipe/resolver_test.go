package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRecipe = `recipe: v1
metadata:
  name: %s
  category: cache
  description: test recipe
  scope: project
  version: "9.9.9"
container:
  image: example/%s:latest
`

func writeRecipe(t *testing.T, path, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(testRecipe, name, name))
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolve_FindsTapRecipe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())

	// TapAdd clones into taps/<host>/<owner>/<repo>.
	writeRecipe(t, filepath.Join(home, ".lbmesh", "taps", "github.com", "acme", "recipes",
		"templates", "kafka", "recipe.v1.yaml"), "kafka")

	res, err := Resolve("kafka")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Source != SourceTap {
		t.Fatalf("Source = %v, want %v", res.Source, SourceTap)
	}
}

func TestResolve_WorkspaceRecipeOnlyMatchesItsName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	writeRecipe(t, filepath.Join(".lbmesh", "recipe.yaml"), "myapp")

	res, err := Resolve("myapp")
	if err != nil || res.Source != SourceLocalWorkspace {
		t.Fatalf("Resolve(myapp) = %+v, %v; want local workspace", res, err)
	}

	// Nothing ships in the binary, so any other name is unknown until synced.
	if _, err = Resolve("redis"); err == nil || !strings.Contains(err.Error(), "catalog sync") {
		t.Fatalf("Resolve(redis) error = %v; want not-found hint to run catalog sync", err)
	}
}

func TestResolve_FindsCatalogRecipe(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	writeRecipe(t, filepath.Join(home, ".lbmesh", "catalog", "templates", "cache", "redis", "recipe.v1.yaml"), "redis")

	res, err := Resolve("redis")
	if err != nil || res.Source != SourceCatalog {
		t.Fatalf("Resolve(redis) = %+v, %v; want cached catalog", res, err)
	}
}

func TestResolve_InvalidCatalogRecipeReportsReason(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	path := filepath.Join(home, ".lbmesh", "catalog", "templates", "gateway", "portkey", "recipe.v1.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("apiVersion: catalog/v1\nkind: Recipe\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Resolve("portkey")
	if err == nil || !strings.Contains(err.Error(), "invalid") || !strings.Contains(err.Error(), "unsupported recipe version") {
		t.Fatalf("Resolve(portkey) error = %v; want invalid-recipe reason", err)
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"redis", "postgresql", "my-app_2"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "../etc", "a/b", `a\b`, "red*", "x?"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", bad)
		}
	}
}
