package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/pkg/config"
	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Bootstrap local directory structure and create default config.yaml",
		Long: `init creates the ~/.lbmesh/ directory tree, writes a default config.yaml
if one does not already exist, and prepares the local state database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := dirs.EnsureLocalDirs(); err != nil {
				return fmt.Errorf("scaffolding local dirs: %w", err)
			}
			if err := config.EnsureDefaults(); err != nil {
				return fmt.Errorf("writing default config: %w", err)
			}
			database, err := db.OpenDefaultAndMigrate()
			if err != nil {
				return fmt.Errorf("initializing state database: %w", err)
			}
			_ = database.Close()
			fmt.Println("lbmesh initialised successfully.")
			fmt.Println("Next: run `lbmesh catalog sync` to download the recipe catalog.")
			return nil
		},
	}
}
