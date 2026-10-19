package recipe

import (
	"fmt"
	"os"
	"path/filepath"
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

	res, err = Resolve("redis")
	if err != nil {
		t.Fatalf("Resolve(redis): %v", err)
	}
	if res.Source != SourceEmbedded || res.Recipe.Metadata.Name != "redis" {
		t.Fatalf("Resolve(redis) = %s from %v; want embedded redis", res.Recipe.Metadata.Name, res.Source)
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

// Resolve silently skips embedded recipes that fail validation, so check every
// built-in template explicitly.
func TestEmbeddedRecipesResolve(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	names, err := EmbeddedNames()
	if err != nil {
		t.Fatalf("EmbeddedNames: %v", err)
	}
	for _, name := range names {
		res, err := Resolve(name)
		if err != nil {
			t.Errorf("Resolve(%s): %v", name, err)
			continue
		}
		if res.Source != SourceEmbedded || res.Recipe.Metadata.Name != name {
			t.Errorf("Resolve(%s) = %s from %v; want embedded %s", name, res.Recipe.Metadata.Name, res.Source, name)
		}
	}
}
