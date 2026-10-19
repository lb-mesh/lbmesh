package socket

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestPlatformHosts_Darwin(t *testing.T) {
	tmp := t.TempDir()
	// A Podman 5 machine with a non-default name.
	if err := os.MkdirAll(filepath.Join(tmp, "podman"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "podman", "dev-api.sock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	hosts := platformHosts("darwin", "/Users/me", tmp, "", 501)
	want := []string{
		"unix://" + filepath.Join(tmp, "podman", "podman-machine-default-api.sock"),
		"unix://" + filepath.Join(tmp, "podman", "dev-api.sock"),
		"unix:///Users/me/.local/share/containers/podman/machine/podman.sock",
		"unix:///Users/me/.docker/run/docker.sock",
		"unix:///var/run/docker.sock",
	}
	for _, w := range want {
		if !slices.Contains(hosts, w) {
			t.Errorf("darwin hosts missing %s\ngot %v", w, hosts)
		}
	}
	// Podman must be preferred over the shared /var/run/docker.sock symlink.
	if slices.Index(hosts, want[0]) > slices.Index(hosts, "unix:///var/run/docker.sock") {
		t.Errorf("Podman socket should come before /var/run/docker.sock: %v", hosts)
	}
}

func TestPlatformHosts_Linux(t *testing.T) {
	hosts := platformHosts("linux", "/home/me", "", "", 1000)
	for _, w := range []string{
		"unix:///run/user/1000/podman/podman.sock", // rootless, XDG_RUNTIME_DIR unset
		"unix:///run/podman/podman.sock",           // rootful
		"unix:///var/run/docker.sock",
	} {
		if !slices.Contains(hosts, w) {
			t.Errorf("linux hosts missing %s\ngot %v", w, hosts)
		}
	}

	hosts = platformHosts("linux", "/home/me", "", "/run/user/42", 1000)
	if hosts[0] != "unix:///run/user/42/podman/podman.sock" {
		t.Errorf("XDG_RUNTIME_DIR not honoured: %v", hosts)
	}
}

func TestPlatformHosts_Windows(t *testing.T) {
	hosts := platformHosts("windows", `C:\Users\me`, "", "", -1)
	want := []string{"npipe:////./pipe/podman-machine-default", "npipe:////./pipe/docker_engine"}
	if !slices.Equal(hosts, want) {
		t.Errorf("windows hosts = %v, want %v", hosts, want)
	}
}

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "", false},
		{"unix:///run/podman/podman.sock", "unix:///run/podman/podman.sock", true},
		{"/run/podman/podman.sock", "unix:///run/podman/podman.sock", true},
		{"tcp://127.0.0.1:2375", "tcp://127.0.0.1:2375", true},
		{"npipe:////./pipe/podman-machine-default", "npipe:////./pipe/podman-machine-default", true},
		{"ssh://core@127.0.0.1:49804/run/podman/podman.sock", "", false},
	}
	for _, tc := range tests {
		got, ok := normalizeHost(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("normalizeHost(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestCandidateHosts_EnvFirstAndDeduped(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("CONTAINER_HOST", "unix:///var/run/docker.sock")
	hosts := candidateHosts()
	if hosts[0] != "unix:///var/run/docker.sock" {
		t.Fatalf("CONTAINER_HOST not first: %v", hosts)
	}
	if n := countOf(hosts, "unix:///var/run/docker.sock"); n != 1 {
		t.Fatalf("duplicate host listed %d times: %v", n, hosts)
	}
}

func TestReachable(t *testing.T) {
	dir, err := os.MkdirTemp("", "lbm") // short path: unix socket paths are length-limited
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if !reachable("unix://" + sock) {
		t.Error("live unix socket reported unreachable")
	}
	if reachable("unix://" + filepath.Join(dir, "missing.sock")) {
		t.Error("missing socket reported reachable")
	}
	if !reachable("npipe:////./pipe/docker_engine") || !reachable("tcp://127.0.0.1:1") {
		t.Error("non-unix hosts must always be tried")
	}
}

func TestDetect_FallsBackToPodmanCLI(t *testing.T) {
	// No well-known endpoint and no podman: Detect must fail cleanly.
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/a.sock")
	t.Setenv("CONTAINER_HOST", "")
	origPlatform := platformHostsFn
	platformHostsFn = func(string, string, string, string, int) []string { return nil }
	defer func() { platformHostsFn = origPlatform }()

	called := false
	orig := podmanAPIHosts
	podmanAPIHosts = func() ([]string, error) {
		called = true
		return nil, errors.New("podman not installed")
	}
	defer func() { podmanAPIHosts = orig }()

	_, _, err := Detect()
	if !errors.Is(err, ErrNoEngine) {
		t.Fatalf("Detect err = %v, want ErrNoEngine", err)
	}
	if !called {
		t.Fatal("podman CLI fallback was not consulted")
	}
}

func countOf(list []string, v string) int {
	n := 0
	for _, s := range list {
		if s == v {
			n++
		}
	}
	return n
}
