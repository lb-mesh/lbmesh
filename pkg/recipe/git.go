package recipe

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// gitClone runs `git clone --depth 1 <url> <targetDir>` using the system git
// binary so that the caller's SSH keys, HTTPS credential helpers, and
// git-config settings are all respected transparently.
//
// If the clone fails the combined stdout+stderr output is included in the
// returned error to aid diagnosis without exposing any credential material.
func gitClone(url, targetDir string) error {
	cmd := exec.Command("git", "clone", "--depth", "1", url, targetDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone %q into %q: %w\n%s",
			url, targetDir, err, sanitizeOutput(out))
	}
	return nil
}

// gitPull runs `git pull` inside an already-cloned repository directory using
// the system git binary.
func gitPull(repoDir string) error {
	cmd := exec.Command("git", "pull")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull in %q: %w\n%s",
			repoDir, err, sanitizeOutput(out))
	}
	return nil
}

// sanitizeOutput trims trailing whitespace from a command's combined output
// so that error messages remain tidy.  It never returns raw credential strings
// because git itself redacts those from its output.
func sanitizeOutput(b []byte) string {
	return strings.TrimRight(string(bytes.TrimSpace(b)), "\n")
}
