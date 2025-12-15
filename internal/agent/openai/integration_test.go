package openai

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"echo-cli/internal/agent"
	"echo-cli/internal/auth"
	"echo-cli/internal/config"
)

func TestIntegration_EchoConfig_Complete_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if strings.TrimSpace(os.Getenv("ECHO_CLI_OPENAI_INTEGRATION")) != "1" {
		t.Skip("set ECHO_CLI_OPENAI_INTEGRATION=1 to enable this integration test")
	}

	silenceRootLogger(t)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load() error: %v", err)
	}

	apiKey, err := auth.LoadAPIKey()
	if err != nil {
		t.Skipf("auth.LoadAPIKey() error (run `echo-cli login --with-api-key`): %v", err)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		t.Skip("empty API key (run `echo-cli login --with-api-key`)")
	}

	providerName := strings.ToLower(strings.TrimSpace(cfg.ModelProvider))
	if providerName == "" {
		providerName = "openai"
	}
	if providerName != "openai" && !strings.HasPrefix(providerName, "openai:") {
		t.Skipf("config model_provider=%q; this integration test expects openai-compatible provider", providerName)
	}

	baseURL, wire := resolveProviderSettingsForTest(cfg, providerName)
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}

	client, err := New(Options{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
		WireAPI: wire,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	got, err := client.Complete(ctx, agent.Prompt{
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "请严格只输出 ping（全小写），不要任何其他字符（不要标点、不要换行）。"},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error (base_url=%q wire_api=%q model=%q): %v", baseURL, wire, model, err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatalf("Complete() returned empty text")
	}
	if strings.ToLower(strings.TrimSpace(got)) != "ping" {
		t.Fatalf("Complete() = %q, want %q", got, "ping")
	}
}

func resolveProviderSettingsForTest(cfg config.Config, provider string) (baseURL string, wireAPI string) {
	// 与 echo-cli 运行时逻辑对齐：
	// 1) 优先读取 OPENAI_BASE_URL
	// 2) 其次读取 config.toml 的 model_providers.<provider>.base_url
	// 3) wire_api 固定为 responses（强制使用 /responses，不做 fallback）
	wireAPI = "responses"
	if env := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")); env != "" {
		baseURL = env
	}

	if cfg.Raw != nil {
		if rawProviders, ok := cfg.Raw["model_providers"]; ok {
			if mp, ok := rawProviders.(map[string]any); ok {
				if entry, ok := mp[provider]; ok {
					if base := baseURLFromEntryForTest(entry); base != "" && strings.TrimSpace(baseURL) == "" {
						baseURL = base
					}
				}
			}
		}
	}

	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return normalizeBaseURL(baseURL), wireAPI
}

func baseURLFromEntryForTest(entry any) string {
	m, ok := entry.(map[string]any)
	if !ok {
		return ""
	}
	if val, ok := m["base_url"]; ok {
		if s, ok := val.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	if val, ok := m["base-url"]; ok {
		if s, ok := val.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// wire_api 配置在 echo-cli 中已不再支持（固定 responses）。
