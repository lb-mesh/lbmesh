package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPostgresRecipe = `recipe: v1
metadata:
  name: postgresql
  category: database
  description: PostgreSQL database.
  scope: system
  version: "1.0.0"
container:
  image: postgres:17
  environment:
    POSTGRES_DB: app
`

// seedCatalog points HOME at a temp dir holding a synced catalog with a
// postgresql recipe, since no recipes ship inside the binary.
func seedCatalog(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())

	dir := filepath.Join(home, ".lbmesh", "catalog", "templates", "database", "postgresql")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "recipe.v1.yaml"), []byte(testPostgresRecipe), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverAllRecipes(t *testing.T) {
	seedCatalog(t)

	recipes, err := DiscoverAllRecipes()
	if err != nil {
		t.Fatalf("unexpected error discovering recipes: %v", err)
	}

	foundPG := false
	for _, r := range recipes {
		if r.Recipe.Metadata.Name == "postgresql" {
			foundPG = true
			break
		}
	}

	if !foundPG {
		t.Error("expected to find postgresql recipe")
	}
}

func TestDiscoverAllRecipes_EmptyBeforeSync(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	recipes, err := DiscoverAllRecipes()
	if err != nil {
		t.Fatalf("unexpected error discovering recipes: %v", err)
	}
	if len(recipes) != 0 {
		t.Errorf("expected no recipes before catalog sync, got %d", len(recipes))
	}
}

func TestSummarizeCatalog(t *testing.T) {
	seedCatalog(t)

	summary, err := SummarizeCatalog()
	if err != nil {
		t.Fatalf("unexpected error summarizing catalog: %v", err)
	}

	if !strings.Contains(summary, "Recipe: postgresql") {
		t.Errorf("summary missing postgresql: %s", summary)
	}

	if !strings.Contains(summary, "POSTGRES_DB") {
		t.Errorf("summary missing postgresql env vars: %s", summary)
	}

	prompt := BuildSystemPrompt(summary)
	if !strings.Contains(prompt, "LBMesh AI Orchestrator") {
		t.Errorf("system prompt missing orchestrator header: %s", prompt)
	}
	if !strings.Contains(prompt, "Recipe: postgresql") {
		t.Errorf("system prompt missing catalog: %s", prompt)
	}
}
