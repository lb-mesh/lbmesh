package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/pkg/dashboard"
	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/socket"
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
			// Open the state database (best-effort; dashboard functions without it).
			sqlDB, dbErr := db.OpenDefaultAndMigrate()
			if dbErr != nil {
				fmt.Fprintf(os.Stderr, "warning: could not open state database: %v\n", dbErr)
			}
			if sqlDB != nil {
				defer sqlDB.Close()
			}

			// Probe for a running container engine (best-effort).
			dockerClient, _, engineErr := socket.Detect()
			if engineErr != nil {
				fmt.Fprintf(os.Stderr, "warning: no container engine detected — engine-dependent endpoints will return 503\n")
			}
			if dockerClient != nil {
				defer dockerClient.Close()
			}

			// Build and start the dashboard server.
			srv, err := dashboard.New(dashboard.Config{
				Host:         host,
				Port:         port,
				DB:           sqlDB,
				DockerClient: dockerClient,
			})
			if err != nil {
				return fmt.Errorf("creating dashboard server: %w", err)
			}

			// Create a context that cancels on SIGINT / SIGTERM.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			fmt.Printf("dashboard: listening on http://%s:%d\n", host, port)

			if err := srv.Start(ctx); err != nil {
				return fmt.Errorf("dashboard server: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&port, "port", 8080, "TCP port for the dashboard HTTP server")
	cmd.Flags().StringVar(&host, "host", "127.0.0.1", "interface address to bind the dashboard server")

	return cmd
}
