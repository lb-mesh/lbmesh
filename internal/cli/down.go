package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down [recipe]",
		Short: "Tear down a running recipe",
		Long: `down stops and removes all containers associated with the named recipe,
cleans up custom network routes, and releases active port mappings.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			recipeName := ""
			if len(args) > 0 {
				recipeName = args[0]
			}
			// TODO(sub-task-2+): locate running project, tear down containers, remove network.
			fmt.Printf("down: recipe=%q\n", recipeName)
			return nil
		},
	}
}
