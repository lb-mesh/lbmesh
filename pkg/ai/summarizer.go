package ai

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
	"github.com/lbmesh/lbmesh-v2/pkg/recipe"
)

// RecipeSummary represents a concise summary of a recipe suitable for LLM context.
type RecipeSummary struct {
	Name        string
	Category    string
	Scope       string
	Description string
	Image       string
	Ports       []string
	Environment map[string]string
	Outputs     map[string]string
}

// DiscoverAllRecipes finds and loads all available unique recipes across all sources:
// workspace, user custom templates, taps, cached catalog, and embedded defaults.
func DiscoverAllRecipes() ([]recipe.ResolvedRecipe, error) {
	namesSet := make(map[string]struct{})

	// 1. Embedded recipe names
	if embedded, err := recipe.EmbeddedNames(); err == nil {
		for _, name := range embedded {
			namesSet[name] = struct{}{}
		}
	}

	// 2. Local workspace recipe
	wsPath := filepath.Join(".lbmesh", "recipe.yaml")
	if r, err := recipe.LoadFile(wsPath); err == nil && r.Metadata.Name != "" {
		namesSet[r.Metadata.Name] = struct{}{}
	}

	// 3. User custom templates & cached catalog & taps
	if root, err := dirs.LocalRoot(); err == nil {
		// Custom templates: ~/.lbmesh/custom/templates/<name>/
		customDir := filepath.Join(root, "custom", "templates")
		if entries, err := os.ReadDir(customDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					namesSet[e.Name()] = struct{}{}
				}
			}
		}

		// Cached catalog: ~/.lbmesh/catalog/templates/<cat>/<name>/
		catDir := filepath.Join(root, "catalog", "templates")
		if cats, err := os.ReadDir(catDir); err == nil {
			for _, cat := range cats {
				if cat.IsDir() {
					if subEntries, err := os.ReadDir(filepath.Join(catDir, cat.Name())); err == nil {
						for _, sub := range subEntries {
							if sub.IsDir() {
								namesSet[sub.Name()] = struct{}{}
							}
						}
					}
				}
			}
		}

		// Taps: ~/.lbmesh/taps/.../templates/<name>/
		if taps, err := recipe.TapList(); err == nil {
			for _, tap := range taps {
				tapTemplates := filepath.Join(tap.Path, "templates")
				if subEntries, err := os.ReadDir(tapTemplates); err == nil {
					for _, sub := range subEntries {
						if sub.IsDir() {
							namesSet[sub.Name()] = struct{}{}
						}
					}
				}
			}
		}
	}

	names := make([]string, 0, len(namesSet))
	for n := range namesSet {
		names = append(names, n)
	}
	sort.Strings(names)

	var resolved []recipe.ResolvedRecipe
	for _, n := range names {
		rec, err := recipe.Resolve(n)
		if err == nil && rec != nil {
			resolved = append(resolved, *rec)
		}
	}

	return resolved, nil
}

// SummarizeCatalog returns a human/LLM-readable text summary of all available recipes.
func SummarizeCatalog() (string, error) {
	recipes, err := DiscoverAllRecipes()
	if err != nil {
		return "", fmt.Errorf("discovering recipes: %w", err)
	}

	if len(recipes) == 0 {
		return "No recipes currently available in the catalog.", nil
	}

	var sb strings.Builder
	sb.WriteString("Available Recipe Catalog:\n\n")

	for _, res := range recipes {
		r := res.Recipe
		sb.WriteString(fmt.Sprintf("- Recipe: %s\n", r.Metadata.Name))
		if r.Metadata.Category != "" {
			sb.WriteString(fmt.Sprintf("  Category: %s\n", r.Metadata.Category))
		}
		if r.Metadata.Scope != "" {
			sb.WriteString(fmt.Sprintf("  Default Scope: %s\n", r.Metadata.Scope))
		}
		if r.Metadata.Description != "" {
			sb.WriteString(fmt.Sprintf("  Description: %s\n", r.Metadata.Description))
		}
		if r.Container.Image != "" {
			sb.WriteString(fmt.Sprintf("  Container Image: %s\n", r.Container.Image))
		}

		if len(r.Container.Ports) > 0 {
			var ports []string
			for _, p := range r.Container.Ports {
				proto := p.Protocol
				if proto == "" {
					proto = "tcp"
				}
				ports = append(ports, fmt.Sprintf("%d/%s (host: %d)", p.Container, proto, p.Host))
			}
			sb.WriteString(fmt.Sprintf("  Ports: %s\n", strings.Join(ports, ", ")))
		}

		if len(r.Container.Environment) > 0 {
			sb.WriteString("  Environment Variables:\n")
			var envKeys []string
			for k := range r.Container.Environment {
				envKeys = append(envKeys, k)
			}
			sort.Strings(envKeys)
			for _, k := range envKeys {
				val := r.Container.Environment[k]
				if val != "" {
					sb.WriteString(fmt.Sprintf("    * %s (default: %q)\n", k, val))
				} else {
					sb.WriteString(fmt.Sprintf("    * %s\n", k))
				}
			}
		}

		if len(r.Outputs.Env) > 0 {
			sb.WriteString("  Outputs:\n")
			var outKeys []string
			for k := range r.Outputs.Env {
				outKeys = append(outKeys, k)
			}
			sort.Strings(outKeys)
			for _, k := range outKeys {
				sb.WriteString(fmt.Sprintf("    * %s: %s\n", k, r.Outputs.Env[k]))
			}
		}

		if len(r.Addons) > 0 {
			sb.WriteString("  Optional Addons (enable with --with <name>):\n")
			for _, a := range r.Addons {
				sb.WriteString(fmt.Sprintf("    * %s: %s (image: %s)\n", a.Name, a.Description, a.Container.Image))
			}
		}

		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String()), nil
}

// BuildSystemPrompt creates the system prompt for the AI LLM orchestrator.
func BuildSystemPrompt(catalogSummary string) string {
	return fmt.Sprintf(`You are the LBMesh AI Orchestrator, an autonomous container and local infrastructure planning engine.
Your task is to analyze user requests and construct a minimal, conflict-free, structured execution plan using available recipes from the catalog.

%s

Guidelines:
1. Select only recipes from the available catalog that satisfy the user's intent.
2. If multiple services need to communicate (e.g. backend and database), configure appropriate environment variables (like DATABASE_URL, POSTGRES_DB, POSTGRES_PASSWORD, etc.).
3. Choose scope:
   - "project" (default): isolated project network for project-level services.
   - "system": global shared network for shared infrastructure.
   - "custom": custom isolated developer stacks.
4. Respond ONLY with a valid JSON object matching the following schema:
{
  "explanation": "<Short, 1-2 sentence explanation of the planned deployment>",
  "plan": [
    {
      "recipe": "<exact recipe name from catalog>",
      "scope": "project" | "system" | "custom",
      "environment": {
        "<ENV_VAR_KEY>": "<ENV_VAR_VALUE>"
      },
      "description": "<short reason for including this service>"
    }
  ]
}

DO NOT include any conversational markdown or text outside the JSON object. Output strict valid JSON.`, catalogSummary)
}
