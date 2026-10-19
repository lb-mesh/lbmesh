package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/lbmesh/lbmesh-v2/internal/version"
)

var testInfo = version.Info{
	Version:   "1.2.3",
	Commit:    "abc1234",
	Date:      "2026-10-19T19:00:00Z",
	GoVersion: "go1.26.4",
	Platform:  "darwin/arm64",
}

func TestPrintVersion_Text(t *testing.T) {
	var out bytes.Buffer
	if err := printVersion(&out, testInfo, false, "text"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"lbmesh 1.2.3", "abc1234", "2026-10-19T19:00:00Z", "go1.26.4", "darwin/arm64"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestPrintVersion_Short(t *testing.T) {
	var out bytes.Buffer
	if err := printVersion(&out, testInfo, true, "text"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "1.2.3\n" {
		t.Fatalf("short output = %q, want %q", out.String(), "1.2.3\n")
	}
}

func TestPrintVersion_JSON(t *testing.T) {
	var out bytes.Buffer
	if err := printVersion(&out, testInfo, false, "json"); err != nil {
		t.Fatal(err)
	}
	var got version.Info
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	if got != testInfo {
		t.Fatalf("decoded %+v, want %+v", got, testInfo)
	}
}

func TestPrintVersion_UnknownFormat(t *testing.T) {
	if err := printVersion(&bytes.Buffer{}, testInfo, false, "yaml"); err == nil {
		t.Fatal("expected an error for an unknown output format")
	}
}

// TestVersionCmd_DoesNotTouchState runs the real command tree with HOME
// pointed at an empty directory and checks that ~/.lbmesh is not created.
func TestVersionCmd_DoesNotTouchState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version", "--short"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatal("version printed nothing")
	}
	if entries, _ := os.ReadDir(home); len(entries) != 0 {
		t.Fatalf("version command created %v in HOME", entries)
	}
}
