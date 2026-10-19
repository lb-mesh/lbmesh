package mcp

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
	"github.com/lbmesh/lbmesh-v2/pkg/recipe"
)

// JSONRPCRequest represents a standard Model Context Protocol JSON-RPC request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a standard Model Context Protocol JSON-RPC response.
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// JSONRPCError represents a protocol-level JSON-RPC error block.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolCallParams represents the arguments block when an LLM invokes an MCP tool.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Start launches the MCP JSON-RPC loop over Stdin and Stdout.
func Start(db *sql.DB) error {
	// MCP communicates strictly over stdin/stdout. We redirect logs to stderr
	// so they don't corrupt the JSON-RPC stream.
	fmt.Fprintln(os.Stderr, "LBMesh MCP Server started. Listening on stdin...")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(nil, -32700, "Parse error: "+err.Error())
			continue
		}

		// Dispatch request method
		handleRequest(db, &req)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "MCP read error: %v\n", err)
		return err
	}
	return nil
}

func handleRequest(db *sql.DB, req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		sendResponse(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]string{
				"name":    "lbmesh-mcp",
				"version": "1.0.4",
			},
		})

	case "tools/list":
		sendResponse(req.ID, map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "lbmesh_status",
					"description": "Retrieve the current running container services, active scopes, and mapped host-to-container TCP port allocations.",
					"inputSchema": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
				{
					"name":        "lbmesh_list_recipes",
					"description": "List all available cascading recipe templates from local workspaces, custom directories, connected Git Taps, and embedded defaults.",
					"inputSchema": map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
			},
		})

	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			sendError(req.ID, -32602, "Invalid params: "+err.Error())
			return
		}

		switch params.Name {
		case "lbmesh_status":
			statusMarkdown, err := queryStatus(db)
			if err != nil {
				sendError(req.ID, -32603, "Database status query failed: "+err.Error())
				return
			}
			sendResponse(req.ID, map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": statusMarkdown,
					},
				},
			})

		case "lbmesh_list_recipes":
			recipesMarkdown, err := queryRecipes()
			if err != nil {
				sendError(req.ID, -32603, "Catalog resolution failed: "+err.Error())
				return
			}
			sendResponse(req.ID, map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": recipesMarkdown,
					},
				},
			})

		default:
			sendError(req.ID, -32601, "Tool not found: "+params.Name)
		}

	case "notifications/initialized":
		// No response required for JSON-RPC notifications

	default:
		sendError(req.ID, -32601, "Method not found: "+req.Method)
	}
}

func sendResponse(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	writeJSON(resp)
}

func sendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	writeJSON(resp)
}

func writeJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "MCP marshal error: %v\n", err)
		return
	}
	// Write payload with newline as standard delimiter
	os.Stdout.Write(append(data, '\n'))
}

// ---------------------------------------------------------------------------
// Database & Catalog Helper Queries
// ---------------------------------------------------------------------------

