// Package mcp implements the LBMesh Model Context Protocol (MCP) server: a
// newline-delimited JSON-RPC 2.0 loop over stdin/stdout that exposes LBMesh
// state and orchestration as MCP tools.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lbmesh/lbmesh-v2/internal/version"
	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
	"github.com/lbmesh/lbmesh-v2/pkg/orchestrate"
	"github.com/lbmesh/lbmesh-v2/pkg/recipe"
	"github.com/lbmesh/lbmesh-v2/pkg/socket"
)

const serverName = "lbmesh-mcp"

// supportedProtocolVersions lists the MCP revisions this server can speak,
// newest first. The first entry is offered when the client asks for an
// unknown revision.
var supportedProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// JSONRPCRequest represents a JSON-RPC request or notification. A message
// without an "id" member is a notification and never receives a response.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC response. ID is null when the request
// ID could not be determined (for example on a parse error).
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
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

// projectArgs is the input of the lbmesh_up and lbmesh_down tools.
type projectArgs struct {
	Project string `json:"project"`
}

// EngineFactory connects to the container engine. The returned close function
// releases the connection.
type EngineFactory func() (orchestrate.Engine, func(), error)

// Options configures a Server.
type Options struct {
	// DB is the open state database. It may be nil, in which case tools that
	// need it report the database as offline.
	DB *sql.DB
	// Version is reported to clients in serverInfo.
	Version string
	// NewEngine connects to Docker/Podman. It is called lazily on the first
	// tool call that needs an engine, and again after a failed attempt.
	NewEngine EngineFactory
}

// Server is an MCP server instance.
type Server struct {
	opts        Options
	engine      orchestrate.Engine
	closeEngine func()
}

// NewServer constructs a Server.
func NewServer(opts Options) *Server {
	return &Server{opts: opts}
}

// Close releases the container engine connection, if one was opened.
func (s *Server) Close() {
	if s.closeEngine != nil {
		s.closeEngine()
		s.closeEngine = nil
		s.engine = nil
	}
}

// Start launches the MCP JSON-RPC loop over Stdin and Stdout.
func Start(db *sql.DB) error {
	// MCP communicates strictly over stdin/stdout. We redirect logs to stderr
	// so they don't corrupt the JSON-RPC stream.
	fmt.Fprintln(os.Stderr, "LBMesh MCP Server started. Listening on stdin...")

	srv := NewServer(Options{
		DB:        db,
		Version:   version.Version,
		NewEngine: detectEngine,
	})
	defer srv.Close()
	return srv.Serve(context.Background(), os.Stdin, os.Stdout)
}

// detectEngine is the default EngineFactory, backed by socket.Detect.
func detectEngine() (orchestrate.Engine, func(), error) {
	c, _, err := socket.Detect()
	if err != nil {
		return nil, nil, err
	}
	return c, func() { _ = c.Close() }, nil
}

// Serve reads newline-delimited JSON-RPC messages from in and writes responses
// to out until in reaches EOF.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	// bufio.Reader rather than bufio.Scanner: Scanner caps lines at 64KB.
	reader := bufio.NewReader(in)
	for {
		line, readErr := reader.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			if resp := s.handleMessage(ctx, line); resp != nil {
				if err := writeJSON(out, resp); err != nil {
					return err
				}
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "MCP read error: %v\n", readErr)
			return readErr
		}
	}
}

// handleMessage parses and dispatches one message, returning the response to
// send, or nil when none is due (notifications).
func (s *Server) handleMessage(ctx context.Context, line []byte) *JSONRPCResponse {
	var req JSONRPCRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return errorResponse(nil, codeParseError, "Parse error: "+err.Error())
	}

	if len(req.ID) == 0 {
		// Notifications (notifications/initialized, notifications/cancelled,
		// ...) must never be answered, even when unrecognised.
		return nil
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		return errorResponse(req.ID, codeInvalidRequest, `Invalid request: expected jsonrpc "2.0" and a method`)
	}

	result, rpcErr := s.dispatch(ctx, &req)
	if rpcErr != nil {
		return &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
	}
	return &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) dispatch(ctx context.Context, req *JSONRPCRequest) (interface{}, *JSONRPCError) {
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				return nil, &JSONRPCError{Code: codeInvalidParams, Message: "Invalid params: " + err.Error()}
			}
		}
		return map[string]interface{}{
			"protocolVersion": negotiateProtocolVersion(params.ProtocolVersion),
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]string{
				"name":    serverName,
				"version": s.opts.Version,
			},
		}, nil

	case "ping":
		return map[string]interface{}{}, nil

	case "tools/list":
		return map[string]interface{}{"tools": toolDefinitions()}, nil

	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &JSONRPCError{Code: codeInvalidParams, Message: "Invalid params: " + err.Error()}
		}
		return s.callTool(ctx, &params)

	default:
		return nil, &JSONRPCError{Code: codeMethodNotFound, Message: "Method not found: " + req.Method}
	}
}

