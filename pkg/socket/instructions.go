package socket

import (
	"fmt"
	"os"
	"runtime"
)

// PrintSetupInstructions writes a detailed, platform-aware guide to os.Stderr
// explaining how to install and start a container engine when no active daemon
// socket was found.  It is called automatically by Detect before returning
// ErrNoEngine so callers only need to handle the error value.
func PrintSetupInstructions() {
	w := os.Stderr

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "╔══════════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(w, "║  lbmesh — No Active Container Engine Detected                    ║")
	fmt.Fprintln(w, "╚══════════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "lbmesh requires either Docker or Podman to be installed and running.")
	fmt.Fprintln(w, "No responsive daemon socket was found after checking:")
	fmt.Fprintln(w, "")

	for _, p := range candidatePaths() {
		fmt.Fprintf(w, "  • %s\n", p)
	}

	fmt.Fprintln(w, "")

	switch runtime.GOOS {
	case "darwin":
		printMacOSInstructions(w)
	case "linux":
		printLinuxInstructions(w)
	case "windows":
		printWindowsInstructions(w)
	default:
		printGenericInstructions(w)
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "After starting your container engine, re-run your lbmesh command.")
	fmt.Fprintln(w, "")
}

// ---------------------------------------------------------------------------
// Platform-specific guides
// ---------------------------------------------------------------------------

func printMacOSInstructions(w *os.File) {
	fmt.Fprintln(w, "── macOS Setup ─────────────────────────────────────────────────────")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Option A — Podman Desktop (recommended, free & open-source):")
	fmt.Fprintln(w, "  1. Install Podman Desktop:")
	fmt.Fprintln(w, "       brew install --cask podman-desktop")
	fmt.Fprintln(w, "  2. Open Podman Desktop and follow the onboarding wizard, or run:")
	fmt.Fprintln(w, "       podman machine init")
	fmt.Fprintln(w, "       podman machine start")
	fmt.Fprintln(w, "  3. Verify the socket is active:")
	fmt.Fprintln(w, "       podman info")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Option B — Docker Desktop:")
	fmt.Fprintln(w, "  1. Download Docker Desktop from https://www.docker.com/products/docker-desktop/")
	fmt.Fprintln(w, "  2. Install and launch Docker Desktop.")
	fmt.Fprintln(w, "  3. Wait for the whale icon in the menu bar to show 'Docker Desktop is running'.")
	fmt.Fprintln(w, "  4. Verify:")
	fmt.Fprintln(w, "       docker info")
}

func printLinuxInstructions(w *os.File) {
	fmt.Fprintln(w, "── Linux Setup ─────────────────────────────────────────────────────")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Option A — Rootless Podman (recommended, no root required):")
	fmt.Fprintln(w, "  1. Install Podman:")
	fmt.Fprintln(w, "       # Debian/Ubuntu")
	fmt.Fprintln(w, "       sudo apt-get install -y podman")
	fmt.Fprintln(w, "       # Fedora/RHEL")
	fmt.Fprintln(w, "       sudo dnf install -y podman")
	fmt.Fprintln(w, "  2. Enable the Podman user socket:")
	fmt.Fprintln(w, "       systemctl --user enable --now podman.socket")
	fmt.Fprintln(w, "  3. Export the socket path in your shell profile:")
	fmt.Fprintln(w, "       export CONTAINER_HOST=unix://${XDG_RUNTIME_DIR}/podman/podman.sock")
	fmt.Fprintln(w, "  4. Verify:")
	fmt.Fprintln(w, "       podman info")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Option B — Docker Engine (requires root / sudo group):")
	fmt.Fprintln(w, "  1. Install Docker Engine:")
	fmt.Fprintln(w, "       curl -fsSL https://get.docker.com | sh")
	fmt.Fprintln(w, "  2. Add your user to the docker group:")
	fmt.Fprintln(w, "       sudo usermod -aG docker $USER && newgrp docker")
	fmt.Fprintln(w, "  3. Start the Docker daemon:")
	fmt.Fprintln(w, "       sudo systemctl enable --now docker")
	fmt.Fprintln(w, "  4. Verify:")
	fmt.Fprintln(w, "       docker info")
}

func printWindowsInstructions(w *os.File) {
	fmt.Fprintln(w, "── Windows Setup ───────────────────────────────────────────────────")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Option A — Podman Desktop (recommended, free & open-source):")
	fmt.Fprintln(w, "  1. Download Podman Desktop from https://podman-desktop.io/")
	fmt.Fprintln(w, "  2. Run the installer and complete the onboarding wizard.")
	fmt.Fprintln(w, "  3. Ensure Podman Machine is initialised and running:")
	fmt.Fprintln(w, "       podman machine init")
	fmt.Fprintln(w, "       podman machine start")
	fmt.Fprintln(w, "  4. Verify:")
	fmt.Fprintln(w, "       podman info")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Option B — Docker Desktop:")
	fmt.Fprintln(w, "  1. Download Docker Desktop from https://www.docker.com/products/docker-desktop/")
	fmt.Fprintln(w, "  2. Install and launch Docker Desktop (requires WSL 2 or Hyper-V).")
	fmt.Fprintln(w, "  3. Verify from PowerShell or WSL2:")
	fmt.Fprintln(w, "       docker info")
}

func printGenericInstructions(w *os.File) {
	fmt.Fprintln(w, "── Generic Setup ───────────────────────────────────────────────────")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Install either Docker or Podman for your OS and ensure the daemon is")
	fmt.Fprintln(w, "running before retrying. You can override the socket path by setting:")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  export DOCKER_HOST=unix:///path/to/docker.sock")
	fmt.Fprintln(w, "  export CONTAINER_HOST=unix:///path/to/podman.sock")
}