func queryStatus(db *sql.DB) (string, error) {
	if db == nil {
		return "LBMesh Error: State Database is currently offline.", nil
	}

	// 1. Query Projects
	pRows, err := db.Query(`SELECT id, name, working_dir FROM projects`)
	if err != nil {
		return "", err
	}
	defer pRows.Close()

	type proj struct {
		id, name, dir string
	}
	var projects []proj
	for pRows.Next() {
		var p proj
		if err := pRows.Scan(&p.id, &p.name, &p.dir); err == nil {
			projects = append(projects, p)
		}
	}

	// 2. Query Services
	sRows, err := db.Query(`SELECT container_id, project_id, name, image, status FROM services`)
	if err != nil {
		return "", err
	}
	defer sRows.Close()

	type serv struct {
		id, projID, name, image, status string
	}
	var services []serv
	for sRows.Next() {
		var s serv
		if err := sRows.Scan(&s.id, &s.projID, &s.name, &s.image, &s.status); err == nil {
			services = append(services, s)
		}
	}

	// 3. Query Port Mappings
	ptRows, err := db.Query(`SELECT container_id, container_port, host_port, protocol FROM port_allocations`)
	if err != nil {
		return "", err
	}
	defer ptRows.Close()

	type portAlloc struct {
		cid, protocol            string
		containerPort, hostPort int
	}
	var ports []portAlloc
	for ptRows.Next() {
		var pt portAlloc
		if err := ptRows.Scan(&pt.cid, &pt.containerPort, &pt.hostPort, &pt.protocol); err == nil {
			ports = append(ports, pt)
		}
	}

	// Compile Markdown output
	out := "# LBMesh Orchestration Status\n\n"

	if len(projects) == 0 {
		out += "No active projects or system services registered in the database.\n"
		return out, nil
	}

	for _, p := range projects {
		out += fmt.Sprintf("## Project: %s (ID: %s)\n", p.name, p.id)
		if p.dir != "" {
			out += fmt.Sprintf("- **Workspace Directory**: `%s`\n", p.dir)
		}
		out += "\n"

		// Filter services belonging to this project
		var projServices []serv
		for _, s := range services {
			// Handle SQL NULL represented as empty string or match ID
			if s.projID == p.id || (p.id == "system-services" && s.projID == "") {
				projServices = append(projServices, s)
			}
		}

		if len(projServices) == 0 {
			out += "No containers running in this project scope.\n\n"
			continue
		}

		out += "| Container ID | Service Name | Image | Host Port Mapping | Status |\n"
		out += "| :--- | :--- | :--- | :--- | :--- |\n"

		for _, s := range projServices {
			// Find mapped ports
			portStr := "internal"
			for _, pt := range ports {
				if pt.cid == s.id {
					portStr = fmt.Sprintf("127.0.0.1:%d -> %d/%s", pt.hostPort, pt.containerPort, pt.protocol)
					break
				}
			}

			out += fmt.Sprintf("| `%s` | **%s** | `%s` | `%s` | %s |\n",
				s.id[:12], s.name, s.image, portStr, s.status)
		}
		out += "\n"
	}

	return out, nil
}

func queryRecipes() (string, error) {
	type row struct {
		name, category, source string
	}
	var rows []row

	// 1. Embedded defaults
	embeddedNames, err := recipe.EmbeddedNames()
	if err == nil {
		for _, n := range embeddedNames {
			rows = append(rows, row{name: n, category: "core", source: "embedded"})
		}
	}

	// Local check paths
	root, err := dirs.LocalRoot()
	if err == nil {
		// 2. User custom templates
		customDir := filepath.Join(root, "custom", "templates")
		if entries, err := os.ReadDir(customDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					rows = append(rows, row{name: e.Name(), category: "custom", source: "custom-override"})
				}
			}
		}

		// 3. Cached upstream catalog
		catDir := filepath.Join(root, "catalog", "templates")
		if cats, err := os.ReadDir(catDir); err == nil {
			for _, cat := range cats {
				if !cat.IsDir() {
					continue
				}
				catPath := filepath.Join(catDir, cat.Name())
				if names, err := os.ReadDir(catPath); err == nil {
					for _, n := range names {
						if n.IsDir() {
							rows = append(rows, row{name: n.Name(), category: cat.Name(), source: "catalog-cache"})
						}
					}
				}
			}
		}

		// 4. Git Taps
		taps, err := recipe.TapList()
		if err == nil {
			for _, tap := range taps {
				tapTemplates := filepath.Join(tap.Path, "templates")
				if names, err := os.ReadDir(tapTemplates); err == nil {
					for _, n := range names {
						if n.IsDir() {
							rows = append(rows, row{name: n.Name(), category: "tap-sync", source: "tap:" + tap.Slug})
						}
					}
				}
			}
		}
	}

	// Compile Markdown output
	out := "# LBMesh Cascading Recipe Registry\n\n"
	out += "All available templates that can be deployed using `lbmesh up <name>`:\n\n"
	out += "| Template Name | Category | Catalog Source |\n"
	out += "| :--- | :--- | :--- |\n"

	for _, r := range rows {
		out += fmt.Sprintf("| **%s** | `%s` | `%s` |\n", r.name, r.category, r.source)
	}

	return out, nil
}
