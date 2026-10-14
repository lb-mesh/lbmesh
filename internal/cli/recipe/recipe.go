// Package recipe provides the `lbmesh recipe` sub-command group.
package recipe

import (
	"fmt"

	"github.com/spf13/cobra"
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
		Long: `init writes a well-commented recipe.v1.yaml starter file into the current
working directory so you can customise it for your service.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(sub-task-6): write starter recipe template to ./<name>/recipe.v1.yaml.
			fmt.Printf("recipe init: name=%q\n", args[0])
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
			// TODO(sub-task-6): unmarshal and validate recipe.v1.yaml.
			fmt.Printf("recipe lint: file=%q\n", args[0])
			return nil
		},
	}
}
