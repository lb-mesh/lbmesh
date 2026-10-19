package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseTimestamp_DefaultColumnValue(t *testing.T) {
	database, err := OpenAndMigrate(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec(`INSERT INTO projects (id, name, path) VALUES ('p1', 'demo', '/tmp/demo')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var raw any
	if err := database.QueryRow(`SELECT created_at FROM projects WHERE id = 'p1'`).Scan(&raw); err != nil {
		t.Fatalf("scan: %v", err)
	}
	got := ParseTimestamp(raw)
	if got.IsZero() || time.Since(got) > time.Minute {
		t.Fatalf("ParseTimestamp(%#v) = %v, want a time within the last minute", raw, got)
	}
}

func TestParseTimestamp_TextForms(t *testing.T) {
	want := time.Date(2026, 10, 19, 18, 39, 23, 0, time.UTC)
	for _, in := range []any{
		"2026-10-19 18:39:23",
		"2026-10-19T18:39:23Z",
		"2026-10-19 18:39:23+00:00",
		[]byte("2026-10-19T18:39:23Z"),
		want,
	} {
		if got := ParseTimestamp(in); !got.Equal(want) {
			t.Errorf("ParseTimestamp(%#v) = %v, want %v", in, got, want)
		}
	}
	if got := ParseTimestamp("garbage"); !got.IsZero() {
		t.Errorf("ParseTimestamp(garbage) = %v, want zero", got)
	}
}

func TestGetNetwork_CreatedAt(t *testing.T) {
	database, err := OpenAndMigrate(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("OpenAndMigrate: %v", err)
	}
	defer database.Close()

	if err := UpsertNetwork(database, Network{ID: "n1", Name: "lbmesh-global", Scope: "system", Driver: "bridge"}); err != nil {
		t.Fatalf("UpsertNetwork: %v", err)
	}
	n, err := GetNetwork(database, "lbmesh-global")
	if err != nil {
		t.Fatalf("GetNetwork: %v", err)
	}
	if n.CreatedAt.IsZero() || time.Since(n.CreatedAt) > time.Minute {
		t.Fatalf("CreatedAt = %v, want a recent time", n.CreatedAt)
	}
}
