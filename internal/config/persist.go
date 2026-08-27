package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

type oauthFileConfig struct {
	BaseURL        string `toml:"JIRA_BASE_URL"`
	DefaultProject string `toml:"DEFAULT_PROJECT,omitempty"`
	DefaultUser    string `toml:"DEFAULT_USER,omitempty"`
}

// Save validates and atomically persists interactive configuration to the canonical path.
func Save(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	path, err := DefaultPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating configuration directory: %w", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("securing configuration directory: %w", err)
		}
	}

	tmp, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return fmt.Errorf("creating temporary configuration file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("securing temporary configuration file: %w", err)
	}
	persisted := oauthFileConfig{
		BaseURL:        cfg.BaseURL,
		DefaultProject: cfg.DefaultProject,
		DefaultUser:    cfg.DefaultUser,
	}
	if err := toml.NewEncoder(tmp).Encode(persisted); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encoding configuration: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary configuration file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("persisting configuration: %w", err)
	}
	return nil
}
