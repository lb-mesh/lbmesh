package ai

import (
	"strings"
	"testing"
)

func TestDiscoverAllRecipes(t *testing.T) {
	recipes, err := DiscoverAllRecipes()
	if err != nil {
		t.Fatalf("unexpected error discovering recipes: %v", err)
	}

	if len(recipes) == 0 {
		t.Fatal("expected at least embedded recipes to be discovered")
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

func TestSummarizeCatalog(t *testing.T) {
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
