package recipe

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
)

// ---------------------------------------------------------------------------
// Tap layout
//
// Each tap is cloned into:
//   ~/.lbmesh/taps/<host>/<owner>/<repo>/
//
// The directory path is derived from the git URL so that multiple taps from
// different hosts never collide and the layout mirrors Go module cache
// conventions, making it immediately recognisable to developers.
// ---------------------------------------------------------------------------

// tapDirForURL returns the absolute filesystem path where a tap for the
// given git URL should be cloned.
//
// Examples:
//
//	https://github.com/acme/recipes  → ~/.lbmesh/taps/github.com/acme/recipes
//	git@github.com:acme/recipes.git  → ~/.lbmesh/taps/github.com/acme/recipes
func tapDirForURL(rawURL string) (string, error) {
	root, err := dirs.LocalRoot()
	if err != nil {
		return "", err
	}

	slug, err := urlToSlug(rawURL)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "taps", filepath.FromSlash(slug)), nil
}

// urlToSlug converts an HTTPS or SCP-style SSH git URL into a clean
// "<host>/<owner>/<repo>" path segment (without a trailing ".git").
func urlToSlug(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)

	// SCP-style SSH: git@github.com:owner/repo.git
	if strings.HasPrefix(rawURL, "git@") {
		// Strip "git@" prefix.
		rest := strings.TrimPrefix(rawURL, "git@")
		// Split on the first colon: "github.com:owner/repo.git"
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("could not parse SCP-style git URL %q", rawURL)
		}
		host := parts[0]
		path := strings.TrimSuffix(parts[1], ".git")
		return host + "/" + path, nil
	}

	// Standard HTTPS / SSH URL.
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing git URL %q: %w", rawURL, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("git URL %q has no host component", rawURL)
	}
	path := strings.TrimSuffix(u.Path, ".git")
	path = strings.TrimPrefix(path, "/")
	return u.Host + "/" + path, nil
}

// ---------------------------------------------------------------------------
// Public tap operations called by the CLI
// ---------------------------------------------------------------------------

// TapAdd clones a remote git repository into the lbmesh taps directory.
// It uses the system git binary so that the caller's SSH keys and credential
// helpers are honoured without any special handling inside lbmesh.
//
// If the tap directory already exists the call is a no-op and a descriptive
// error is returned so the CLI can print a helpful message.
func TapAdd(gitURL string) (string, error) {
	tapDir, err := tapDirForURL(gitURL)
	if err != nil {
		return "", fmt.Errorf("resolving tap directory: %w", err)
	}

	if dirExists(tapDir) {
		return tapDir, fmt.Errorf("tap already registered at %s — run `lbmesh tap sync` to update it", tapDir)
	}

	// Ensure the parent directory exists before cloning.
	if err := os.MkdirAll(filepath.Dir(tapDir), 0o755); err != nil {
		return "", fmt.Errorf("creating tap parent directory: %w", err)
	}

	if err := gitClone(gitURL, tapDir); err != nil {
		return "", err
	}

	return tapDir, nil
}

// TapEntry describes a single registered tap.
type TapEntry struct {
	// Slug is the "<host>/<owner>/<repo>" identifier.
	Slug string
	// Path is the absolute filesystem path to the cloned repository.
	Path string
}

// TapList returns all taps registered under ~/.lbmesh/taps/.
// It performs a three-level directory walk (host/owner/repo) to mirror the
// layout created by TapAdd.
func TapList() ([]TapEntry, error) {
	root, err := dirs.LocalRoot()
	if err != nil {
		return nil, err
	}

	tapsRoot := filepath.Join(root, "taps")

	var entries []TapEntry

	// Level 1: host directories (e.g. github.com)
	hosts, err := readDirNames(tapsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return entries, nil
		}
		return nil, fmt.Errorf("reading taps directory: %w", err)
	}

	for _, host := range hosts {
		hostPath := filepath.Join(tapsRoot, host)

		// Level 2: owner directories
		owners, err := readDirNames(hostPath)
		if err != nil {
			continue
		}

		for _, owner := range owners {
			ownerPath := filepath.Join(hostPath, owner)

			// Level 3: repo directories
			repos, err := readDirNames(ownerPath)
			if err != nil {
				continue
			}

			for _, repo := range repos {
				repoPath := filepath.Join(ownerPath, repo)
				if !dirExists(repoPath) {
					continue
				}
				entries = append(entries, TapEntry{
					Slug: host + "/" + owner + "/" + repo,
					Path: repoPath,
				})
			}
		}
	}

	return entries, nil
}

// TapSyncOne runs `git pull` inside the single repository directory at
// repoDir.  It is used by the CLI to report per-tap progress independently.
func TapSyncOne(repoDir string) error {
	return gitPull(repoDir)
}

// TapSync runs `git pull` on every registered tap repository.  It continues
// past individual failures, collecting errors, and returns a combined error
// at the end so the caller sees a full picture of what succeeded and what
// failed.
func TapSync() error {
	entries, err := TapList()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	var errs []string
	for _, e := range entries {
		if err := TapSyncOne(e.Path); err != nil {
			errs = append(errs, fmt.Sprintf("  %s: %v", e.Slug, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("one or more taps failed to sync:\n%s", strings.Join(errs, "\n"))
	}

	return nil
}

// ---------------------------------------------------------------------------
// Filesystem helpers
// ---------------------------------------------------------------------------

// dirExists returns true when path points to an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// readDirNames returns the names of direct children of dir that are
// directories, ignoring hidden entries.
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
