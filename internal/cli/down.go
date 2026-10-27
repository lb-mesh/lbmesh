package cli

import (
	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/pkg/teardown"
)

func newDownCmd() *cobra.Command {
	var scope string

	cmd := &cobra.Command{
		Use:   "down <recipe>",
		Short: "Tear down a running recipe",
		Long: `down stops and removes all containers associated with the named recipe,
cleans up custom network routes, and releases active port mappings.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return teardown.Run(teardown.Options{
				RecipeName: args[0],
				Scope:      scope,
			})
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "",
		"narrow teardown to a specific scope: project | system | custom (default: all matching)")

	return cmd
}
