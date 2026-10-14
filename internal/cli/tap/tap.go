// Package tap provides the `lbmesh tap` sub-command group.
package tap

import (
	"fmt"

	"github.com/spf13/cobra"
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
			// TODO(sub-task-7): git clone --depth 1 into ~/.lbmesh/taps/<org>/<repo>.
			fmt.Printf("tap add: url=%q\n", args[0])
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List currently connected git recipe taps",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(sub-task-7): enumerate ~/.lbmesh/taps/ directories.
			fmt.Println("tap list: no taps registered.")
			return nil
		},
	}
}

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Run git pull on all connected tap repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(sub-task-7): iterate taps and run git pull.
			fmt.Println("tap sync: no taps to sync.")
			return nil
		},
	}
}
