package config

import (
	"errors"
	"os"
	"path/filepath"

	"echo-cli/internal/features"
	"echo-cli/internal/i18n"

	"github.com/pelletier/go-toml/v2"
)

// Config mirrors the minimal fields we need for M1; expand as milestones grow.
type Config struct {
	Model           string          `toml:"model"`
	ModelProvider   string          `toml:"model_provider"`
	SandboxMode     string          `toml:"sandbox_mode"`
	ApprovalPolicy  string          `toml:"approval_policy"`
	DefaultLanguage string          `toml:"default_language"`
	ReasoningEffort string          `toml:"reasoning_effort"`
	RequestTimeout  int             `toml:"request_timeout_seconds"`
	Retries         int             `toml:"retries"`
	WorkspaceDirs   []string        `toml:"workspace_dirs"`
	Features        map[string]bool `toml:"features"`
	ConfigProfile   string          `toml:"config_profile"`
	Raw             map[string]any  `toml:"-"`
	Source          string          `toml:"-"`
}

type Overrides struct {
	Model           string
	ModelProvider   string
	ReasoningEffort string
	RequestTimeout  int
	Retries         int
	WorkspaceDirs   []string
	Workdir         string
	Path            string
	ConfigProfile   string
	DefaultLanguage string
}

func Default() Config {
	defaultFeatures := map[string]bool{}
	for _, spec := range features.Specs {
		defaultFeatures[spec.Key] = spec.DefaultEnabled
	}
	return Config{
		Model:           "gpt-4o-mini",
		ModelProvider:   "openai",
		SandboxMode:     "read-only",
		ApprovalPolicy:  "on-request",
		DefaultLanguage: i18n.DefaultLanguage.Code(),
		ReasoningEffort: "",
		RequestTimeout:  120,
		Retries:         0,
		Features:        defaultFeatures,
	}
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

	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := toml.Unmarshal(content, &cfg); err != nil {
		return cfg, err
	}
	var raw map[string]any
	if err := toml.Unmarshal(content, &raw); err == nil {
		cfg.Raw = raw
	}
	cfg.Source = path
	if cfg.Features == nil {
		cfg.Features = map[string]bool{}
	}
	for _, spec := range features.Specs {
		if _, ok := cfg.Features[spec.Key]; !ok {
			cfg.Features[spec.Key] = spec.DefaultEnabled
		}
	}
	cfg.DefaultLanguage = i18n.Normalize(cfg.DefaultLanguage).Code()
	return cfg, nil
}

func ApplyOverrides(cfg Config, overrides Overrides) Config {
	if overrides.Model != "" {
		cfg.Model = overrides.Model
	}
	if overrides.ModelProvider != "" {
		cfg.ModelProvider = overrides.ModelProvider
	}
	if overrides.ReasoningEffort != "" {
		cfg.ReasoningEffort = overrides.ReasoningEffort
	}
	if overrides.RequestTimeout > 0 {
		cfg.RequestTimeout = overrides.RequestTimeout
	}
	if len(overrides.WorkspaceDirs) > 0 {
		cfg.WorkspaceDirs = overrides.WorkspaceDirs
	}
	if overrides.Retries > 0 {
		cfg.Retries = overrides.Retries
	}
	if overrides.ConfigProfile != "" {
		cfg.ConfigProfile = overrides.ConfigProfile
	}
	if overrides.DefaultLanguage != "" {
		cfg.DefaultLanguage = i18n.Normalize(overrides.DefaultLanguage).Code()
	}
	return cfg
}