// negotiateProtocolVersion echoes the client's requested revision when
// supported, and otherwise offers the newest supported revision.
func negotiateProtocolVersion(requested string) string {
	for _, v := range supportedProtocolVersions {
		if v == requested {
			return v
		}
	}
	return supportedProtocolVersions[0]
}

func toolDefinitions() []map[string]interface{} {
	noArgs := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
	projectArg := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"project": map[string]interface{}{
				"type":        "string",
				"description": "Project ID or exact project name, as shown by lbmesh_status.",
			},
		},
		"required": []string{"project"},
	}
	return []map[string]interface{}{
		{
			"name":        "lbmesh_status",
			"description": "Retrieve registered projects, their container services, and mapped host-to-container port allocations.",
			"inputSchema": noArgs,
		},
		{
			"name":        "lbmesh_list_recipes",
			"description": "List all available cascading recipe templates from custom directories, connected Git Taps, the cached catalog, and embedded defaults.",
			"inputSchema": noArgs,
		},
		{
			"name": "lbmesh_up",
			"description": "Start the containers of a project already registered with LBMesh. " +
				"Deploying a recipe that has never been deployed is not supported yet.",
			"inputSchema": projectArg,
		},
		{
			"name":        "lbmesh_down",
			"description": "Stop the running containers of a project registered with LBMesh. Containers are kept and can be started again with lbmesh_up.",
			"inputSchema": projectArg,
		},
	}
}

// toolResult is the result of a tools/call request.
type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textResult(text string) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: text}}}
}

// toolError reports a tool execution failure inside the result, so the model
// can see the message, rather than as a protocol error.
func toolError(text string) toolResult {
	r := textResult(text)
	r.IsError = true
	return r
}

func (s *Server) callTool(ctx context.Context, params *ToolCallParams) (interface{}, *JSONRPCError) {
	switch params.Name {
	case "lbmesh_status":
		if s.opts.DB == nil {
			return toolError("LBMesh state database is offline."), nil
		}
		out, err := queryStatus(ctx, s.opts.DB)
		if err != nil {
			return toolError("Database status query failed: " + err.Error()), nil
		}
		return textResult(out), nil

	case "lbmesh_list_recipes":
		return textResult(formatRecipes(collectRecipes())), nil

	case "lbmesh_up", "lbmesh_down":
		var args projectArgs
		if len(params.Arguments) > 0 && string(params.Arguments) != "null" {
			if err := json.Unmarshal(params.Arguments, &args); err != nil {
				return nil, &JSONRPCError{Code: codeInvalidParams, Message: "Invalid arguments: " + err.Error()}
			}
		}
		return s.projectAction(ctx, strings.TrimSpace(args.Project), params.Name == "lbmesh_up"), nil

	default:
		return nil, &JSONRPCError{Code: codeInvalidParams, Message: "Unknown tool: " + params.Name}
	}
}

// projectAction starts (up) or stops (!up) every container of a project.
func (s *Server) projectAction(ctx context.Context, ref string, up bool) toolResult {
	if ref == "" {
		return toolError(`The "project" argument is required. Call lbmesh_status to list registered projects.`)
	}
	if s.opts.DB == nil {
		return toolError("LBMesh state database is offline.")
	}

	project, err := orchestrate.ResolveProject(ctx, s.opts.DB, ref)
	if errors.Is(err, orchestrate.ErrProjectNotFound) {
		msg := fmt.Sprintf("No registered project matches %q. Call lbmesh_status to list registered projects.", ref)
		if up && isKnownRecipe(ref) {
			msg = fmt.Sprintf("%q is an available recipe, but it has not been deployed as a project. "+
				"Deploying a new recipe is not supported yet; lbmesh_up can only start a project that is already registered. "+
				"Call lbmesh_status to list registered projects.", ref)
		}
		return toolError(msg)
	}
	if err != nil {
		return toolError("Looking up project: " + err.Error())
	}

	ids, err := orchestrate.ContainerIDsForProject(ctx, s.opts.DB, project.ID)
	if err != nil {
		return toolError("Looking up project services: " + err.Error())
	}
	if len(ids) == 0 {
		return toolError(fmt.Sprintf("Project %s (ID: %s) has no containers registered.", project.Name, project.ID))
	}

	eng, err := s.getEngine()
	if err != nil {
		return toolError("No container engine available (is Docker or Podman running?): " + err.Error())
	}

	var res orchestrate.Result
	var summary, status string
	if up {
		res = orchestrate.StartContainers(ctx, eng, ids)
		summary, status = orchestrate.FormatStarted(res.Changed), "running"
	} else {
		res = orchestrate.StopContainers(ctx, eng, ids, orchestrate.DefaultStopTimeout)
		summary, status = orchestrate.FormatStopped(res.Changed), "exited"
	}

	text := fmt.Sprintf("Project %s (ID: %s): %s.", project.Name, project.ID, summary)
	if err := orchestrate.SetServiceStatus(ctx, s.opts.DB, res.Changed, status); err != nil {
		text += "\nWarning: the state database was not updated: " + err.Error()
	}
	if len(res.Errors) > 0 {
		return toolError(text + "\nFailed containers:\n- " + strings.Join(res.Errors, "\n- "))
	}
	return textResult(text)
}

