// Package socket provides auto-detection and probing of container runtime
// daemon sockets (Docker and Podman) without spawning external processes.
package socket

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/docker/docker/client"
)

// ---------------------------------------------------------------------------
// Candidate socket path resolution
// ---------------------------------------------------------------------------

// candidatePaths returns the ordered list of socket paths to probe.
// Environment variable overrides (DOCKER_HOST / CONTAINER_HOST) are checked
// first, followed by well-known default locations for each platform.
//
// Priority order:
//  1. DOCKER_HOST env var
//  2. CONTAINER_HOST env var  (Podman)
//  3. macOS / Windows: Podman machine QEMU socket
//  4. Linux rootless Podman: $XDG_RUNTIME_DIR/podman/podman.sock
//  5. Classic Docker socket:  /var/run/docker.sock
func candidatePaths() []string {
	var paths []string

	// 1. DOCKER_HOST environment override.
	if v := os.Getenv("DOCKER_HOST"); v != "" {
		paths = append(paths, stripUnixScheme(v))
	}

	// 2. CONTAINER_HOST environment override (Podman).
	if v := os.Getenv("CONTAINER_HOST"); v != "" {
		paths = append(paths, stripUnixScheme(v))
	}

	switch runtime.GOOS {
	case "darwin", "windows":
		// 3. Podman Desktop / podman machine (QEMU) socket path on macOS and Windows.
		if home, err := os.UserHomeDir(); err == nil {
			paths = append(paths,
				filepath.Join(home, ".local", "share", "containers", "podman",
					"machine", "qemu", "podman.sock"),
			)
		}
	default:
		// 4. Linux rootless Podman: $XDG_RUNTIME_DIR/podman/podman.sock.
		if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
			paths = append(paths, filepath.Join(xdg, "podman", "podman.sock"))
		}
	}

	// 5. Default Docker socket present on all UNIX-like systems.
	paths = append(paths, "/var/run/docker.sock")

	return paths
}

// stripUnixScheme removes the "unix://" prefix that tools like Docker CLI write
// into DOCKER_HOST / CONTAINER_HOST so we always work with a raw file path.
func stripUnixScheme(s string) string {
	const prefix = "unix://"
	if len(s) > len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

// socketExists returns true when path points to an existing UNIX socket file.
func socketExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// ModeSocket is set on UNIX domain socket files.
	return info.Mode()&os.ModeSocket != 0
}

// ---------------------------------------------------------------------------
// Client probing
// ---------------------------------------------------------------------------

// pingTimeout is the maximum time allowed for a single daemon ping attempt.
const pingTimeout = 3 * time.Second

// Detect walks the candidate socket list, attempts a live ping against each
// one, and returns the first verified *client.Client together with the socket
// path it is connected to.
//
// If no active daemon is found, an [ErrNoEngine] sentinel error is returned and
// a human-readable setup guide is printed to stderr so that callers do not need
// to handle the messaging themselves.
func Detect() (*client.Client, string, error) {
	for _, sockPath := range candidatePaths() {
		if !socketExists(sockPath) {
			continue
		}

		c, err := client.NewClientWithOpts(
			client.WithHost("unix://"+sockPath),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			// Construction failure is unusual; skip and try the next candidate.
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		_, pingErr := c.Ping(ctx)
		cancel()

		if pingErr == nil {
			return c, sockPath, nil
		}

		// Ping failed — close the idle connection and try the next candidate.
		_ = c.Close()
	}

	PrintSetupInstructions()
	return nil, "", ErrNoEngine
}

// ---------------------------------------------------------------------------
// Sentinel error
// ---------------------------------------------------------------------------

// ErrNoEngine is returned by Detect when no responsive container runtime
// daemon can be found on any of the candidate socket paths.
var ErrNoEngine = fmt.Errorf("no active container engine detected")
