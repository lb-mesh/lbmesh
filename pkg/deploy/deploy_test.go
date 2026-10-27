package deploy

import (
	"archive/tar"
	"io"
	"testing"
)

func TestParseEnvOverrides(t *testing.T) {
	pairs := []string{"FOO=bar", "BAZ=qux=with=equals", "EMPTY="}
	got := parseEnvOverrides(pairs)
	want := map[string]string{
		"FOO":   "bar",
		"BAZ":   "qux=with=equals",
		"EMPTY": "",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("parseEnvOverrides[%q] = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("len = %d, want %d", len(got), len(want))
	}
}

func TestMergeEnv(t *testing.T) {
	base := map[string]string{"A": "1", "B": "2"}
	overrides := map[string]string{"B": "override", "C": "3"}
	result := mergeEnv(base, overrides)

	m := make(map[string]string, len(result))
	for _, kv := range result {
		k, v := splitKV(kv)
		m[k] = v
	}
	if m["A"] != "1" {
		t.Errorf("A = %q, want %q", m["A"], "1")
	}
	if m["B"] != "override" {
		t.Errorf("B = %q, want %q", m["B"], "override")
	}
	if m["C"] != "3" {
		t.Errorf("C = %q, want %q", m["C"], "3")
	}
}

// splitKV splits "KEY=VALUE" at the first '='.
func splitKV(kv string) (string, string) {
	for i, c := range kv {
		if c == '=' {
			return kv[:i], kv[i+1:]
		}
	}
	return kv, ""
}

func TestVolumeName(t *testing.T) {
	cases := []struct {
		service string
		path    string
		want    string
	}{
		{"datapower", "/opt/ibm/datapower/drouter/config", "lbmesh-datapower-opt-ibm-datapower-drouter-config"},
		{"mysql", "/var/lib/mysql", "lbmesh-mysql-var-lib-mysql"},
	}
	for _, tc := range cases {
		got := volumeName(tc.service, tc.path)
		if got != tc.want {
			t.Errorf("volumeName(%q, %q) = %q, want %q", tc.service, tc.path, got, tc.want)
		}
	}
}

func TestToSocketScope(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"system", false},
		{"project", false},
		{"custom", false},
		{"invalid", true},
		{"", true},
	}
	for _, tc := range cases {
		_, err := toSocketScope(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("toSocketScope(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
		}
	}
}

func TestFileMode(t *testing.T) {
	if fileMode("") != 0o644 {
		t.Errorf("fileMode(\"\") = %o, want 644", fileMode(""))
	}
	if fileMode("0755") != 0o755 {
		t.Errorf("fileMode(\"0755\") = %o, want 755", fileMode("0755"))
	}
	if fileMode("0666") != 0o666 {
		t.Errorf("fileMode(\"0666\") = %o, want 666", fileMode("0666"))
	}
	if fileMode("bad") != 0o644 {
		t.Errorf("fileMode(\"bad\") should fall back to 644, got %o", fileMode("bad"))
	}
}

func TestBuildTar(t *testing.T) {
	content := []byte("hello world")
	r := buildTar("config.cfg", content, 0o644)
	tr := tar.NewReader(r)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tar.Next: %v", err)
	}
	if hdr.Name != "config.cfg" {
		t.Errorf("header name = %q, want %q", hdr.Name, "config.cfg")
	}
	if hdr.Mode != 0o644 {
		t.Errorf("header mode = %o, want 644", hdr.Mode)
	}
	got, _ := io.ReadAll(tr)
	if string(got) != "hello world" {
		t.Errorf("content = %q, want %q", string(got), "hello world")
	}
}
