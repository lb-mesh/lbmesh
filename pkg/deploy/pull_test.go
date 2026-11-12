package deploy

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderProgressBar(t *testing.T) {
	tests := []struct {
		width      int
		percentage float64
		want       string
	}{
		{width: 10, percentage: 0, want: "[          ]"},
		{width: 10, percentage: 50, want: "[====>     ]"},
		{width: 10, percentage: 100, want: "[==========]"},
		{width: 10, percentage: -10, want: "[          ]"},
		{width: 10, percentage: 150, want: "[==========]"},
	}

	for _, tt := range tests {
		got := renderProgressBar(tt.width, tt.percentage)
		if got != tt.want {
			t.Errorf("renderProgressBar(%d, %f) = %q, want %q", tt.width, tt.percentage, got, tt.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		got := formatBytes(tt.bytes)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestTrackPullProgress_DownloadAndComplete(t *testing.T) {
	jsonStream := `{"status":"Pulling fs layer","progressDetail":{},"id":"329828f10dd1"}
{"status":"Pulling fs layer","progressDetail":{},"id":"9c94f276affd"}
{"status":"Downloading","progressDetail":{"current":10000000,"total":20000000},"id":"329828f10dd1"}
{"status":"Downloading","progressDetail":{"current":20000000,"total":20000000},"id":"329828f10dd1"}
{"status":"Download complete","progressDetail":{},"id":"329828f10dd1"}
{"status":"Downloading","progressDetail":{"current":5000000,"total":10000000},"id":"9c94f276affd"}
{"status":"Downloading","progressDetail":{"current":10000000,"total":10000000},"id":"9c94f276affd"}
{"status":"Download complete","progressDetail":{},"id":"9c94f276affd"}
{"status":"Pull complete","progressDetail":{},"id":"329828f10dd1"}
{"status":"Pull complete","progressDetail":{},"id":"9c94f276affd"}
`
	var out bytes.Buffer
	err := trackPullProgress(strings.NewReader(jsonStream), &out, false)
	if err != nil {
		t.Fatalf("trackPullProgress failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "✓ Download complete") {
		t.Errorf("expected completion message, got: %q", output)
	}
	if !strings.Contains(output, "%") {
		t.Errorf("expected percentage in output, got: %q", output)
	}
}

func TestTrackPullProgress_AlreadyUpToDate(t *testing.T) {
	jsonStream := `{"status":"Image is up to date for mysql:8.4"}`
	var out bytes.Buffer
	err := trackPullProgress(strings.NewReader(jsonStream), &out, false)
	if err != nil {
		t.Fatalf("trackPullProgress failed: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "✓ Image is up to date") {
		t.Errorf("expected up to date message, got: %q", output)
	}
}

func TestTrackPullProgress_Error(t *testing.T) {
	jsonStream := `{"errorDetail":{"message":"manifest unknown"},"error":"manifest unknown"}`
	var out bytes.Buffer
	err := trackPullProgress(strings.NewReader(jsonStream), &out, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "manifest unknown") {
		t.Errorf("expected 'manifest unknown' error, got: %v", err)
	}
}
