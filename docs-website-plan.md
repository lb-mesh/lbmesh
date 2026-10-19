# LBMesh Documentation Website Plan & Unified Brand Specification

This document details the master plan for updating the official documentation website (`https://github.com/lb-mesh/lbmesh-website-docs.git`) to reflect the new compiled Go CLI, and establishes the **LBMesh Unified Brand & Color Palette Specification** to style all future assets, merchandise, documentation views, and dashboard interfaces.

---

## Part 1: LBMesh Unified Brand Specification

To support developer merchandise (t-shirts, stickers, hoodies) and maintain a consistent look across the documentation site and future dashboard updates, we define a **Neon Cyber-Tech** visual identity. It represents high-speed Go compilation, native socket piping, and LLM orchestration.

### 1. The Color Palette

| Usage Scope | Color Name | Hex Code | Visual Aura & Context |
| :--- | :--- | :--- | :--- |
| **Primary Backdrop (Dark)** | Deep Space | `#0B0C10` | Rich cybernetic dark background for dark-mode CLI, code snippets, and app backdrops. |
| **Secondary Backdrop (Dark)**| Hyper Slate | `#1F2833` | Component cards, dashboard tiles, and code block containers. |
| **Primary Backdrop (Light)**| Clean Space | `#F4F5F7` | Light-mode base color for responsive, readable documentation and clean dashboards. |
| **Primary Accent** | Electric Cyan | `#00F0FF` | Represents native Docker/Podman UNIX sockets, high-speed HTTP streams, and port bindings. |
| **Secondary Accent** | Neon Purple | `#A800FF` | Represents the cascading recipe engine, LLM planning, and conversational AI provisioning. |
| **Core Text (Light Mode)** | Tech Charcoal| `#1F2328` | High-contrast body copy for light layouts. |
| **Core Text (Dark Mode)** | Cyber Silver | `#C5C6C7` | Crisp, easy-to-read silver-gray for dark layouts and logs. |
| **Telemetry / Status OK** | Terminal Green| `#39E539` | Standard glowing green for running indicators and terminal text. |
| **OOM Deficit / Status Error**| Electric Rose | `#FF0055` | Glowing hot-pinkish-red for errors, OOM warnings, and halts. |

### 2. Typography & Fonts
- **Monospace Code**: `IBM Plex Mono` (fits our IBM Carbon connection perfectly, ideal for CLI outputs, code blocks, and hardware specifications).
- **Body & Headings**: `IBM Plex Sans` or system sans-serif (`-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto`).

### 3. Merchandise Concept & Aesthetics
- **Vector Icons**: High-contrast, minimalist outline geometries of intertwined mesh lattices or custom container nodes using **Electric Cyan** (`#00F0FF`) and **Neon Purple** (`#A800FF`) over **Deep Space** (`#0B0C10`) fabrics.
- **The CLI Glow**: Terminal outputs can use ANSI color escapes matching Electric Cyan for commands and Terminal Green for success outputs.

---

## Part 2: Documentation Website Update Plan

The documentation repository `lbmesh-website-docs` must be synchronized to guide developers on setting up, configuring, and deploying stacks using our single Go binary.

### Top-Level Overview
We will restructure the existing documentation repository, updating installation guides, architecture blueprints, directory layouts, and step-by-step command reference tables to match the new CGO-free, socket-probing execution model.

---

### Sub-Tasks

#### Sub-Task W1: Installation & Getting Started Overhaul
- **Intent**: Guide developers through installing and initializing LBMesh, emphasizing the single-binary zero-dependencies model.
- **Expected Outcomes**:
  - Rewritten installation guide covering macOS (Homebrew tap), Linux (binary curl installer), and Windows (WSL2 config).
  - Clear `lbmesh init` startup walkthrough.
