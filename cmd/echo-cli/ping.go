package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	openaimodel "echo-cli/internal/agent/openai"
	"echo-cli/internal/auth"
	"echo-cli/internal/config"
)

func pingMain(root rootArgs, args []string) {
	if err := runPing(root, args, os.Stdout); err != nil {
		log.Fatalf("ping failed: %v", err)
	}
}

func runPing(root rootArgs, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfgPath string
	var providerOverride string
	var modelOverride string
	var baseURLOverride string
	var apiKeyOverride string
	var timeoutSeconds int

	fs.StringVar(&cfgPath, "config", "", "Path to config file (default ~/.echo/config.toml)")
	fs.StringVar(&providerOverride, "provider", "", "Provider name (default from config)")
	fs.StringVar(&modelOverride, "model", "", "Model name (default from config)")
	fs.StringVar(&baseURLOverride, "base-url", "", "Override base URL (e.g. http://127.0.0.1:1234/v1)")
	fs.StringVar(&apiKeyOverride, "api-key", "", "Override API key (prefer config.toml)")
	fs.IntVar(&timeoutSeconds, "timeout", 0, "Timeout seconds (default from config)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	cfg = config.ApplyKVOverrides(cfg, prependOverrides(root.overrides, nil))

	provider := strings.ToLower(strings.TrimSpace(providerOverride))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(cfg.ModelProvider))
	}
	if provider == "" {
		provider = "openai"
	}

	model := strings.TrimSpace(modelOverride)
	if model == "" {
		model = strings.TrimSpace(cfg.Model)
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	baseURL := strings.TrimSpace(baseURLOverride)
	if baseURL == "" {
		settings := resolveProviderSettings(cfg, provider)
		baseURL = strings.TrimSpace(settings.BaseURL)
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(baseURLFromHostPort(providerEntryFromConfig(cfg, provider)))
	}
	if baseURL != "" && !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}

	apiKey := strings.TrimSpace(apiKeyOverride)
	if apiKey == "" {
		apiKey = strings.TrimSpace(apiKeyFromEntry(providerEntryFromConfig(cfg, provider)))
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if apiKey == "" {
		key, err := auth.LoadAPIKey()
		if err != nil {
			return fmt.Errorf("load ~/.echo/auth.json: %w", err)
		}
		apiKey = strings.TrimSpace(key)
	}
	if apiKey == "" {
		return errors.New("missing api key: configure model_providers.<provider>.api_key, or run `echo-cli login --with-api-key`")
	}

	if timeoutSeconds <= 0 {
		timeoutSeconds = cfg.RequestTimeout
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	if err := openaimodel.CheckBaseURLReachable(ctx, baseURL); err != nil {
		return err
	}
	got, err := openaimodel.CheckResponsesEndpoint(ctx, baseURL, apiKey, model)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "ok: %s\n", got)
	return nil
}

func providerEntryFromConfig(cfg config.Config, provider string) any {
	if cfg.Raw == nil {
		return nil
	}
	rawProviders, ok := cfg.Raw["model_providers"]
	if !ok {
		return nil
	}
	mp, ok := rawProviders.(map[string]any)
	if !ok {
		return nil
	}
	entry, ok := mp[provider]
	if ok {
		return entry
	}
	return nil
}

func apiKeyFromEntry(entry any) string {
	m, ok := entry.(map[string]any)
	if !ok {
		return ""
	}
	for _, k := range []string{"api_key", "api-key", "key", "token"} {
		if val, ok := m[k]; ok {
			if s, ok := val.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func baseURLFromHostPort(entry any) string {
	m, ok := entry.(map[string]any)
	if !ok {
		return ""
	}
	if addr, ok := m["address"]; ok {
		if s, ok := addr.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				if strings.Contains(s, "://") {
					return s
				}
				return "http://" + s
			}
		}
	}

	port, ok := intFromAny(m["port"])
	if !ok || port <= 0 {
		return ""
	}
	host := "127.0.0.1"
	if val, ok := m["host"]; ok {
		if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
			host = strings.TrimSpace(s)
		}
	}
	scheme := "http"
	if val, ok := m["scheme"]; ok {
		if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
			scheme = strings.ToLower(strings.TrimSpace(s))
		}
	}
	pathPrefix := ""
	for _, k := range []string{"path_prefix", "path-prefix", "path"} {
		if val, ok := m[k]; ok {
			if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
				pathPrefix = strings.TrimSpace(s)
				break
			}
		}
	}

	base := fmt.Sprintf("%s://%s:%d", scheme, host, port)
	if pathPrefix != "" {
		base = strings.TrimRight(base, "/") + "/" + strings.Trim(pathPrefix, "/")
	}
	return base
}

func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		n = strings.TrimSpace(n)
		if n == "" {
			return 0, false
		}
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}
