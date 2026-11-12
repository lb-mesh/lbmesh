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

// Help group IDs for the root command.
const (
	groupCore    = "core"
	groupRecipe  = "recipe"
	groupTools   = "tools"
	groupUtility = "utility"
)

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
		// A runnable root ensures PersistentPreRunE (and thus the banner) fires
		// when lbmesh is invoked with no sub-command.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
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

	// Replaced by newCompletionCmd, which installs the script instead of
	// only printing it.
	root.CompletionOptions.DisableDefaultCmd = true

	// Keep commands in the order they are registered rather than alphabetical.
	cobra.EnableCommandSorting = false

	root.AddGroup(
		&cobra.Group{ID: groupCore, Title: "Core Commands:"},
		&cobra.Group{ID: groupRecipe, Title: "Recipe Commands:"},
		&cobra.Group{ID: groupTools, Title: "Interface & Tooling Commands:"},
		&cobra.Group{ID: groupUtility, Title: "Utility Commands:"},
	)
	root.SetHelpCommandGroupID(groupUtility)

	// withGroup assigns a help group to a command and returns it.
	withGroup := func(c *cobra.Command, id string) *cobra.Command {
		c.GroupID = id
		return c
	}

	// Ungrouped commands (version) are listed last under "Additional Commands:".
	root.AddCommand(
		withGroup(newInitCmd(), groupCore),
		withGroup(newStatusCmd(), groupCore),
		withGroup(catalog.NewCatalogCmd(), groupCore),
		withGroup(tap.NewTapCmd(), groupCore),

		withGroup(recipe.NewRecipeCmd(), groupRecipe),
		withGroup(newUpCmd(), groupRecipe),
		withGroup(newDownCmd(), groupRecipe),

		withGroup(newDashboardCmd(), groupTools),
		withGroup(newAskCmd(), groupTools),
		withGroup(NewMcpCmd(), groupTools),

		withGroup(newCompletionCmd(), groupUtility),

		newVersionCmd(),
	)

	return root
}
