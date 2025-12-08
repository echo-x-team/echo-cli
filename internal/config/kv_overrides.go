package config

import (
	"strconv"
	"strings"

	"echo-cli/internal/i18n"
)

// ApplyKVOverrides applies free-form -c key=value overrides.
// Only a small subset of keys are mapped to typed fields; all are stored in Raw.
func ApplyKVOverrides(cfg Config, overrides []string) Config {
	if len(overrides) == 0 {
		return cfg
	}
	if cfg.Raw == nil {
		cfg.Raw = map[string]any{}
	}
	for _, raw := range overrides {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		cfg.Raw[key] = val
		switch key {
		case "model":
			cfg.Model = val
		case "model_provider", "provider":
			cfg.ModelProvider = val
		case "sandbox_mode", "sandbox":
			cfg.SandboxMode = val
		case "approval_policy", "ask_for_approval":
			cfg.ApprovalPolicy = val
		case "reasoning_effort":
			cfg.ReasoningEffort = val
		case "config_profile", "profile":
			cfg.ConfigProfile = val
		case "default_language", "language":
			cfg.DefaultLanguage = i18n.Normalize(val).Code()
		}
		if strings.HasPrefix(key, "features.") {
			name := strings.TrimPrefix(key, "features.")
			if cfg.Features == nil {
				cfg.Features = map[string]bool{}
			}
			if b, err := strconv.ParseBool(val); err == nil {
				cfg.Features[name] = b
			}
		}
	}
	return cfg
}
