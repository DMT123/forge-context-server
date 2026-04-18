// Package config loads runtime configuration from YAML.
package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the full server configuration loaded from YAML.
type Config struct {
	Server  ServerConfig   `yaml:"server"`
	Sources []SourceConfig `yaml:"sources"`
	Logging LoggingConfig  `yaml:"logging"`
}

// ServerConfig is the transport layer config.
type ServerConfig struct {
	Transport string `yaml:"transport"` // "stdio" | "http" | "sse"
	Host      string `yaml:"host"`      // http only
	Port      int    `yaml:"port"`      // http only
	Name      string `yaml:"name"`      // MCP implementation name
	Version   string `yaml:"version"`   // MCP implementation version
}

// SourceConfig describes a single source to mount.
type SourceConfig struct {
	Name    string            `yaml:"name"`    // e.g. "workspace-main", "obsidian-openclaw"
	Type    string            `yaml:"type"`    // "workspace" | "obsidian" | "github" | ...
	Enabled bool              `yaml:"enabled"`
	Options map[string]string `yaml:"options"` // type-specific options (root path, URL, token, etc.)
}

// LoggingConfig controls logging verbosity and destination.
type LoggingConfig struct {
	Level  string `yaml:"level"` // debug | info | warn | error
	Format string `yaml:"format"` // text | json
}

// Load reads and parses a YAML config file.
func Load(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("no config path provided")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	// Sensible defaults
	if cfg.Server.Name == "" {
		cfg.Server.Name = "davzy-vault"
	}
	if cfg.Server.Version == "" {
		cfg.Server.Version = "0.1.0-dev"
	}
	if cfg.Server.Transport == "" {
		cfg.Server.Transport = "stdio"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "text"
	}
	return &cfg, nil
}
