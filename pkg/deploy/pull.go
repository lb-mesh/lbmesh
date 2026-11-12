package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/mattn/go-isatty"
)

// imagePullTimeout is the maximum time to allow for image downloads.
const imagePullTimeout = 15 * time.Minute

// pullMessage represents a single JSON event emitted by Docker/Podman ImagePull.
type pullMessage struct {
	Status         string          `json:"status"`
	ProgressDetail pullProgress    `json:"progressDetail"`
	Progress       string          `json:"progress"`
	ID             string          `json:"id"`
	Error          string          `json:"error"`
	ErrorDetail    *pullErrDetail  `json:"errorDetail"`
}

type pullProgress struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}

type pullErrDetail struct {
	Message string `json:"message"`
}

type layerState struct {
	downloadCurrent int64
	downloadTotal   int64
	extractCurrent  int64
	extractTotal    int64
	downloaded      bool
	extracted       bool
}

// ensureImage pulls the specified image ref if not already present, displaying
// a clean single-line progress bar.
//
// TODO: Support configurable pull policies (e.g. --pull=always|missing|never).
// When policy is "missing", probe local engine with ImageInspect first to avoid
// remote registry checks and allow offline execution.
func ensureImage(c *client.Client, ref string) error {
	ctx, cancel := context.WithTimeout(context.Background(), imagePullTimeout)
	defer cancel()

	fmt.Printf("Pulling image %q (skipped if already present)...\n", ref)
	rc, err := c.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pulling image %q: %w", ref, err)
	}
	defer rc.Close()

	return trackPullProgress(rc, os.Stdout, isTerminal(os.Stdout))
}

// isTerminal checks whether the given writer is an interactive terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// trackPullProgress reads Docker/Podman JSON messages from r and renders a clean
// progress bar to out.
func trackPullProgress(r io.Reader, out io.Writer, tty bool) error {
	dec := json.NewDecoder(r)
	layers := make(map[string]*layerState)
	var imageUpToDate bool
	var lastReportedMilestone int = -1
	var lastStatus string
	var hadProgress bool

	for {
		var msg pullMessage
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			// If JSON decoding fails, break without failing the entire pull.
			break
		}

		if msg.Error != "" {
			if tty {
				fmt.Fprint(out, "\r\033[K")
			}
			return fmt.Errorf("pulling image: %s", msg.Error)
		}
		if msg.ErrorDetail != nil && msg.ErrorDetail.Message != "" {
			if tty {
				fmt.Fprint(out, "\r\033[K")
			}
			return fmt.Errorf("pulling image: %s", msg.ErrorDetail.Message)
		}

		if strings.Contains(msg.Status, "Image is up to date") {
			imageUpToDate = true
		}

		if msg.ID != "" {
			l, ok := layers[msg.ID]
			if !ok {
				l = &layerState{}
				layers[msg.ID] = l
			}

			switch {
			case msg.Status == "Downloading":
				l.downloadCurrent = msg.ProgressDetail.Current
				l.downloadTotal = msg.ProgressDetail.Total
				hadProgress = true
			case msg.Status == "Download complete":
				l.downloaded = true
				if l.downloadTotal > 0 {
					l.downloadCurrent = l.downloadTotal
				}
				hadProgress = true
			case msg.Status == "Extracting":
				l.downloaded = true
				if l.downloadTotal > 0 {
					l.downloadCurrent = l.downloadTotal
				}
				l.extractCurrent = msg.ProgressDetail.Current
				l.extractTotal = msg.ProgressDetail.Total
				hadProgress = true
			case msg.Status == "Pull complete":
				l.downloaded = true
				l.extracted = true
				if l.downloadTotal > 0 {
					l.downloadCurrent = l.downloadTotal
				}
				if l.extractTotal > 0 {
					l.extractCurrent = l.extractTotal
				}
			case msg.Status == "Already exists":
				l.downloaded = true
				l.extracted = true
			}
		}

		// Calculate total download progress across all layers
		var totalDownload, currentDownload int64
		var totalExtract, currentExtract int64
		for _, l := range layers {
			if l.downloadTotal > 0 {
				totalDownload += l.downloadTotal
				currentDownload += l.downloadCurrent
			}
			if l.extractTotal > 0 {
				totalExtract += l.extractTotal
				currentExtract += l.extractCurrent
			}
		}

		var line string
		var currentPct float64

		if totalDownload > 0 {
			currentPct = (float64(currentDownload) * 100.0) / float64(totalDownload)
			if currentPct > 100 {
				currentPct = 100
			}

			// If download finished and extracting has progress, show extraction
			if currentDownload >= totalDownload && totalExtract > 0 {
				extractPct := (float64(currentExtract) * 100.0) / float64(totalExtract)
				if extractPct > 100 {
					extractPct = 100
				}
				bar := renderProgressBar(25, extractPct)
				line = fmt.Sprintf("  Extracting %s %3.0f%%", bar, extractPct)
			} else {
				bar := renderProgressBar(25, currentPct)
				line = fmt.Sprintf("  %s %3.0f%% (%s / %s)", bar, currentPct, formatBytes(currentDownload), formatBytes(totalDownload))
			}
		} else if len(layers) > 0 && !imageUpToDate {
			line = "  Preparing image layers..."
		}

		if tty {
			if line != "" && line != lastStatus {
				fmt.Fprintf(out, "\r\033[K%s", line)
				lastStatus = line
			}
		} else {
			if totalDownload > 0 {
				milestone := (int(currentPct) / 25) * 25
				if milestone > lastReportedMilestone && milestone > 0 {
					lastReportedMilestone = milestone
					fmt.Fprintf(out, "  %s %3d%% (%s / %s)\n", renderProgressBar(25, float64(milestone)), milestone, formatBytes(currentDownload), formatBytes(totalDownload))
				}
			}
		}
	}

	if tty {
		fmt.Fprint(out, "\r\033[K")
	}

	if imageUpToDate || (!hadProgress && len(layers) > 0) {
		fmt.Fprintln(out, "  ✓ Image is up to date")
	} else {
		fmt.Fprintln(out, "  ✓ Download complete")
	}

	return nil
}

// renderProgressBar formats an ASCII progress bar of given character width.
func renderProgressBar(width int, percentage float64) string {
	if percentage < 0 {
		percentage = 0
	}
	if percentage > 100 {
		percentage = 100
	}
	filled := int((percentage / 100.0) * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	var bar strings.Builder
	bar.WriteByte('[')
	if filled > 0 {
		if filled == width {
			bar.WriteString(strings.Repeat("=", filled))
		} else {
			bar.WriteString(strings.Repeat("=", filled-1))
			bar.WriteByte('>')
		}
	}
	if empty > 0 {
		bar.WriteString(strings.Repeat(" ", empty))
	}
	bar.WriteByte(']')
	return bar.String()
}

// formatBytes returns a human-readable representation of byte count.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
