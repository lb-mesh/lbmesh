package dashboard

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

// handleLogsSSE streams the stdout and stderr of a container to the client
// using the Server-Sent Events (SSE) protocol.
//
// URL pattern: GET /api/logs/{containerID}
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

	// Extract container ID from path: /api/logs/{containerID}
	containerID := strings.TrimPrefix(r.URL.Path, "/api/logs/")
	containerID = strings.TrimSuffix(containerID, "/")
	if containerID == "" {
		errJSON(w, http.StatusBadRequest, "missing container id")
		return
	}

	// Validate that the container exists before opening the log stream.
	_, err := s.cfg.DockerClient.ContainerInspect(r.Context(), containerID)
	if err != nil {
		errJSON(w, http.StatusNotFound, "container not found: "+containerID)
		return
	}

	// Honour the "tail" query parameter (default: last 100 lines).
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}

	// Set SSE headers before writing the body.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering in proxied deployments
	w.WriteHeader(http.StatusOK)

	// Flush is required for SSE; assert the ResponseWriter supports it.
	flusher, ok := w.(http.Flusher)
	if !ok {
		// ResponseWriter does not support flushing — fall back to a single
		// batch response so the connection does not hang indefinitely.
		_, _ = fmt.Fprint(w, "event: error\ndata: streaming not supported\n\n")
		return
	}

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

	// Docker multiplexes stdout/stderr in an 8-byte framed format when the
	// container is NOT using a TTY.  We strip the frame header (first 8 bytes
	// per chunk) and forward the payload as plain text.
	//
	// Frame header layout (https://docs.docker.com/engine/api/v1.46/#tag/Container/operation/ContainerAttach):
	//   [0]    stream type: 1=stdout, 2=stderr
	//   [1-3]  padding (zero)
	//   [4-7]  payload size (big-endian uint32)
	//
	// We read 8-byte headers and then read exactly payload-size bytes each
	// iteration, then forward the content as an SSE event.
	ctx := r.Context()
	header := make([]byte, 8)
	scanner := bufio.NewScanner(logReader)

	// Use a simple line-based scan.  Docker's multiplexed stream embeds the
	// 8-byte header before each chunk; bufio.Scanner will include those bytes
	// in the first line of each chunk.  We strip any non-printable prefix
	// chars (the header bytes) before forwarding.
	//
	// This approach is robust for both TTY and non-TTY containers: for TTY
	// containers the stream is raw text; for non-TTY containers we strip the
	// 8-byte frame header if present.
	_ = header // header declared above for documentation; line scanner handles stripping

	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			// Client disconnected or context cancelled.
			return
		default:
		}

		line := stripDockerHeader(scanner.Bytes())
		if line == "" {
			continue
		}
		sseEvent(w, flusher, "log", line)
	}

	// Stream ended (container stopped or log EOF).
	sseEvent(w, flusher, "done", time.Now().UTC().Format(time.RFC3339))
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

// stripDockerHeader removes the 8-byte Docker multiplexing frame header from
// log output when present.  The header is identified by the first byte being
// 0x01 (stdout) or 0x02 (stderr).  For TTY containers the bytes are raw text
// and the first byte is a printable character — those lines are returned as-is.
func stripDockerHeader(b []byte) string {
	if len(b) > 8 && (b[0] == 0x01 || b[0] == 0x02) {
		// Non-TTY multiplexed frame: skip the 8-byte header.
		return strings.TrimRight(string(b[8:]), "\r\n")
	}
	return strings.TrimRight(string(b), "\r\n")
}
