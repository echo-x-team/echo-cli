package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"echo-cli/internal/agent"
	openaimodel "echo-cli/internal/agent/openai"
	"echo-cli/internal/auth"
	"echo-cli/internal/config"
	"echo-cli/internal/events"
	"echo-cli/internal/execution"
	"echo-cli/internal/instructions"
	"echo-cli/internal/logger"
	"echo-cli/internal/policy"
	"echo-cli/internal/repl"
	"echo-cli/internal/sandbox"
	"echo-cli/internal/session"
	"echo-cli/internal/tools/dispatcher"
	toolengine "echo-cli/internal/tools/engine"
	"github.com/google/uuid"
)

func main() {
	logger.Configure()
	if logFile, _, err := logger.SetupFile(logger.DefaultLogPath); err != nil {
		log.Warnf("failed to initialize log file: %v", err)
	} else {
		defer logFile.Close()
	}
	logger.SetGlobalLLMLogger(logger.NewLLMLogger(logger.Root()))

	root, rest, err := parseRootArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("parse args: %v", err)
	}
	if len(rest) > 0 {
		switch rest[0] {
		case "exec":
			execMain(root, rest[1:])
			return
		case "completion":
			completionMain(rest[1:])
			return
		case "resume":
			resumeMain(root, rest[1:])
			return
		case "review":
			reviewMain(root, rest[1:])
			return
		case "login":
			loginMain(root, rest[1:])
			return
		case "logout":
			logoutMain(root, rest[1:])
			return
		case "apply":
			applyMain(root, rest[1:])
			return
		case "sandbox":
			sandboxMain(root, rest[1:])
			return
		case "execpolicy":
			execpolicyMain(root, rest[1:])
			return
		case "mcp":
			mcpMain(root, rest[1:])
			return
		case "mcp-server":
			mcpServerMain(root, rest[1:])
			return
		case "cloud", "cloud-tasks":
			cloudMain(root, rest[1:])
			return
		case "responses-proxy":
			responsesProxyMain(root, rest[1:])
			return
		case "stdio-to-uds":
			stdioToUDSMain(root, rest[1:])
			return
		case "features":
			featuresMain(root, rest[1:])
			return
		}
	}

	runInteractive(root, rest)
}

func runInteractive(root rootArgs, args []string) {
	fs, cli := newInteractiveFlagSet("echo-cli")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse args: %v", err)
	}
	cli.finalizePrompt(fs)

	cli.configOverrides = stringSlice(prependOverrides(root.overrides, []string(cli.configOverrides)))
	startInteractiveSession(cli, nil)
}

