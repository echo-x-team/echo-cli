package main

import (
	"strconv"
	"strings"

	"echo-cli/internal/i18n"
)

type runtimeConfig struct {
	Model              string
	SandboxMode        string
	ApprovalPolicy     string
	DefaultLanguage    string
	ReasoningEffort    string
	RequestTimeoutSecs int
	Retries            int
}

func defaultRuntimeConfig() runtimeConfig {
	return runtimeConfig{
		Model:              "claude-3-5-sonnet-20240620",
		SandboxMode:        "read-only",
		ApprovalPolicy:     "on-request",
		DefaultLanguage:    i18n.DefaultLanguage.Code(),
		ReasoningEffort:    "",
		RequestTimeoutSecs: 120,
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
		case "sandbox_mode", "sandbox":
			cfg.SandboxMode = val
		case "approval_policy", "ask_for_approval", "ask-for-approval":
			cfg.ApprovalPolicy = val
		case "reasoning_effort", "reasoning-effort":
			cfg.ReasoningEffort = val
		case "default_language", "language":
			cfg.DefaultLanguage = i18n.Normalize(val).Code()
		case "request_timeout_seconds", "timeout":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.RequestTimeoutSecs = n
			}
		case "retries":
			if n, err := strconv.Atoi(val); err == nil && n >= 0 {
				cfg.Retries = n
			}
		}
	}
	return cfg
}