// getEngine returns the cached engine connection, connecting on first use.
// A failed connection is not cached, so a later call can retry.
func (s *Server) getEngine() (orchestrate.Engine, error) {
	if s.engine != nil {
		return s.engine, nil
	}
	if s.opts.NewEngine == nil {
		return nil, errors.New("no container engine configured")
	}
	eng, closeFn, err := s.opts.NewEngine()
	if err != nil {
		return nil, err
	}
	s.engine, s.closeEngine = eng, closeFn
	return eng, nil
}

func errorResponse(id json.RawMessage, code int, message string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	}
}

func writeJSON(out io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "MCP marshal error: %v\n", err)
		return nil
	}
	// Write payload with newline as standard delimiter
	_, err = out.Write(append(data, '\n'))
	return err
}

// ---------------------------------------------------------------------------
// Database & Catalog Helper Queries
// ---------------------------------------------------------------------------

func queryStatus(ctx context.Context, db *sql.DB) (string, error) {
	// 1. Query Projects
	type proj struct {
		id, name, path, status string
	}
	var projects []proj
	pRows, err := db.QueryContext(ctx, `SELECT id, name, path, status FROM projects ORDER BY name, id`)
	if err != nil {
		return "", err
	}
	defer pRows.Close()
	for pRows.Next() {
		var p proj
		if err := pRows.Scan(&p.id, &p.name, &p.path, &p.status); err != nil {
			return "", err
		}
		projects = append(projects, p)
	}
	if err := pRows.Err(); err != nil {
		return "", err
	}

	// 2. Query Services, grouped by project ID ("" for unscoped services).
	type serv struct {
		id, name, scope, containerID, image, status string
	}
	servicesByProject := map[string][]serv{}
	sRows, err := db.QueryContext(ctx,
		`SELECT id, project_id, name, scope, container_id, image, status FROM services ORDER BY name, id`)
	if err != nil {
		return "", err
	}
	defer sRows.Close()
	for sRows.Next() {
		var s serv
		var projectID sql.NullString
		if err := sRows.Scan(&s.id, &projectID, &s.name, &s.scope, &s.containerID, &s.image, &s.status); err != nil {
			return "", err
		}
		servicesByProject[projectID.String] = append(servicesByProject[projectID.String], s)
	}
	if err := sRows.Err(); err != nil {
		return "", err
	}

	// 3. Query Port Mappings, grouped by service ID.
	portsByService := map[string][]string{}
	ptRows, err := db.QueryContext(ctx,
		`SELECT service_id, container_port, host_port, protocol FROM port_allocations ORDER BY host_port`)
	if err != nil {
		return "", err
	}
	defer ptRows.Close()
	for ptRows.Next() {
		var serviceID, protocol string
		var containerPort, hostPort int
		if err := ptRows.Scan(&serviceID, &containerPort, &hostPort, &protocol); err != nil {
			return "", err
		}
		portsByService[serviceID] = append(portsByService[serviceID],
			fmt.Sprintf("127.0.0.1:%d -> %d/%s", hostPort, containerPort, protocol))
	}
	if err := ptRows.Err(); err != nil {
		return "", err
	}

	writeServices := func(b *strings.Builder, services []serv) {
		if len(services) == 0 {
			b.WriteString("No services registered in this scope.\n\n")
			return
		}
		b.WriteString("| Container ID | Service Name | Scope | Image | Host Port Mapping | Status |\n")
		b.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
		for _, s := range services {
			containerID := "—"
			if s.containerID != "" {
				containerID = "`" + orchestrate.ShortID(s.containerID) + "`"
			}
			ports := "internal"
			if p := portsByService[s.id]; len(p) > 0 {
				ports = "`" + strings.Join(p, "`, `") + "`"
			}
			fmt.Fprintf(b, "| %s | **%s** | %s | `%s` | %s | %s |\n",
				containerID, cell(s.name), cell(s.scope), cell(s.image), ports, cell(s.status))
		}
		b.WriteString("\n")
	}

	// Compile Markdown output
	var b strings.Builder
	b.WriteString("# LBMesh Orchestration Status\n\n")

	unscoped := servicesByProject[""]
	if len(projects) == 0 && len(unscoped) == 0 {
		b.WriteString("No projects or system services registered in the database.\n")
		return b.String(), nil
	}

	for _, p := range projects {
		fmt.Fprintf(&b, "## Project: %s (ID: %s)\n", p.name, p.id)
		if p.path != "" {
			fmt.Fprintf(&b, "- **Workspace Directory**: `%s`\n", p.path)
		}
		fmt.Fprintf(&b, "- **Project Status**: %s\n\n", p.status)
		writeServices(&b, servicesByProject[p.id])
	}

	if len(unscoped) > 0 {
		b.WriteString("## System Services\n\n")
		writeServices(&b, unscoped)
	}

	return b.String(), nil
}