func startInteractiveSession(cli *interactiveArgs, seedMessages []agent.Message) {
	cfg, err := config.Load(cli.cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	cfg = config.ApplyOverrides(cfg, config.Overrides{
		Model:         cli.modelOverride,
		Path:          cli.cfgPath,
		ConfigProfile: cli.configProfile,
	})
	cfg = config.ApplyKVOverrides(cfg, []string(cli.configOverrides))
	if cli.configProfile != "" {
		cfg.ConfigProfile = cli.configProfile
	}
	if cli.oss {
		cfg = applyOSSBootstrap(cfg, cli.localProvider)
	}
	if !cli.oss && cli.localProvider != "" {
		cfg.ModelProvider = cli.localProvider
	}
	if cli.sandboxMode != "" {
		cfg.SandboxMode = cli.sandboxMode
	}
	if cli.fullAuto {
		cfg.SandboxMode = "workspace-write"
		if cfg.ApprovalPolicy == "" {
			cfg.ApprovalPolicy = "on-request"
		}
	}
	if cli.dangerouslyBypass {
		cfg.SandboxMode = "danger-full-access"
		cfg.ApprovalPolicy = "never"
	}
	if cli.askApproval != "" {
		cfg.ApprovalPolicy = cli.askApproval
	}
	if cli.search {
		if cfg.Features == nil {
			cfg.Features = map[string]bool{}
		}
		cfg.Features["web_search_request"] = true
	}

	workdir := resolveWorkdir(cli.workdir)
	if len(seedMessages) == 0 && cli.resumeSessionID != "" {
		if rec, err := session.Load(cli.resumeSessionID); err == nil {
			seedMessages = append(seedMessages, rec.Messages...)
			if cli.resumeSessionID == "" {
				cli.resumeSessionID = rec.ID
			}
		}
	} else if len(seedMessages) == 0 && cli.resumeLast {
		if rec, err := session.Last(); err == nil {
			seedMessages = append(seedMessages, rec.Messages...)
			if cli.resumeSessionID == "" {
				cli.resumeSessionID = rec.ID
			}
		}
	}

	resumeIDs := []string(nil)
	if cli.resumePicker {
		records, err := session.List(cli.resumeShowAll, workdir)
		if err != nil {
			log.Fatalf("failed to load sessions: %v", err)
		}
		for _, rec := range records {
			resumeIDs = append(resumeIDs, rec.ID)
		}
		if len(resumeIDs) == 0 {
			log.Info("no sessions available to resume; starting new chat")
			cli.resumePicker = false
		}
	}

	client := buildModelClient(cfg)
	system := instructions.Discover(workdir)
	bus := events.NewBus()
	defer bus.Close()
	pol := policy.Policy{SandboxMode: cfg.SandboxMode, ApprovalPolicy: cfg.ApprovalPolicy}
	roots := append([]string{}, cfg.WorkspaceDirs...)
	roots = append(roots, workdir)
	roots = append(roots, []string(cli.addDirs)...)
	runner := sandbox.NewRunner(cfg.SandboxMode, roots...)
	approver := toolengine.NewUIApprover()
	disp := dispatcher.New(pol, runner, bus, workdir, approver)
	disp.Start(context.Background())

	manager := events.NewManager(events.ManagerConfig{})
	toolTimeout := time.Duration(cfg.RequestTimeout) * time.Second
	if toolTimeout == 0 {
		toolTimeout = 2 * time.Minute
	}
	engine := execution.NewEngine(execution.Options{
		Manager:        manager,
		Client:         client,
		Bus:            bus,
		Defaults:       execution.SessionDefaults{Model: cfg.Model, System: system, ReasoningEffort: cfg.ReasoningEffort, Language: cfg.DefaultLanguage},
		ToolTimeout:    toolTimeout,
		RequestTimeout: time.Duration(cfg.RequestTimeout) * time.Second,
		Retries:        cfg.Retries,
	})
	engine.Start(context.Background())
	defer engine.Close()
	gateway := repl.NewGateway(manager)

	seedSessionID := cli.resumeSessionID
	if seedSessionID == "" && len(seedMessages) > 0 {
		seedSessionID = uuid.NewString()
	}
	if seedSessionID != "" && len(seedMessages) > 0 {
		engine.SeedHistory(seedSessionID, seedMessages)
	}

	attachments := append([]agent.Message{}, seedMessages...)
	attachments = append(attachments, loadImageAttachments([]string(cli.imagePaths), workdir)...)
	uiResult, err := repl.RunUI(repl.UIOptions{
		Engine:          engine,
		Gateway:         gateway,
		Model:           cfg.Model,
		Reasoning:       cfg.ReasoningEffort,
		Sandbox:         cfg.SandboxMode,
		Workdir:         workdir,
		InitialPrompt:   cli.prompt,
		Language:        cfg.DefaultLanguage,
		InitialMessages: attachments,
		Policy:          pol,
		Events:          bus,
		Runner:          runner,
		Approver:        approver,
		Roots:           roots,
		ResumePicker:    cli.resumePicker,
		ResumeShowAll:   cli.resumeShowAll,
		ResumeSessions:  resumeIDs,
		ResumeSessionID: seedSessionID,
	})
	if err != nil {
		log.Fatalf("program exit: %v", err)
	}
	history := uiResult.History
	if len(history) == 0 {
		return
	}
	sessionID := cli.resumeSessionID
	if id := uiResult.SessionID; id != "" {
		sessionID = id
	}
	savedID, err := session.Save(sessionID, workdir, history)
	if err != nil {
		log.Warnf("failed to save session: %v", err)
		return
	}
	if action := uiResult.UpdateAction; action != "" {
		if err := runUpdateAction(action); err != nil {
			log.Warnf("update action failed: %v", err)
		}
	}
	usage := estimateUsage(history)
	printExitSummary(savedID, usage)
}

func buildModelClient(cfg config.Config) agent.ModelClient {
	apiKey, err := auth.LoadAPIKey()
	if err != nil {
		log.Fatalf("failed to load ~/.echo/auth.json: %v", err)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		log.Fatalf("no API key found in ~/.echo/auth.json; run `echo-cli login --with-api-key` to configure")
	}
	provider := strings.ToLower(cfg.ModelProvider)
	if provider == "" {
		provider = "openai"
	}
	settings := resolveProviderSettings(cfg, provider)
	switch {
	case provider == "openai":
		client, err := openaimodel.New(openaimodel.Options{
			APIKey:  apiKey,
			BaseURL: settings.BaseURL,
			Model:   cfg.Model,
			WireAPI: settings.WireAPI,
		})
		if err != nil {
			log.Fatalf("failed to init Echo Team client: %v", err)
		}
		return client
	case strings.HasPrefix(provider, "oss"):
		log.Info("OSS provider configured; using echo fallback (implement OSS client to enable responses).")
		return agent.EchoClient{Prefix: "assistant: "}
	default:
		client, err := openaimodel.New(openaimodel.Options{
			APIKey:  apiKey,
			BaseURL: settings.BaseURL,
			Model:   cfg.Model,
			WireAPI: settings.WireAPI,
		})
		if err != nil {
			log.Fatalf("failed to init client for provider %s: %v", provider, err)
		}
		return client
	}
}

type providerSettings struct {
	BaseURL string
	WireAPI string
}

func resolveProviderSettings(cfg config.Config, provider string) providerSettings {
	settings := providerSettings{WireAPI: "chat"}
	if env := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")); env != "" {
		settings.BaseURL = env
	}
	if cfg.Raw == nil {
		return settings
	}
	rawProviders, ok := cfg.Raw["model_providers"]
	if !ok {
		return settings
	}
	switch mp := rawProviders.(type) {
	case map[string]any:
		if entry, ok := mp[provider]; ok {
			if base := baseURLFromEntry(entry); base != "" && settings.BaseURL == "" {
				settings.BaseURL = base
			}
			if wire := wireAPIFromEntry(entry); wire != "" {
				settings.WireAPI = wire
			}
		}
	}
	return settings
}

