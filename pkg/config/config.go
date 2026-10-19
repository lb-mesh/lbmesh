// Package config handles reading and writing the lbmesh configuration file
// located at ~/.lbmesh/config.yaml.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
)

// ---------------------------------------------------------------------------
// Config schema
// ---------------------------------------------------------------------------

// Config is the top-level structure for ~/.lbmesh/config.yaml.
type Config struct {
	// Core holds general lbmesh behaviour settings.
	Core CoreConfig `yaml:"core"`

	// AI holds optional LLM provider settings for `lbmesh ask`.
	AI AIConfig `yaml:"ai,omitempty"`
}

// CoreConfig contains general operational settings.
type CoreConfig struct {
	// DefaultScope is the scope used when --scope is not provided (project|system|custom).
	DefaultScope string `yaml:"default_scope"`

	// DashboardPort is the default port for the embedded HTTP dashboard.
	DashboardPort int `yaml:"dashboard_port"`

	// DashboardHost is the default bind address for the embedded HTTP dashboard.
	DashboardHost string `yaml:"dashboard_host"`
}

// AIConfig holds connection details for the LLM provider.
type AIConfig struct {
	// Provider selects the backend: ollama | openai | anthropic | litellm.
	Provider string `yaml:"provider,omitempty"`

	// BaseURL is the API endpoint (e.g. http://localhost:4000 for LiteLLM).
	BaseURL string `yaml:"base_url,omitempty"`

	// APIKey is the authentication token / virtual key.
	APIKey string `yaml:"api_key,omitempty"`

	// Model specifies the model tag or name-prefix mapping (e.g. "gemini/gemini-1.5-pro").
	Model string `yaml:"model,omitempty"`
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

// defaults returns a Config pre-populated with sensible out-of-the-box values.
func defaults() Config {
	return Config{
		Core: CoreConfig{
			DefaultScope:  "project",
			DashboardPort: 8080,
			DashboardHost: "127.0.0.1",
		},
	}
}

// ---------------------------------------------------------------------------
// File path helpers
// ---------------------------------------------------------------------------

// configPath returns the absolute path to ~/.lbmesh/config.yaml.
func configPath() (string, error) {
	root, err := dirs.LocalRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.yaml"), nil
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Load reads ~/.lbmesh/config.yaml and returns the parsed Config.
// If the file does not exist, the default Config is returned without error so
// that first-run situations are handled gracefully.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := defaults()
			return &cfg, nil
		}
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := defaults()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return &cfg, nil
}

// Save marshals cfg and writes it atomically to ~/.lbmesh/config.yaml.
// The parent directory must already exist (EnsureLocalDirs covers this).
func Save(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	// Write to a temp file then rename for atomic replacement.
	tmp := path + ".tmp"
	// 0600: the file can hold LLM provider API keys.
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing temp config %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replacing config file: %w", err)
	}

	return nil
}

// EnsureDefaults writes the default config.yaml only if it does not already
// exist.  This is idempotent and safe to call on every `lbmesh init`.
func EnsureDefaults() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(path); err == nil {
		// File already exists — leave it untouched.
		return nil
	}

	cfg := defaults()
	return Save(&cfg)
}
