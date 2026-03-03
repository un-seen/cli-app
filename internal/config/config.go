package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the build-time configuration read from config.yaml.
type Config struct {
	BinaryName string            `yaml:"binary_name"`
	Version    string            `yaml:"version"`
	Auth       AuthConfig        `yaml:"auth"`
	Specs      map[string]string `yaml:"specs"` // name → URL or file path
}

type AuthConfig struct {
	EnvVar string `yaml:"env_var"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.BinaryName == "" {
		return nil, fmt.Errorf("binary_name is required in config")
	}
	if cfg.Auth.EnvVar == "" {
		return nil, fmt.Errorf("auth.env_var is required in config")
	}
	if len(cfg.Specs) == 0 {
		return nil, fmt.Errorf("at least one spec must be defined in config")
	}
	if len(cfg.Version) > 20 {
		return nil, fmt.Errorf("version must be at most 20 characters")
	}

	return &cfg, nil
}
