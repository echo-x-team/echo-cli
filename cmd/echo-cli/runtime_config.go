package main

import (
	"strconv"
	"strings"

	"echo-cli/internal/i18n"
)

type runtimeConfig struct {
	Model              string
	DefaultLanguage    string
	ReasoningEffort    string
	RequestTimeoutSecs int
	ToolTimeoutSecs    int
	Retries            int
}

func defaultRuntimeConfig() runtimeConfig {
	return runtimeConfig{
		Model:              "glm4.6",
		DefaultLanguage:    i18n.DefaultLanguage.Code(),
		ReasoningEffort:    "",
		RequestTimeoutSecs: 120,
		ToolTimeoutSecs:    600,
		Retries:            0,
	}
}

func applyRuntimeKVOverrides(cfg runtimeConfig, overrides []string) runtimeConfig {
	for _, raw := range overrides {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "model":
			cfg.Model = val
		case "reasoning_effort", "reasoning-effort":
			cfg.ReasoningEffort = val
		case "default_language", "language":
			cfg.DefaultLanguage = i18n.Normalize(val).Code()
		case "request_timeout_seconds", "timeout":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.RequestTimeoutSecs = n
			}
		case "tool_timeout_seconds", "tool_timeout", "tool-timeout-seconds", "tool-timeout":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.ToolTimeoutSecs = n
			}
		case "retries":
			if n, err := strconv.Atoi(val); err == nil && n >= 0 {
				cfg.Retries = n
			}
		}
	}
	return cfg
}
