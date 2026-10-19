package dashboard

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// handleLogsSSE streams the stdout and stderr of a container to the client
// using the Server-Sent Events (SSE) protocol.
//
// URL patterns: GET /api/logs/{containerID}  or  GET /api/logs?container={id}
//
// Each log line is emitted as an SSE event:
//
//	event: log
//	data: <line text>\n\n
//
// When the container stops producing output, a final "event: done" event is
// sent and the connection is closed.
//
// The client may request historical lines via the "tail" query parameter
// (default "100").  Pass "all" to receive the full log history.
func (s *Server) handleLogsSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.cfg.DockerClient == nil {
		errJSON(w, http.StatusServiceUnavailable, "container engine not available")
		return
	}

	containerID := logsContainerID(r)
	if containerID == "" {
		errJSON(w, http.StatusBadRequest, "missing container id")
		return
	}

	// Validate that the container exists before opening the log stream, and
	// learn whether it uses a TTY, which decides the log stream framing.
	inspect, err := s.cfg.DockerClient.ContainerInspect(r.Context(), containerID)
	if err != nil {
		errJSON(w, http.StatusNotFound, "container not found: "+containerID)
		return
	}
	tty := inspect.Config != nil && inspect.Config.Tty

	// Honour the "tail" query parameter (default: last 100 lines).
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}

	// Flush is required for SSE; check before committing to a 200 response.
	flusher, ok := w.(http.Flusher)
	if !ok {
		errJSON(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Set SSE headers before writing the body.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering in proxied deployments
	w.WriteHeader(http.StatusOK)

	// Open the Docker log stream: stdout + stderr, follow=true (live tail).
	logOpts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       tail,
		Timestamps: false,
	}

	logReader, err := s.cfg.DockerClient.ContainerLogs(r.Context(), containerID, logOpts)
	if err != nil {
		sseEvent(w, flusher, "error", "failed to open log stream: "+err.Error())
		return
	}
	defer logReader.Close()

	ctx := r.Context()
	scanner := bufio.NewScanner(logLines(logReader, tty))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			// Client disconnected or context cancelled.
			return
		default:
		}

		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			continue
		}
		sseEvent(w, flusher, "log", line)
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		sseEvent(w, flusher, "error", "reading log stream: "+err.Error())
	}

	// Stream ended (container stopped or log EOF).
	sseEvent(w, flusher, "done", time.Now().UTC().Format(time.RFC3339))
}

// logsContainerID extracts the container ID from /api/logs/{id} or, failing
// that, from the ?container= query parameter.
func logsContainerID(r *http.Request) string {
	id := strings.TrimPrefix(r.URL.Path, "/api/logs")
	id = strings.Trim(id, "/")
	if id == "" {
		id = r.URL.Query().Get("container")
	}
	return strings.TrimSpace(id)
}

// logLines returns a reader of plain log text. Containers without a TTY send
// stdout and stderr multiplexed in 8-byte-header frames whose boundaries do
// not line up with newlines, so the frames are decoded with stdcopy and both
// streams are merged; TTY containers already send raw text.
func logLines(src io.Reader, tty bool) io.Reader {
	if tty {
		return src
	}
	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, src)
		_ = pw.CloseWithError(err)
	}()
	return pr
}

// ---------------------------------------------------------------------------
// SSE helpers
// ---------------------------------------------------------------------------

// sseEvent writes a single Server-Sent Event to w and flushes.
// The SSE wire format is:
//
//	event: <eventType>\n
//	data: <payload>\n
//	\n
func sseEvent(w http.ResponseWriter, f http.Flusher, eventType, data string) {
	// Replace newlines in data with SSE continuation lines to keep the
	// protocol well-formed (each data: line is one logical message fragment).
	data = strings.ReplaceAll(data, "\n", "\ndata: ")
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
	f.Flush()
}
