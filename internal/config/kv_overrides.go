package config

import (
	"strings"
)

// ApplyKVOverrides applies free-form -c key=value overrides.
func ApplyKVOverrides(cfg Config, overrides []string) Config {
	if len(overrides) == 0 {
		return cfg
	}
	for _, raw := range overrides {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "url":
			cfg.URL = val
		case "token":
			cfg.Token = val
		case "model":
			cfg.Model = val
		}
	}
	return cfg
}
