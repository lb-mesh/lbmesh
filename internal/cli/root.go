// Package cli wires together the root Cobra command and all sub-commands.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/internal/cli/catalog"
	"github.com/lbmesh/lbmesh-v2/internal/cli/recipe"
	"github.com/lbmesh/lbmesh-v2/internal/cli/tap"
	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
)

// NewRootCmd constructs and returns the root lbmesh Cobra command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "lbmesh",
		Short: "Local infrastructure & container orchestration CLI",
		Long: `lbmesh is a dependency-free, single-binary CLI that orchestrates local
containers via Docker or Podman, manages isolated scoped bridge networks,
and exposes an embedded React + Carbon UI dashboard.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Ensure ~/.lbmesh directory tree exists before every command.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return dirs.EnsureLocalDirs()
		},
	}

	root.AddCommand(
		newInitCmd(),
		newUpCmd(),
		newDownCmd(),
		newStatusCmd(),
		newDashboardCmd(),
		newAskCmd(),
		catalog.NewCatalogCmd(),
		tap.NewTapCmd(),
		recipe.NewRecipeCmd(),
	)

	return root
}
