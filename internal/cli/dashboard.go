package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDashboardCmd() *cobra.Command {
	var port int
	var host string

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Launch the embedded React + Carbon UI dashboard",
		Long: `dashboard starts an embedded HTTP server and opens the React + Carbon Design
System dashboard in your default browser. The server streams live container
logs and metrics via Server-Sent Events.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(sub-task-10): start embedded HTTP/SSE server and serve web/dist.
			fmt.Printf("dashboard: listening on http://%s:%d\n", host, port)
			return nil
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "TCP port for the dashboard HTTP server")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "interface address to bind the dashboard server")

	return cmd
}