// cell escapes a value for use inside a Markdown table cell.
func cell(s string) string {
	return strings.ReplaceAll(s, "|", `\|`)
}

// recipeEntry is one template name in the recipe registry. Source is where the
// version that `lbmesh up` would use comes from; Shadowed lists lower-priority
// sources that also provide the name.
type recipeEntry struct {
	name, category, source string
	shadowed               []string
}

// collectRecipes lists recipe templates in the same priority order as
// recipe.Resolve (custom, taps, cached catalog, embedded) and keeps one entry
// per name. The workspace-local ./.lbmesh/recipe.yaml is skipped because the
// MCP server's working directory is chosen by the client.
func collectRecipes() []recipeEntry {
	var entries []recipeEntry
	index := map[string]int{}
	add := func(name, category, source string) {
		if i, ok := index[name]; ok {
			entries[i].shadowed = append(entries[i].shadowed, source)
			return
		}
		index[name] = len(entries)
		entries = append(entries, recipeEntry{name: name, category: category, source: source})
	}

	if root, err := dirs.LocalRoot(); err == nil {
		// 1. User custom templates
		for _, n := range subdirNames(filepath.Join(root, "custom", "templates")) {
			add(n, "custom", "custom-override")
		}

		// 2. Git Taps
		if taps, err := recipe.TapList(); err == nil {
			for _, tap := range taps {
				for _, n := range subdirNames(filepath.Join(tap.Path, "templates")) {
					add(n, "tap-sync", "tap:"+tap.Slug)
				}
			}
		}

		// 3. Cached upstream catalog
		catDir := filepath.Join(root, "catalog", "templates")
		for _, cat := range subdirNames(catDir) {
			for _, n := range subdirNames(filepath.Join(catDir, cat)) {
				add(n, cat, "catalog-cache")
			}
		}
	}

	// 4. Embedded defaults
	if names, err := recipe.EmbeddedNames(); err == nil {
		for _, n := range names {
			add(n, "core", "embedded")
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	return entries
}

func formatRecipes(entries []recipeEntry) string {
	var b strings.Builder
	b.WriteString("# LBMesh Cascading Recipe Registry\n\n")
	if len(entries) == 0 {
		b.WriteString("No recipe templates found.\n")
		return b.String()
	}
	b.WriteString("Available templates. When several sources provide the same name, the one listed under Catalog Source wins.\n\n")
	b.WriteString("| Template Name | Category | Catalog Source | Also Provided By |\n")
	b.WriteString("| :--- | :--- | :--- | :--- |\n")
	for _, e := range entries {
		also := "—"
		if len(e.shadowed) > 0 {
			also = "`" + strings.Join(e.shadowed, "`, `") + "`"
		}
		fmt.Fprintf(&b, "| **%s** | `%s` | `%s` | %s |\n", cell(e.name), cell(e.category), cell(e.source), also)
	}
	return b.String()
}

func isKnownRecipe(name string) bool {
	for _, e := range collectRecipes() {
		if e.name == name {
			return true
		}
	}
	return false
}

// subdirNames returns the names of the directories directly under dir, or nil
// when dir cannot be read.
func subdirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}
