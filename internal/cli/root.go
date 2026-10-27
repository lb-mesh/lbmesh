// Package cli wires together the root Cobra command and all sub-commands.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/internal/cli/catalog"
	"github.com/lbmesh/lbmesh-v2/internal/cli/recipe"
	"github.com/lbmesh/lbmesh-v2/internal/cli/tap"
	"github.com/lbmesh/lbmesh-v2/internal/version"
	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
)

// banner is the LB Mesh dot-matrix logo rendered in bright green.
const (
	ansiGreen = "\033[92m"
	ansiReset = "\033[0m"
)

const bannerText = `
 _     ______  ___  ___          _
| |    | ___ \ |  \/  |         | |
| |    | |_/ / | .  . | ___  ___| |__
| |    | ___ \ | |\/| |/ _ \/ __| '_ \
| |____| |_/ / | |  | |  __/\__ \ | | |
\_____/\____/  \_|  |_/\___||___/_| |_|
`

func printBanner() {
	// Skip the banner when stdout is not a terminal (e.g. pipes, CI).
	if fi, err := os.Stdout.Stat(); err == nil {
		if fi.Mode()&os.ModeCharDevice == 0 {
			return
		}
	}
	fmt.Print(ansiGreen + bannerText + ansiReset + "\n")
}

// NewRootCmd constructs and returns the root lbmesh Cobra command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "lbmesh",
		Version: version.Get().Version,
		Short:   "Local infrastructure & container orchestration CLI",
		Long: `lbmesh is a dependency-free, single-binary CLI that orchestrates local
containers via Docker or Podman, manages isolated scoped bridge networks,
and exposes an embedded React + Carbon UI dashboard.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Print the banner then ensure ~/.lbmesh dir tree and state DB migrations
		// run before every command.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			printBanner()
			if err := dirs.EnsureLocalDirs(); err != nil {
				return err
			}
			database, err := db.OpenDefaultAndMigrate()
			if err != nil {
				return fmt.Errorf("initializing state database: %w", err)
			}
			return database.Close()
		},
	}

	// Keep `lbmesh --version` consistent with `lbmesh version --short`.
	root.SetVersionTemplate("lbmesh {{.Version}}\n")

	root.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newUpCmd(),
		newDownCmd(),
		newStatusCmd(),
		newDashboardCmd(),
		newAskCmd(),
		NewMcpCmd(),
		catalog.NewCatalogCmd(),
		tap.NewTapCmd(),
		recipe.NewRecipeCmd(),
	)

	return root
}
