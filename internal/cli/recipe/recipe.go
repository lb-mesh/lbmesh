// Package recipe provides the `lbmesh recipe` sub-command group.
package recipe

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
	"github.com/lbmesh/lbmesh-v2/pkg/recipe"
)

// NewRecipeCmd returns the parent `recipe` command with its sub-commands attached.
func NewRecipeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipe",
		Short: "Manage local recipe definitions",
		Long:  `recipe groups operations for scaffolding and validating local recipe.v1.yaml files.`,
	}

	cmd.AddCommand(newInitCmd(), newLintCmd())
	return cmd
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <name>",
		Short: "Scaffold a starter recipe.v1.yaml template",
		Long: `init writes a well-commented recipe.v1.yaml starter file into
~/.lbmesh/custom/templates/<name>/ so you can customise it for your service.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			root, err := dirs.LocalRoot()
			if err != nil {
				return fmt.Errorf("resolving lbmesh home directory: %w", err)
			}

			targetDir := filepath.Join(root, "custom", "templates", name)
			if err := os.MkdirAll(targetDir, 0o755); err != nil {
				return fmt.Errorf("creating template directory %s: %w", targetDir, err)
			}

			targetFile := filepath.Join(targetDir, "recipe.v1.yaml")
			if _, err := os.Stat(targetFile); err == nil {
				return fmt.Errorf("recipe template already exists at %s", targetFile)
			}

			if err := os.WriteFile(targetFile, []byte(starterTemplate(name)), 0o644); err != nil {
				return fmt.Errorf("writing starter recipe %s: %w", targetFile, err)
			}

			fmt.Printf("Created starter recipe template at %s\n", targetFile)
			return nil
		},
	}
}

func newLintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lint <file>",
		Short: "Validate a recipe file against schema constraints",
		Long: `lint parses the given recipe.v1.yaml and checks it against the expected
structural schema, reporting any missing required fields or type mismatches.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file := args[0]
			if _, err := recipe.LoadFile(file); err != nil {
				return fmt.Errorf("lint failed for %s: %w", file, err)
			}
			fmt.Printf("lint: %q is valid\n", file)
			return nil
		},
	}
}

// starterTemplate returns a documented recipe.v1.yaml scaffold for the given
// service name.
func starterTemplate(name string) string {
	return fmt.Sprintf(`# recipe.v1.yaml — scaffolded starter template for %s
# Customise the fields below and deploy with `+"`lbmesh up`"+`.
recipe: v1
metadata:
  name: %s
  category: database
  description: A short description of your %s service.
  scope: project
  version: "1.0.0"
resources:
  min_memory_mb: 256
  recommended_memory_mb: 512
  min_cpus: 1
container:
  image: your-image:tag
  environment:
    KEY: value
  ports:
    - container: 8080
      host: 8080
      protocol: tcp
  volumes:
    - /data
outputs:
  env:
    SERVICE_URL: http://localhost:8080
  instructions: Your %s service is running on localhost:8080.
`, name, name, name, name)
}
