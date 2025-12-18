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
	anthropicmodel "echo-cli/internal/agent/anthropic"
	"echo-cli/internal/config"
	"echo-cli/internal/events"
	"echo-cli/internal/execution"
	"echo-cli/internal/i18n"
	"echo-cli/internal/instructions"
	"echo-cli/internal/logger"
	"echo-cli/internal/repl"
	"echo-cli/internal/session"
	"echo-cli/internal/tools"
	"echo-cli/internal/tools/dispatcher"
	"github.com/google/uuid"
)

func main() {
	logger.Configure()
	if logFile, _, err := logger.SetupFile(logger.DefaultLogPath); err != nil {
		log.Warnf("failed to initialize log file: %v", err)
	} else {
		defer logFile.Close()
	}
	if toolsCloser, _, err := tools.SetupToolsLog(tools.DefaultToolsLogPath); err != nil {
		log.Warnf("failed to initialize tools log (%s): %v", tools.DefaultToolsLogPath, err)
	} else if toolsCloser != nil {
		defer toolsCloser.Close()
	}
	if errCloser, _, err := execution.SetupErrorLog(execution.DefaultErrorLogPath); err != nil {
		log.Warnf("failed to initialize error log (%s): %v", execution.DefaultErrorLogPath, err)
	} else if errCloser != nil {
		defer errCloser.Close()
	}
	if llmCloser, _, err := execution.SetupLLMLog(execution.DefaultLLMLogPath); err != nil {
		log.Warnf("failed to initialize llm log (%s): %v", execution.DefaultLLMLogPath, err)
	} else if llmCloser != nil {
		defer llmCloser.Close()
	}

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
		case "ping":
			pingMain(root, rest[1:])
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
	endpoint, err := config.Load(cli.cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	endpoint = config.ApplyKVOverrides(endpoint, []string(cli.configOverrides))

	rt := defaultRuntimeConfig()
	if strings.TrimSpace(cli.modelOverride) != "" {
		rt.Model = strings.TrimSpace(cli.modelOverride)
	}
	rt = applyRuntimeKVOverrides(rt, []string(cli.configOverrides))
	if strings.TrimSpace(rt.DefaultLanguage) == "" {
		rt.DefaultLanguage = i18n.DefaultLanguage.Code()
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

	client := buildModelClient(endpoint, rt.Model, cli.oss)
	system := instructions.Discover(workdir)
	bus := events.NewBus()
	defer bus.Close()
	conversationLog := logger.Named("conversation")
	if entry, closer, _, err := logger.SetupComponentFile("conversation", logger.DefaultConversationLogPath); err != nil {
		log.Warnf("failed to initialize conversation log (%s): %v", logger.DefaultConversationLogPath, err)
	} else {
		conversationLog = entry
		if closer != nil {
			defer closer.Close()
		}
	}
	runner := tools.DirectRunner{}
	reviewer := tools.NewLLMCommandReviewer(client, rt.Model)
	disp := dispatcher.New(runner, bus, workdir, dispatcher.Options{
		Reviewer: reviewer,
	})
	disp.Start(context.Background())

	manager := events.NewManager(events.ManagerConfig{})
	toolTimeout := time.Duration(rt.ToolTimeoutSecs) * time.Second
	if toolTimeout == 0 {
		toolTimeout = 10 * time.Minute
	}
	engine := execution.NewEngine(execution.Options{
		Manager:        manager,
		Client:         client,
		Bus:            bus,
		Defaults:       execution.SessionDefaults{Model: rt.Model, System: system, ReasoningEffort: rt.ReasoningEffort, Language: rt.DefaultLanguage},
		ToolTimeout:    toolTimeout,
		RequestTimeout: time.Duration(rt.RequestTimeoutSecs) * time.Second,
		Retries:        rt.Retries,
	})
	engine.Start(context.Background())
	defer engine.Close()
	gateway := repl.NewGateway(manager)

	seedSessionID := cli.resumeSessionID
	if seedSessionID == "" && len(seedMessages) > 0 {
		seedSessionID = uuid.NewString()
	}
	if seedSessionID != "" && len(seedMessages) > 0 {
		// 会话文件可能包含 role="tool" 的 UI 调试块；喂给模型前必须过滤。
		engine.SeedHistory(seedSessionID, extractConversationHistory(seedMessages))
	}

	attachments := append([]agent.Message{}, seedMessages...)
	attachments = append(attachments, loadImageAttachments([]string(cli.imagePaths), workdir)...)
	uiResult, err := repl.RunUI(repl.UIOptions{
		Engine:          engine,
		Gateway:         gateway,
		Model:           rt.Model,
		Reasoning:       rt.ReasoningEffort,
		Workdir:         workdir,
		InitialPrompt:   cli.prompt,
		Language:        rt.DefaultLanguage,
		InitialMessages: attachments,
		Events:          bus,
		Runner:          runner,
		ResumePicker:    cli.resumePicker,
		ResumeShowAll:   cli.resumeShowAll,
		ResumeSessions:  resumeIDs,
		ResumeSessionID: seedSessionID,
		ConversationLog: conversationLog,
		CopyableOutput:  cli.copyableOutput,
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

func buildModelClient(endpoint config.Config, model string, oss bool) agent.ModelClient {
	if oss {
		log.Info("OSS provider configured; using echo fallback (implement OSS client to enable responses).")
		return agent.EchoClient{Prefix: "assistant: "}
	}
	token := strings.TrimSpace(endpoint.Token)
	if token == "" {
		return agent.EchoClient{Prefix: "assistant: "}
	}
	if strings.TrimSpace(endpoint.URL) == "" {
		log.Warnf("empty url in config; falling back to echo mode")
		return agent.EchoClient{Prefix: "assistant: "}
	}
	client, err := anthropicmodel.New(anthropicmodel.Options{
		Token:   token,
		BaseURL: endpoint.URL,
		Model:   model,
	})
	if err != nil {
		log.Fatalf("failed to init anthropic client: %v", err)
	}
	return client
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
		if msg.Role != agent.RoleUser && msg.Role != agent.RoleAssistant {
			continue
		}
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
