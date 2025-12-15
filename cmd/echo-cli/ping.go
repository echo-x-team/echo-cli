package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"echo-cli/internal/agent"
	anthropicmodel "echo-cli/internal/agent/anthropic"
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
	fs.StringVar(&baseURLOverride, "base-url", "", "Override base URL (e.g. http://127.0.0.1:1234; trailing /v1 is ok)")
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

	if strings.TrimSpace(providerOverride) != "" {
		log.Warnf("provider override %q is ignored; echo-cli now configures only url/token", providerOverride)
	}

	model := strings.TrimSpace(modelOverride)
	if model == "" {
		model = defaultRuntimeConfig().Model
	}

	baseURL := strings.TrimSpace(baseURLOverride)
	if baseURL == "" {
		baseURL = strings.TrimSpace(cfg.URL)
	}
	if baseURL == "" {
		return errors.New("missing url: set ANTHROPIC_BASE_URL or configure url in ~/.echo/config.toml")
	}

	apiKey := strings.TrimSpace(apiKeyOverride)
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.Token)
	}
	if apiKey == "" {
		return errors.New("missing token: set ANTHROPIC_AUTH_TOKEN or configure token in ~/.echo/config.toml")
	}

	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	client, err := anthropicmodel.New(anthropicmodel.Options{
		Token:   apiKey,
		BaseURL: baseURL,
		Model:   model,
	})
	if err != nil {
		return fmt.Errorf("init anthropic client: %w", err)
	}
	got, err := client.Complete(ctx, agent.Prompt{
		Model: model,
		Messages: []agent.Message{
			{Role: agent.RoleSystem, Content: "请严格只输出 pong（全小写），不要任何其他字符（不要标点、不要换行）。"},
			{Role: agent.RoleUser, Content: "ping"},
		},
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "ok: %s\n", got)
	return nil
}
