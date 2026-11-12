package recipe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogClean_RemovesCatalogOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())

	catalogRecipe := filepath.Join(home, ".lbmesh", "catalog", "templates", "cache", "redis", "recipe.v1.yaml")
	customRecipe := filepath.Join(home, ".lbmesh", "custom", "templates", "mine", "recipe.v1.yaml")
	writeRecipe(t, catalogRecipe, "redis")
	writeRecipe(t, customRecipe, "mine")

	path, removed, err := CatalogClean()
	if err != nil {
		t.Fatalf("CatalogClean: %v", err)
	}
	if !removed {
		t.Fatal("removed = false, want true")
	}
	if want := filepath.Join(home, ".lbmesh", "catalog"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("catalog dir still exists (stat err = %v)", err)
	}
	if _, err := os.Stat(customRecipe); err != nil {
		t.Fatalf("custom recipe should be untouched: %v", err)
	}
	if _, err := Resolve("redis"); err == nil {
		t.Fatal("Resolve(redis) succeeded after clean, want not found")
	}
}

func TestCatalogClean_NothingToRemove(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, removed, err := CatalogClean()
	if err != nil {
		t.Fatalf("CatalogClean: %v", err)
	}
	if removed {
		t.Fatal("removed = true, want false when no catalog exists")
	}
}
