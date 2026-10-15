// Package tap provides the `lbmesh tap` sub-command group.
package tap

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/pkg/recipe"
)

// NewTapCmd returns the parent `tap` command with its sub-commands attached.
func NewTapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tap",
		Short: "Manage git-based recipe taps",
		Long:  `tap groups operations for registering, listing, and updating private or team git recipe repositories.`,
	}

	cmd.AddCommand(newAddCmd(), newListCmd(), newSyncCmd())
	return cmd
}

func newAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <git-url>",
		Short: "Register a team or private git repository as a recipe tap",
		Long: `add clones the target git repository into ~/.lbmesh/taps/ using the
system-installed git binary so that your existing SSH/HTTPS credential
helpers are respected automatically.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			gitURL := args[0]

			tapDir, err := recipe.TapAdd(gitURL)
			if err != nil {
				return err
			}

			fmt.Printf("✓ Tap registered: %s\n", tapDir)
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List currently connected git recipe taps",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := recipe.TapList()
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Println("No taps registered. Use `lbmesh tap add <git-url>` to add one.")
				return nil
			}

			fmt.Printf("%-50s  %s\n", "SLUG", "PATH")
			fmt.Printf("%-50s  %s\n", "----", "----")
			for _, e := range entries {
				fmt.Printf("%-50s  %s\n", e.Slug, e.Path)
			}
			return nil
		},
	}
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Run git pull on all connected tap repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := recipe.TapList()
			if err != nil {
				return err
			}

			if len(entries) == 0 {
				fmt.Println("No taps registered. Use `lbmesh tap add <git-url>` to add one.")
				return nil
			}

			fmt.Printf("Syncing %d tap(s)…\n", len(entries))
			for _, e := range entries {
				fmt.Printf("  pulling %s … ", e.Slug)
				if err := recipe.TapSyncOne(e.Path); err != nil {
					fmt.Printf("FAILED: %v\n", err)
				} else {
					fmt.Println("ok")
				}
			}
			return nil
		},
	}
}
