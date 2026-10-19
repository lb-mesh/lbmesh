// Package socket provides auto-detection and probing of container runtime
// daemon endpoints (Docker, and Podman through its Docker-compatible API).
package socket

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/docker/docker/client"

	"github.com/lbmesh/lbmesh-v2/pkg/machine"
)

// ---------------------------------------------------------------------------
// Candidate engine endpoints
// ---------------------------------------------------------------------------

// candidateHosts returns the ordered list of engine endpoints to probe, as
// Docker client host URLs ("unix://…", "npipe://…", "tcp://…").
//
// Priority order:
//  1. DOCKER_HOST, then CONTAINER_HOST (Podman), exactly as set.
//  2. Platform defaults, Podman before Docker (see platformHosts).
//
// Endpoints the Docker client cannot dial itself (ssh://) are skipped.
func candidateHosts() []string {
	var hosts []string
	seen := map[string]bool{}
	add := func(h string) {
		if h == "" || seen[h] {
			return
		}
		seen[h] = true
		hosts = append(hosts, h)
	}

	for _, env := range []string{"DOCKER_HOST", "CONTAINER_HOST"} {
		if h, ok := normalizeHost(os.Getenv(env)); ok {
			add(h)
		}
	}

	home, _ := os.UserHomeDir()
	for _, h := range platformHostsFn(runtime.GOOS, home, os.Getenv("TMPDIR"),
		os.Getenv("XDG_RUNTIME_DIR"), os.Getuid()) {
		add(h)
	}
	return hosts
}

// platformHostsFn is platformHosts, as a variable so tests can stub it out.
var platformHostsFn = platformHosts

// platformHosts lists the well-known Podman and Docker endpoints for goos.
func platformHosts(goos, home, tmpDir, xdgRuntimeDir string, uid int) []string {
	unix := func(path string) string { return "unix://" + path }
	var hosts []string

	switch goos {
	case "darwin":
		if tmpDir == "" {
			tmpDir = os.TempDir()
		}
		// Podman 5 (applehv / libkrun): $TMPDIR/podman/<machine>-api.sock.
		// The default machine is listed first; any other machine name is
		// picked up by the glob.
		hosts = append(hosts, unix(filepath.Join(tmpDir, "podman", "podman-machine-default-api.sock")))
		if matches, _ := filepath.Glob(filepath.Join(tmpDir, "podman", "*-api.sock")); len(matches) > 0 {
			for _, m := range matches {
				hosts = append(hosts, unix(m))
			}
		}
		if home != "" {
			hosts = append(hosts,
				// Podman 5 stable symlink to the API socket.
				unix(filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock")),
				// Podman 4 QEMU machines.
				unix(filepath.Join(home, ".local", "share", "containers", "podman", "machine", "qemu", "podman.sock")),
				unix(filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman-machine-default", "podman.sock")),
				// Docker Desktop 4.13+ (per-user socket).
				unix(filepath.Join(home, ".docker", "run", "docker.sock")),
			)
		}
		// podman-mac-helper or Docker Desktop's privileged symlink.
		hosts = append(hosts, unix("/var/run/docker.sock"))

	case "windows":
		hosts = append(hosts,
			"npipe:////./pipe/podman-machine-default",
			"npipe:////./pipe/docker_engine",
		)

	default: // linux and other unix
		// Rootless Podman: $XDG_RUNTIME_DIR/podman/podman.sock.
		if xdgRuntimeDir == "" && uid > 0 {
			xdgRuntimeDir = fmt.Sprintf("/run/user/%d", uid)
		}
		if xdgRuntimeDir != "" {
			hosts = append(hosts, unix(filepath.Join(xdgRuntimeDir, "podman", "podman.sock")))
		}
		// Rootful Podman.
		hosts = append(hosts, unix("/run/podman/podman.sock"))
		if home != "" {
			// Docker Desktop for Linux.
			hosts = append(hosts, unix(filepath.Join(home, ".docker", "desktop", "docker.sock")))
		}
		hosts = append(hosts, unix("/var/run/docker.sock"))
	}
	return hosts
}

// normalizeHost turns a DOCKER_HOST / CONTAINER_HOST value into a Docker
// client host URL. A bare path is treated as a unix socket. It reports false
// for empty values and for schemes the Docker client cannot dial (ssh://,
// which Podman uses for remote connections).
func normalizeHost(v string) (string, bool) {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return "", false
	case strings.HasPrefix(v, "ssh://"):
		return "", false
	case strings.Contains(v, "://"):
		return v, true
	default:
		return "unix://" + v, true
	}
}

// reachable reports whether host might have a live engine behind it, so that
// missing sockets are skipped without a ping timeout. Only unix sockets can
// be checked locally; pipes and TCP endpoints are always tried.
func reachable(host string) bool {
	path, ok := strings.CutPrefix(host, "unix://")
	if !ok {
		return true
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// ModeSocket is set on UNIX domain socket files (os.Stat follows the
	// symlinks Podman and Docker Desktop create).
	return info.Mode()&os.ModeSocket != 0
}

// ---------------------------------------------------------------------------
// Client probing
// ---------------------------------------------------------------------------

// pingTimeout is the maximum time allowed for a single daemon ping attempt.
const pingTimeout = 3 * time.Second

// podmanAPIHosts asks the podman CLI for its machine sockets. It is a variable
// so tests can stub it out.
var podmanAPIHosts = machine.PodmanAPIHosts

// Detect walks the candidate endpoints, attempts a live ping against each
// one, and returns the first verified *client.Client together with the host
// URL it is connected to. Docker and Podman (via its Docker-compatible API)
// are both supported.
//
// When none of the well-known endpoints answers and the podman CLI is
// installed, the sockets reported by `podman machine inspect` are tried as a
// last resort; this finds machines with custom names or VM providers.
//
// If no active daemon is found, an [ErrNoEngine] sentinel error is returned and
// a human-readable setup guide is printed to stderr so that callers do not need
// to handle the messaging themselves.
func Detect() (*client.Client, string, error) {
	tried := candidateHosts()
	if c, host := firstLive(tried); c != nil {
		return c, host, nil
	}

	if extra, err := podmanAPIHosts(); err == nil {
		var fresh []string
		for _, h := range extra {
			if !slices.Contains(tried, h) {
				fresh = append(fresh, h)
			}
		}
		if c, host := firstLive(fresh); c != nil {
			return c, host, nil
		}
		tried = append(tried, fresh...)
	}

	PrintSetupInstructions(tried)
	return nil, "", ErrNoEngine
}

// firstLive returns a client for the first host that answers a ping.
func firstLive(hosts []string) (*client.Client, string) {
	for _, host := range hosts {
		if !reachable(host) {
			continue
		}

		c, err := client.NewClientWithOpts(
			client.WithHost(host),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			// Unsupported scheme on this platform, malformed URL, etc.
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		_, pingErr := c.Ping(ctx)
		cancel()

		if pingErr == nil {
			return c, host
		}

		// Ping failed — close the idle connection and try the next candidate.
		_ = c.Close()
	}
	return nil, ""
}

// ---------------------------------------------------------------------------
// Sentinel error
// ---------------------------------------------------------------------------

// ErrNoEngine is returned by Detect when no responsive container runtime
// daemon can be found on any of the candidate socket paths.
var ErrNoEngine = fmt.Errorf("no active container engine detected")
