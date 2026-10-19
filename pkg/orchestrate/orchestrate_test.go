package orchestrate

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lbmesh/lbmesh-v2/pkg/db"
)

func TestResolveProject(t *testing.T) {
	database, err := db.OpenAndMigrate(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, s := range []string{
		`INSERT INTO projects (id, name, path) VALUES ('p1', 'webapp', '/a')`,
		`INSERT INTO projects (id, name, path) VALUES ('p2', 'shared', '/b')`,
		`INSERT INTO projects (id, name, path) VALUES ('p3', 'shared', '/c')`,
	} {
		if _, err := database.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()

	if p, err := ResolveProject(ctx, database, "p1"); err != nil || p.Name != "webapp" {
		t.Errorf("by ID: got %+v, %v", p, err)
	}
	if p, err := ResolveProject(ctx, database, "webapp"); err != nil || p.ID != "p1" {
		t.Errorf("by name: got %+v, %v", p, err)
	}
	if _, err := ResolveProject(ctx, database, "missing"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("missing: got %v, want ErrProjectNotFound", err)
	}
	if _, err := ResolveProject(ctx, database, "shared"); err == nil || !strings.Contains(err.Error(), "p2, p3") {
		t.Errorf("ambiguous: got %v, want error listing p2, p3", err)
	}
}

func TestShortID(t *testing.T) {
	for in, want := range map[string]string{
		"":                 "",
		"abc":              "abc",
		"0123456789ab":     "0123456789ab",
		"0123456789abcdef": "0123456789ab",
	} {
		if got := ShortID(in); got != want {
			t.Errorf("ShortID(%q) = %q, want %q", in, got, want)
		}
	}
}
