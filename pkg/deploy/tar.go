package deploy

import (
	"archive/tar"
	"bytes"
	"io"
	"strconv"
	"time"
)

// buildTar returns an in-memory tar archive containing a single file named
// name with the given content and Unix permission mode.
func buildTar(name string, content []byte, mode int64) io.Reader {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{
		Name:     name,
		Size:     int64(len(content)),
		Mode:     mode,
		ModTime:  time.Now(),
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(content)
	_ = tw.Close()
	return &buf
}

// fileMode parses an octal permission string (e.g. "0644") into an int64
// suitable for tar.Header.Mode. Returns 0644 when mode is empty or invalid.
func fileMode(mode string) int64 {
	if mode == "" {
		return 0o644
	}
	m, err := strconv.ParseInt(mode, 8, 64)
	if err != nil {
		return 0o644
	}
	return m
}
