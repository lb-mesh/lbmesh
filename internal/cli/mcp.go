package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/pkg/db"
	"github.com/lbmesh/lbmesh-v2/pkg/mcp"
)

// NewMcpCmd constructs and returns the 'lbmesh mcp' Cobra command hierarchy.
func NewMcpCmd() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol (MCP) integrations",
		Long:  `mcp exposes LBMesh tools and databases directly to AI agents via JSON-RPC.`,
	}

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the LBMesh MCP Server",
		Long:  `start launches the JSON-RPC stdin/stdout server loop for local AI client integrations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. Open and migrate the state database best-effort
			stateDB, err := db.OpenDefaultAndMigrate()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Opening SQLite database: %v\n", err)
			}
			if stateDB != nil {
				defer stateDB.Close()
			}

			// 2. Start the MCP server stdin/stdout loop
			if err := mcp.Start(stateDB); err != nil {
				return fmt.Errorf("running MCP server: %w", err)
			}
			return nil
		},
	}

	mcpCmd.AddCommand(startCmd)
	return mcpCmd
}