- **Todo List**:
  1. [ ] Rewrite the home/getting-started documentation page to outline LBMesh'scompiled Go architecture.
  2. [ ] Provide clear installation copy blocks:
     - macOS Brew: `brew tap lb-mesh/tap && brew install lbmesh`
     - Linux Curl: `curl -fsSL https://get.lbmesh.io | sh`
  3. [ ] Detail the directory structure created on initialization, explaining what resides in `~/.lbmesh/` (`config.yaml`, `state.db`, `taps/`, `catalog/`, `custom/`).
- **Status**: `[ ] pending`

#### Sub-Task W2: Core Architecture & Socket Piping Guide
- **Intent**: Detail how LBMesh connects directly to container daemons over Unix/TCP sockets without executing shell wrappers.
- **Expected Outcomes**:
  - High-fidelity architecture page mapping out Socket Probing and Podman/Docker auto-detection.
- **Todo List**:
  1. [ ] Author the **Piping Architecture** guide explaining the socket priority fallback hierarchy: `DOCKER_HOST` / `CONTAINER_HOST` $\rightarrow$ macOS/Windows Podman VM paths $\rightarrow$ Linux XDG rootless $\rightarrow$ standard `/var/run/docker.sock`.
  2. [ ] Explain why this client model guarantees zero performance lag and native concurrency.
  3. [ ] Detail platform-specific configurations for starting/sharing sockets (e.g. enabling `podman system service`).
- **Status**: `[ ] pending`

#### Sub-Task W3: Cascading Recipe & Git Taps Manual
- **Intent**: Teach developers how to author, lint, override, and distribute custom infrastructure recipes using YAML and Git.
- **Expected Outcomes**:
  - Comprehensive guide explaining the `recipe.v1.yaml` schema, cascading paths, and Git Taps.
- **Todo List**:
  1. [ ] Document the complete `recipe.v1.yaml` schema fields, providing copyable starter blueprints for database and backend stacks.
  2. [ ] Detail the **Cascading Resolution Order** (Local $\rightarrow$ Custom $\rightarrow$ Taps $\rightarrow$ Catalog Cache $\rightarrow$ Embedded Defaults), demonstrating how developers can customize shared templates locally.
  3. [ ] Author the **Git Taps** guide: Explain how teams can manage private repository catalogs and sync them using native git commands via `lbmesh tap add` and `lbmesh tap sync`.
- **Status**: `[ ] pending`

#### Sub-Task W4: Complete CLI Command Reference
- **Intent**: Provide a comprehensive, interactive command-line dictionary for all 13 core CLI commands.
- **Expected Outcomes**:
  - Detailed CLI table with usage guidelines, argument boundaries, and flags for every subcommand.
- **Todo List**:
  1. [ ] Document command parameters, scopes (`project`|`system`|`custom`), environment variables, and output schemas for:
     - Core commands: `init`, `up`, `down`, `status`.
     - Recipe/Catalog commands: `catalog list`, `catalog sync`, `recipe init`, `recipe lint`.
     - Git Tap commands: `tap add`, `tap list`, `tap sync`.
     - Web UI: `dashboard` (detailing embedded assets, REST, and SSE logs).
     - AI Agent: `ask` (explaining Ollama, OpenAI, and LiteLLM gateway connections).
- **Status**: `[ ] pending`

#### Sub-Task W5: Web Dashboard & AI Assistant User Guide
- **Intent**: Showcase the embedded Carbon Design Dashboard and teach developers how to leverage LLMs to deploy stacks conversationally.
- **Expected Outcomes**:
  - Dedicated visual feature guides with screenshots/blueprints for the web console and AI ask workflows.
- **Todo List**:
  1. [ ] Write the **Carbon Dashboard** guide, detailing the System Overview metrics, Project sheets, Port maps, Catalog, and the SSE log-viewer.
  2. [ ] Write the **Conversational Provisioning (`lbmesh ask`)** guide: Show how to configure LiteLLM or local Ollama in `config.yaml` to parse natural language instructions into running containers.
  3. [ ] Explain how LBMesh prevents virtualization vm crashes via the **OOM Capacity Advisor** before container deployment.
- **Status**: `[ ] pending`
