# LBMesh — Autonomous Local Infrastructure & Container Orchestration

`LBMesh` is an open-source, compiled, dependency-free Go CLI and local service engine designed to replace manual `docker-compose` friction and heavyweight Node-based CLI scaffolding. It connects natively to running container daemons (Docker or Podman) over direct HTTP sockets, manages dynamic zero-conflict port allocations, compiles cascading recipe structures, and hosts an embedded React + IBM Carbon Design System dashboard with an integrated Model Context Protocol (MCP) server.

---

## ⚡ Key Architectural Features

1.  **Direct Socket IPC (No Shell Wrappers)**: Native communication with Docker and Podman daemon sockets without shell-exec dependencies or platform-specific wrapper lag.
2.  **Zero-Conflict Port Allocation**: SQLite-backed port broker checks for active mappings combined with physical host TCP socket probes (`net.Listen`) to dynamically re-map conflicting container ports on-the-fly.
3.  **Cascading Recipe Engine**: Declarative `recipe.v1.yaml` templates resolved from a cascading hierarchy (Local Workspace $\rightarrow$ User Custom $\rightarrow$ Git Taps $\rightarrow$ Cached Catalog). No recipes ship inside the binary; `lbmesh catalog sync` downloads the official catalog.
4.  **OOM Capacity Advisor**: Proactive host and `podman machine` memory inspection to prevent out-of-memory container terminations before deployment.
5.  **Embedded Carbon Dashboard**: A single self-contained binary shipping with an embedded React, TypeScript, and Vite single-page application served over local REST/SSE endpoints.
6.  **Native MCP Server**: Fully compliant **Model Context Protocol (MCP)** server built natively into the core binary, enabling AI clients (Cursor, Claude Code, Windsurf) to query, list, and orchestrate container environments out-of-the-box.

---

## 🚀 Fast-Track Installation

To install the latest stable pre-compiled binary of LBMesh for macOS (Intel/Apple Silicon), Linux (AMD64/ARM64), or Windows WSL2, run this single command in your terminal:

```bash
curl -fsSL https://get.lbmesh.io | sh
```

*This script auto-detects your platform/architecture, downloads the matching release, registers `/usr/local/bin/lbmesh`, and runs a silent initialization.*

### Alternative Install via Homebrew Tap
```bash
brew tap lb-mesh/tap
brew install lbmesh
```

---

### Supported Container Engines

LBMesh talks to Docker or Podman through the Docker-compatible API; no CLI wrapper is involved. It probes, in order:

- `DOCKER_HOST`, then `CONTAINER_HOST` (`unix://`, `tcp://` or `npipe://`; `ssh://` is not supported)
- **macOS:** Podman 5 machine sockets (`$TMPDIR/podman/*-api.sock`, `~/.local/share/containers/podman/machine/podman.sock`), Podman 4 QEMU sockets, Docker Desktop (`~/.docker/run/docker.sock`), then `/var/run/docker.sock`
- **Linux:** rootless Podman (`$XDG_RUNTIME_DIR/podman/podman.sock`; run `systemctl --user enable --now podman.socket`), rootful Podman (`/run/podman/podman.sock`), Docker Desktop, then `/var/run/docker.sock`
- **Windows:** `npipe:////./pipe/podman-machine-default`, then `npipe:////./pipe/docker_engine`

If none answers and `podman` is installed, LBMesh asks `podman machine inspect` for the sockets of any running machine.

---

## 🛠️ CLI Quickstart

Once installed, bootstrap your local environment in seconds:

```bash
# 1. Initialize local directory structures (~/.lbmesh/) and default configuration
lbmesh init

# 2. Download the official recipe catalog (required before the first `lbmesh up`)
lbmesh catalog sync

# 3. View running containers, active scopes, and re-mapped port bindings
lbmesh status

# 4. List available recipes across your cascading catalog
lbmesh catalog list

# 5. Deploy a recipe (e.g., PostgreSQL from the synced catalog)
lbmesh up postgresql

# 6. Launch the embedded React + Carbon Web Dashboard
lbmesh dashboard --port 8080
```

---

## 🤖 AI Editor Integration (MCP Server)

LBMesh embeds a native Model Context Protocol (MCP) server. This allows AI developer clients to directly query and manage your local container environments.

### 1. Claude Desktop Configuration
Add the following configuration block to your Claude Desktop configuration file (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "lbmesh": {
      "command": "lbmesh",
      "args": ["mcp", "start"]
    }
  }
}
```

### 2. Available AI Tools
Once configured, your AI assistant can natively call:
-   `lbmesh_status`: View running services, active scopes, container IDs, and port mappings.
-   `lbmesh_list_recipes`: View available Cascading Recipe templates to deploy.
-   `lbmesh_up`: Start the containers of a registered project (by project ID or name).
-   `lbmesh_down`: Stop the running containers of a registered project.

---

## 🎨 Unified Color Themes (Neon Cyber-Tech)
LBMesh assets, terminal outputs, and dashboard environments are styled under a unified cybernetic brand palette:
*   **Deep Space** (`#0B0C10`): Main Canvas Backdrop.
*   **Hyper Slate** (`#1F2833`): Container Frames, Code Blocks, and Cards.
*   **Electric Cyan** (`#00F0FF`): Sockets, TCP Ports, and Dynamic Pipes.
*   **Neon Purple** (`#A800FF`): Cascading Recipes, LLM gateways, and AI.
*   **Terminal Green** (`#39E539`): Healthy running containers and successful tasks.
*   **Electric Rose** (`#FF0055`): OOM halts, warnings, and error logs.

---

## 📄 License

This project is open-source software licensed under the **[Apache License 2.0](LICENSE)**.
