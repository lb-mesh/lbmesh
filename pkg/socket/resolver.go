package socket

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/docker/docker/client"
)

// ---------------------------------------------------------------------------
// Scope constants
// ---------------------------------------------------------------------------

// Scope classifies how a recipe is being launched, which determines which
// bridge network is created and joined.
type Scope string

const (
	// ScopeSystem is for core/global shared stacks (reverse proxies, shared
	// databases, mail catchers). All system-scoped services share the single
	// lbmesh-global bridge network.
	ScopeSystem Scope = "system"

	// ScopeProject is for workspace-specific services. Each project directory
	// path is slugified into a unique lbmesh-project-<slug> bridge so that
	// services from different projects cannot collide on DNS names.
	ScopeProject Scope = "project"

	// ScopeCustom is for user-defined custom templates. Each named custom
	// instance gets its own lbmesh-custom-<instance> bridge.
	ScopeCustom Scope = "custom"
)

// ---------------------------------------------------------------------------
// Network name derivation
// ---------------------------------------------------------------------------

// NetworkNameForScope returns the canonical bridge network name for a given
// execution scope.
//
//   - ScopeSystem  → "lbmesh-global"
//   - ScopeProject → "lbmesh-project-<slug of projectDir>"
//   - ScopeCustom  → "lbmesh-custom-<instance>"
//
// projectDir must be set for ScopeProject; instance must be set for
// ScopeCustom. The function returns an error when required arguments are
// missing.
func NetworkNameForScope(scope Scope, projectDir, instance string) (string, error) {
	switch scope {
	case ScopeSystem:
		return "lbmesh-global", nil

	case ScopeProject:
		if projectDir == "" {
			return "", fmt.Errorf("project scope requires a non-empty project directory")
		}
		slug := slugify(filepath.Base(projectDir))
		if slug == "" {
			// Fall back to a slug of the full path when the base is not useful.
			slug = slugify(projectDir)
		}
		return "lbmesh-project-" + slug, nil

	case ScopeCustom:
		if instance == "" {
			return "", fmt.Errorf("custom scope requires a non-empty instance name")
		}
		return "lbmesh-custom-" + slugify(instance), nil

	default:
		return "", fmt.Errorf("unknown scope %q", scope)
	}
}

// ---------------------------------------------------------------------------
// High-level resolver
// ---------------------------------------------------------------------------

// ResolveNetwork derives the correct network name for the given scope and
// ensures that network exists in the container runtime, creating it when
// necessary. It returns the NetworkInfo for the live network.
//
// This is the primary entry-point for the orchestration engine to call before
// starting containers — callers receive an already-created, ready-to-join
// network without having to orchestrate the create/inspect cycle themselves.
func ResolveNetwork(c *client.Client, scope Scope, projectDir, instance string) (NetworkInfo, error) {
	name, err := NetworkNameForScope(scope, projectDir, instance)
	if err != nil {
		return NetworkInfo{}, err
	}

	info, err := EnsureNetwork(c, name)
	if err != nil {
		return NetworkInfo{}, fmt.Errorf("ensuring %s-scope network %q: %w", scope, name, err)
	}

	return info, nil
}

// ---------------------------------------------------------------------------
// Slug helper
// ---------------------------------------------------------------------------

// nonAlnum matches any character that is not a letter, digit, or hyphen.
var nonAlnum = regexp.MustCompile(`[^a-z0-9-]+`)

// slugify converts an arbitrary string (file-path component, instance name,
// etc.) into a lowercase, hyphen-separated identifier safe for use as a Docker
// network name suffix.
//
// Rules:
//   - Unicode letters/digits are lowercased and kept.
//   - Runs of separator characters (spaces, underscores, slashes, dots, …) are
//     collapsed to a single hyphen.
//   - Leading and trailing hyphens are stripped.
//   - The result is truncated to 48 characters so that the full network name
//     stays well within Docker's 64-character limit.
func slugify(s string) string {
	// Replace common path separators and punctuation with a space so they
	// become hyphen boundaries after the fold step below.
	replacer := strings.NewReplacer(
		"/", " ",
		"\\", " ",
		"_", " ",
		".", " ",
		"@", " ",
	)
	s = replacer.Replace(s)

	// Fold to lowercase, keeping only letters and digits from Unicode.
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(' ')
		}
	}
	s = b.String()

	// Collapse whitespace runs to hyphens.
	s = strings.Join(strings.Fields(s), "-")

	// Remove any remaining non-alnum/hyphen characters (defensive).
	s = nonAlnum.ReplaceAllString(s, "")

	// Strip leading/trailing hyphens.
	s = strings.Trim(s, "-")

	// Truncate to 48 chars to keep the full network name under 64 chars.
	if len(s) > 48 {
		s = s[:48]
		s = strings.TrimRight(s, "-")
	}

	return s
}
