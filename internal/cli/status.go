package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Display all running instances, scopes, container IDs, and port mappings",
		Long: `status queries the local state database and the active container engine
to print a live ASCII table of running projects, their scopes, container IDs,
allocated host ports, and basic resource metrics.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(sub-task-3+): read state.db + query container engine for live metrics.
			fmt.Println("status: no running instances found.")
			return nil
		},
	}
}
