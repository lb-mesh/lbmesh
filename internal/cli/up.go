package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	var scope string
	var autoResize bool
	var envVars []string
	var addons []string

	cmd := &cobra.Command{
		Use:   "up [recipe]",
		Short: "Deploy a recipe",
		Long: `up deploys the named recipe, creating the appropriate scoped bridge
network, allocating host ports, and starting all required containers.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			recipeName := ""
			if len(args) > 0 {
				recipeName = args[0]
			}
			// TODO(sub-task-2+): resolve recipe, probe socket, deploy containers.
			fmt.Printf("up: recipe=%q scope=%s autoResize=%v env=%v with=%v\n",
				recipeName, scope, autoResize, envVars, addons)
			return nil
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "project",
		`execution scope: project | system | custom`)
	cmd.Flags().BoolVar(&autoResize, "auto-resize", false,
		"automatically reconfigure Podman machine VM memory/CPU when resources are insufficient")
	cmd.Flags().StringArrayVarP(&envVars, "env", "e", nil,
		"override environment variables (KEY=VALUE), repeatable")
	cmd.Flags().StringSliceVar(&addons, "with", nil,
		"also deploy optional recipe addons, e.g. --with admin for a database web UI")

	return cmd
}