func baseURLFromEntry(entry any) string {
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

func wireAPIFromEntry(entry any) string {
	m, ok := entry.(map[string]any)
	if !ok {
		return ""
	}
	if val, ok := m["wire_api"]; ok {
		if s, ok := val.(string); ok {
			return strings.ToLower(strings.TrimSpace(s))
		}
	}
	if val, ok := m["wire-api"]; ok {
		if s, ok := val.(string); ok {
			return strings.ToLower(strings.TrimSpace(s))
		}
	}
	return ""
}

func applyOSSBootstrap(cfg config.Config, providerOverride string) config.Config {
	provider := cfg.ModelProvider
	if providerOverride != "" {
		provider = "oss:" + providerOverride
	}
	if provider == "" || provider == "oss" {
		provider = "oss:lmstudio"
	}
	if !strings.HasPrefix(provider, "oss") {
		provider = "oss"
	}
	cfg.ModelProvider = provider
	if cfg.Model == "" {
		if def := defaultOSSModel(provider); def != "" {
			cfg.Model = def
		}
	}
	if cfg.Raw == nil {
		cfg.Raw = map[string]any{}
	}
	cfg.Raw["show_raw_agent_reasoning"] = true
	if err := ensureOSSProviderReady(provider); err != nil {
		log.Warnf("%v", err)
	}
	return cfg
}

func defaultOSSModel(provider string) string {
	switch {
	case strings.Contains(provider, "ollama"):
		return "llama3.1"
	case strings.Contains(provider, "lmstudio"):
		return "gpt-oss:20b"
	default:
		return "gpt-oss:20b"
	}
}

func ensureOSSProviderReady(provider string) error {
	switch {
	case strings.Contains(provider, "ollama"):
		if _, err := exec.LookPath("ollama"); err != nil {
			return fmt.Errorf("ollama provider selected but `ollama` binary not found in PATH")
		}
	case strings.Contains(provider, "lmstudio"):
		if _, err := exec.LookPath("lmstudio"); err != nil {
			if _, errAlt := exec.LookPath("lm-studio"); errAlt != nil {
				return fmt.Errorf("lmstudio provider selected but LM Studio CLI is not available in PATH")
			}
		}
	}
	return nil
}

type usageSummary struct {
	InputTokens  int64
	OutputTokens int64
}

func (u usageSummary) total() int64 {
	return u.InputTokens + u.OutputTokens
}

func estimateUsage(history []agent.Message) usageSummary {
	var u usageSummary
	for _, msg := range history {
		tokens := int64(len(strings.Fields(msg.Content)))
		if msg.Role == agent.RoleAssistant {
			u.OutputTokens += tokens
		} else {
			u.InputTokens += tokens
		}
	}
	return u
}

func printExitSummary(sessionID string, usage usageSummary) {
	lines := []string{}
	if usage.total() > 0 {
		lines = append(lines, fmt.Sprintf("Token usage: total=%d input=%d output=%d", usage.total(), usage.InputTokens, usage.OutputTokens))
	}
	if sessionID != "" {
		lines = append(lines, fmt.Sprintf("To continue this session, run echo-cli resume %s", sessionID))
	}
	for _, line := range lines {
		fmt.Println(line)
	}
}

func runUpdateAction(action string) error {
	cmd := exec.Command("bash", "-lc", action)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func resolveWorkdir(input string) string {
	if strings.TrimSpace(input) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		return wd
	}
	if filepath.IsAbs(input) {
		return input
	}
	wd, err := os.Getwd()
	if err != nil {
		return input
	}
	return filepath.Join(wd, input)
}
