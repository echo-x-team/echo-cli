package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the only persisted config file schema.
type Config struct {
	URL    string `toml:"url"`
	Token  string `toml:"token"`
	Source string `toml:"-"`
}

func Default() Config {
	return Config{}
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".echo", "config.toml")
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultPath()
	}
	if path == "" {
		return cfg, errors.New("config path is empty and $HOME is not set")
	}
	cfg.Source = path

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if env := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")); env != "" {
				cfg.URL = env
			}
			if env := strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN")); env != "" {
				cfg.Token = env
			}
			return cfg, nil
		}
		return cfg, err
	}

	if err := toml.Unmarshal(content, &cfg); err != nil {
		return cfg, err
	}
	if env := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")); env != "" {
		cfg.URL = env
	}
	if env := strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN")); env != "" {
		cfg.Token = env
	}
	return cfg, nil
}
