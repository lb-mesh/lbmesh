// Package catalog provides the `lbmesh catalog` sub-command group.
package catalog

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCatalogCmd returns the parent `catalog` command with its sub-commands attached.
func NewCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Manage recipe catalogs",
		Long:  `catalog groups operations for browsing and synchronising recipe template catalogs.`,
	}

	cmd.AddCommand(newListCmd(), newSyncCmd())
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Display all discovered local, custom, tap, and upstream templates",
		Long: `list scans every configured catalog source — the local custom directory,
connected git taps, the cached upstream catalog, and the embedded default
catalog — and prints a consolidated table of available recipes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(sub-task-6+7): walk resolver paths and print recipe metadata table.
			fmt.Println("catalog list: no recipes found.")
			return nil
		},
	}
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Synchronise official upstream JSON indices to local disk",
		Long: `sync downloads the standardised upstream recipe catalog JSON index from
the CDN endpoint and stores it under ~/.lbmesh/catalog/.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(sub-task-7): download upstream JSON catalog index.
			fmt.Println("catalog sync: not yet implemented.")
			return nil
		},
	}
}
