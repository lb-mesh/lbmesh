// Package dirs manages the ~/.lbmesh/ local directory tree.
package dirs

import (
	"fmt"
	"os"
	"path/filepath"
)

// LocalRoot returns the absolute path to the lbmesh home directory (~/.lbmesh/).
func LocalRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".lbmesh"), nil
}

// requiredDirs lists all subdirectories that must exist under LocalRoot().
var requiredDirs = []string{
	"config",
	"custom/templates",
	"catalog/templates",
	"taps",
	"projects",
}

// EnsureLocalDirs creates all required subdirectories under ~/.lbmesh/ if they
// do not already exist.  It is safe to call on every CLI invocation.
func EnsureLocalDirs() error {
	root, err := LocalRoot()
	if err != nil {
		return err
	}

	for _, rel := range requiredDirs {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", path, err)
		}
	}

	return nil
}
