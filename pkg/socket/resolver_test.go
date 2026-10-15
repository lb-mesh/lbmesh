package socket

import (
	"strings"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"my-project", "my-project"},
		{"My Project", "my-project"},
		{"my_project", "my-project"},
		{"my.project", "my-project"},
		{"/home/user/projects/MyApp", "home-user-projects-myapp"},
		{"  spaced  ", "spaced"},
		{"123numbers", "123numbers"},
		{"---hyphens---", "hyphens"},
		{"", ""},
	}

	for _, tc := range cases {
		got := slugify(tc.input)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNetworkNameForScope(t *testing.T) {
	cases := []struct {
		scope      Scope
		projectDir string
		instance   string
		wantPrefix string
		wantErr    bool
	}{
		{ScopeSystem, "", "", "lbmesh-global", false},
		{ScopeProject, "/home/user/projects/myapp", "", "lbmesh-project-myapp", false},
		{ScopeProject, "", "", "", true},
		{ScopeCustom, "", "redis-dev", "lbmesh-custom-redis-dev", false},
		{ScopeCustom, "", "", "", true},
		{Scope("unknown"), "", "", "", true},
	}

	for _, tc := range cases {
		got, err := NetworkNameForScope(tc.scope, tc.projectDir, tc.instance)
		if tc.wantErr {
			if err == nil {
				t.Errorf("NetworkNameForScope(%q, %q, %q) expected error, got %q", tc.scope, tc.projectDir, tc.instance, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NetworkNameForScope(%q, %q, %q) unexpected error: %v", tc.scope, tc.projectDir, tc.instance, err)
			continue
		}
		if !strings.HasPrefix(got, tc.wantPrefix) {
			t.Errorf("NetworkNameForScope(%q, %q, %q) = %q, want prefix %q", tc.scope, tc.projectDir, tc.instance, got, tc.wantPrefix)
		}
	}
}

func TestNetworkNameLength(t *testing.T) {
	// Network names must stay within Docker's 64-char limit.
	longDir := "/home/user/projects/" + strings.Repeat("a", 100) + "/service"
	name, err := NetworkNameForScope(ScopeProject, longDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(name) > 64 {
		t.Errorf("network name %q is %d chars, must be ≤ 64", name, len(name))
	}
}
