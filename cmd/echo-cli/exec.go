package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"echo-cli/internal/agent"
	"echo-cli/internal/config"
	"echo-cli/internal/events"
	"echo-cli/internal/execution"
	"echo-cli/internal/instructions"
	"echo-cli/internal/repl"
	"echo-cli/internal/session"
	"echo-cli/internal/tools"
	"echo-cli/internal/tools/dispatcher"
	"echo-cli/internal/tui/slash"

	"github.com/google/uuid"
)

type jsonEvent struct {
	Type      string      `json:"type"`
	ThreadID  string      `json:"thread_id,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
	Item      *eventItem  `json:"item,omitempty"`
	Usage     *usage      `json:"usage,omitempty"`
	Error     *eventError `json:"error,omitempty"`
}

type eventItem struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status,omitempty"`
	Text     string `json:"text,omitempty"`
	Command  string `json:"command,omitempty"`
	Path     string `json:"path,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

type eventError struct {
	Message string `json:"message"`
}

type usage struct {
	InputTokens       int64 `json:"input_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
}

var encodeMu sync.Mutex

func execMain(root rootArgs, args []string) {
	subcommand := ""
	if len(args) > 0 {
		switch args[0] {
		case "resume", "review":
			subcommand = args[0]
			args = args[1:]
		}
	}

	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	var cfgPath string
	var modelOverride string
	var providerOverride string
	var attachPaths stringSlice
	var imagePaths csvSlice
	var configOverrides stringSlice
	var prompt string
	var sessionID string
	var resumeLast bool
	var listSessions bool
	var runCmd string
	var applyPatch string
	var reasoningOverride string
	var timeoutOverride int
	var retriesOverride int
	var configProfile string
	var oss bool
	var localProvider string
	var outputSchema string
	var colorMode string
	var jsonOutput bool
	var lastMessageFile string
	var workdir string
	var skipGitRepoCheck bool

	fs.StringVar(&cfgPath, "config", "", "Path to config file (default ~/.echo/config.toml)")
	fs.StringVar(&modelOverride, "model", "", "Model override")
	fs.StringVar(&modelOverride, "m", "", "Alias for --model")
	fs.StringVar(&providerOverride, "provider", "", "Model provider override")
	fs.StringVar(&workdir, "cd", "", "Working directory to display")
	fs.StringVar(&workdir, "C", "", "Alias for --cd")
	fs.Var(&attachPaths, "attach", "Attach a file into initial context (repeatable)")
	fs.Var(&imagePaths, "image", "Attach an image into initial context (repeatable)")
	fs.Var(&configOverrides, "c", "Override config value key=value (repeatable)")
	fs.StringVar(&reasoningOverride, "reasoning-effort", "", "Reasoning effort hint")
	fs.StringVar(&prompt, "prompt", "", "Prompt")
	fs.StringVar(&sessionID, "session", "", "Session id to resume")
	fs.BoolVar(&resumeLast, "resume-last", false, "Resume most recent session")
	fs.BoolVar(&listSessions, "list-sessions", false, "List saved session ids and exit")
	fs.StringVar(&runCmd, "run", "", "Optional command to run after reply (emits command events)")
	fs.StringVar(&applyPatch, "apply-patch", "", "Optional unified diff file to apply after reply (emits file change events)")
	fs.StringVar(&configProfile, "profile", "", "Config profile to use")
	fs.BoolVar(&oss, "oss", false, "Use open-source/local provider")
	fs.StringVar(&localProvider, "local-provider", "", "Local OSS provider (lmstudio|ollama)")
	fs.StringVar(&outputSchema, "output-schema", "", "Path to JSON Schema describing expected final output")
	fs.StringVar(&colorMode, "color", "auto", "Color output (auto|always|never)")
	fs.BoolVar(&jsonOutput, "json", false, "Print events to stdout as JSONL")
	fs.StringVar(&lastMessageFile, "output-last-message", "", "Write last assistant message to file")
	fs.IntVar(&timeoutOverride, "timeout", 0, "Request timeout seconds")
	fs.IntVar(&retriesOverride, "retries", 0, "Retry count on request failure")
	fs.BoolVar(&skipGitRepoCheck, "skip-git-repo-check", false, "Allow running outside a git repository (placeholder)")

	if err := fs.Parse(args); err != nil {
		log.Fatalf("parse exec args: %v", err)
	}
	configOverrides = stringSlice(prependOverrides(root.overrides, []string(configOverrides)))

	rest := fs.Args()
	if subcommand == "resume" && sessionID == "" && len(rest) > 0 {
		sessionID = rest[0]
		rest = rest[1:]
	}
	if prompt == "" && len(rest) > 0 {
		prompt = strings.Join(rest, " ")
	}
	if subcommand == "resume" && sessionID == "" && !resumeLast {
		resumeLast = true
	}

	if listSessions {
		ids, err := session.ListIDs()
		if err != nil {
			log.Fatalf("failed to list sessions: %v", err)
		}
		data, _ := json.Marshal(ids)
		fmt.Println(string(data))
		return
	}
	reviewMode := subcommand == "review"
	if prompt == "" && sessionID == "" && !resumeLast {
		log.Fatalf("prompt is required for exec unless resuming a session")
	}
	switch strings.ToLower(colorMode) {
	case "auto", "always", "never":
	default:
		log.Warnf("unknown color mode %q, defaulting to auto", colorMode)
		colorMode = "auto"
	}

	endpoint, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	endpoint = config.ApplyKVOverrides(endpoint, []string(configOverrides))

	rt := defaultRuntimeConfig()
	if strings.TrimSpace(modelOverride) != "" {
		rt.Model = strings.TrimSpace(modelOverride)
	}
	if strings.TrimSpace(reasoningOverride) != "" {
		rt.ReasoningEffort = strings.TrimSpace(reasoningOverride)
	}
	if timeoutOverride > 0 {
		rt.RequestTimeoutSecs = timeoutOverride
	}
	if retriesOverride > 0 {
		rt.Retries = retriesOverride
	}
	rt = applyRuntimeKVOverrides(rt, []string(configOverrides))
	if strings.TrimSpace(providerOverride) != "" {
		log.Warnf("provider override %q is ignored; echo-cli now configures only url/token", providerOverride)
	}
	if strings.TrimSpace(configProfile) != "" {
		log.Warnf("config profile %q is ignored; echo-cli now configures only url/token", configProfile)
	}
	if !oss && strings.TrimSpace(localProvider) != "" {
		log.Warnf("local-provider=%q ignored unless --oss is set", localProvider)
	}
	if skipGitRepoCheck {
		log.Info("skip-git-repo-check requested (no-op placeholder)")
	}

	workdir = resolveWorkdir(workdir)
	client := buildModelClient(endpoint, rt.Model, oss)
	system := instructions.Discover(workdir)
	outputSchemaContent := ""
	if outputSchema != "" {
		schemaPath := outputSchema
		if !filepath.IsAbs(schemaPath) && workdir != "" {
			schemaPath = filepath.Join(workdir, schemaPath)
		}
		if data, err := os.ReadFile(schemaPath); err != nil {
			log.Warnf("attachment read failed (%s): %v", outputSchema, err)
		} else {
			outputSchemaContent = string(data)
		}
	}
	runner := tools.DirectRunner{}
	bus := events.NewBus()
	defer bus.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	disp := dispatcher.New(runner, bus, workdir)
	disp.Start(ctx)

	emit := func(ev jsonEvent) {
		if jsonOutput {
			encode(ev)
			return
		}
		emitHuman(ev)
	}
	manager := events.NewManager(events.ManagerConfig{})
	toolTimeout := time.Duration(rt.ToolTimeoutSecs) * time.Second
	if toolTimeout == 0 {
		toolTimeout = 10 * time.Minute
	}
	engine := execution.NewEngine(execution.Options{
		Manager:        manager,
		Client:         client,
		Bus:            bus,
		Defaults:       execution.SessionDefaults{Model: rt.Model, System: system, OutputSchema: outputSchemaContent, ReasoningEffort: rt.ReasoningEffort, ReviewMode: reviewMode, Language: rt.DefaultLanguage},
		ToolTimeout:    toolTimeout,
		RequestTimeout: time.Duration(rt.RequestTimeoutSecs) * time.Second,
		Retries:        rt.Retries,
	})
	engine.Start(ctx)
	defer engine.Close()
	gateway := repl.NewGateway(manager)
	var eqRenderer *repl.EQRenderer
	if !jsonOutput {
		eqRenderer = repl.NewEQRenderer(repl.EQRendererOptions{
			SessionID: sessionID,
			Width:     80,
			Writer:    os.Stderr,
		})
	}

	// 提取纯对话历史（不包含系统注入的内容）
	history := []agent.Message{}
	if sessionID != "" {
		rec, err := session.Load(sessionID)
		if err != nil {
			log.Fatalf("failed to load session %s: %v", sessionID, err)
		}
		// 只提取对话历史，过滤掉系统注入的内容
		history = extractConversationHistory(rec.Messages)
	} else if resumeLast {
		rec, err := session.Last()
		if err != nil {
			log.Fatalf("failed to resume last session: %v", err)
		}
		// 只提取对话历史，过滤掉系统注入的内容
		history = extractConversationHistory(rec.Messages)
		sessionID = rec.ID
	}

	seedSessionID := sessionID
	if seedSessionID == "" && len(history) > 0 {
		seedSessionID = uuid.NewString()
	}
	if seedSessionID != "" && len(history) > 0 {
		engine.SeedHistory(seedSessionID, history)
		if sessionID == "" {
			sessionID = seedSessionID
		}
	}

	if sessionID == "" {
		sessionID = uuid.NewString()
	}

	threadID := sessionID
	if threadID == "" {
		threadID = fmt.Sprintf("thread-%d", time.Now().UnixNano())
	}

	emitEvent := func(ev jsonEvent) {
		if ev.ThreadID == "" {
			ev.ThreadID = threadID
		}
		if ev.SessionID == "" {
			ev.SessionID = sessionID
		}
		emit(ev)
	}

	// In JSON mode, mirror tool/approval events from the tool bus as JSONL too.
	// In human mode, tool events should be rendered via EQ so the output stays coherent.
	if jsonOutput {
		go forwardBusEvents(bus.Subscribe(), emitEvent)
	}

	// 准备附件内容
	attachments := []events.InputMessage{}
	attachments = append(attachments, attachmentMessages([]string(attachPaths), workdir)...)
	attachments = append(attachments, imageAttachmentMessages([]string(imagePaths), workdir)...)

	if strings.HasPrefix(strings.TrimSpace(prompt), "/") {
		action := repl.ResolveSlashAction(prompt, slash.Options{})
		switch action.Kind {
		case slash.ActionSubmitPrompt:
			if strings.TrimSpace(action.SubmitText) != "" {
				prompt = action.SubmitText
			}
		case slash.ActionSubmitCommand:
			log.Fatalf("slash command %s is not supported in exec mode; use interactive UI instead", action.Command)
		case slash.ActionError:
			log.Fatalf("%s", action.Message)
		case slash.ActionInsert:
			if strings.TrimSpace(action.NewValue) != "" {
				prompt = strings.TrimSpace(action.NewValue)
			}
		}
	}

	itemID := "item_0"
	summaryID := "summary_0"
	emitEvent(jsonEvent{Type: "thread.started"})

	engineEvents := gateway.Events()
	subID, err := gateway.SubmitUserInput(ctx, []events.InputMessage{
		{Role: "user", Content: prompt},
	}, events.InputContext{
		SessionID:       sessionID,
		Model:           rt.Model,
		System:          system,
		OutputSchema:    outputSchemaContent,
		Language:        rt.DefaultLanguage,
		ReasoningEffort: rt.ReasoningEffort,
		ReviewMode:      reviewMode,
		Attachments:     attachments,
	})
	if err != nil {
		emitEvent(jsonEvent{Type: "turn.failed", Error: &eventError{Message: err.Error()}})
		log.Fatalf("submit turn failed: %v", err)
	}

	var answerBuilder strings.Builder
	turnStarted := false
	answer := ""

	for done := false; !done; {
		select {
		case <-ctx.Done():
			emitEvent(jsonEvent{Type: "turn.failed", Error: &eventError{Message: "context canceled"}})
			log.Fatalf("exec cancelled")
		case ev := <-engineEvents:
			if ev.SubmissionID != subID {
				continue
			}
			if eqRenderer != nil {
				eqRenderer.Handle(ev)
			}
			switch ev.Type {
			case events.EventTaskStarted:
				if !turnStarted {
					emitEvent(jsonEvent{Type: "turn.started"})
					emitEvent(jsonEvent{Type: "item.started", Item: &eventItem{ID: itemID, Type: "agent_message", Status: "in_progress"}})
					turnStarted = true
				}
			case events.EventTaskSummary:
				summary, ok := ev.Payload.(events.TaskSummary)
				if !ok {
					continue
				}
				text := strings.TrimSpace(summary.Text)
				if text != "" {
					emitEvent(jsonEvent{Type: "item.completed", Item: &eventItem{ID: summaryID, Type: "task_summary", Status: "completed", Text: text}})
				}
			case events.EventAgentOutput:
				msg, ok := ev.Payload.(events.AgentOutput)
				if !ok {
					continue
				}
				if msg.Final {
					finalText := msg.Content
					if finalText == "" {
						finalText = answerBuilder.String()
					} else {
						answerBuilder.Reset()
						answerBuilder.WriteString(finalText)
					}
					answer = answerBuilder.String()
					if !turnStarted {
						emitEvent(jsonEvent{Type: "turn.started"})
						turnStarted = true
					}
					emitEvent(jsonEvent{Type: "item.completed", Item: &eventItem{ID: itemID, Type: "agent_message", Status: "completed", Text: answer}})
					continue
				}
				if msg.Content != "" {
					answerBuilder.WriteString(msg.Content)
					emitEvent(jsonEvent{Type: "item.updated", Item: &eventItem{ID: itemID, Type: "agent_message", Status: "in_progress", Text: msg.Content}})
				}
			case events.EventTaskCompleted:
				if answer == "" {
					answer = answerBuilder.String()
				}
				done = true
			case events.EventError:
				errMsg := fmt.Sprint(ev.Payload)
				emitEvent(jsonEvent{Type: "item.completed", Item: &eventItem{ID: itemID, Type: "agent_message", Status: "failed", Text: errMsg}})
				emitEvent(jsonEvent{Type: "turn.failed", Error: &eventError{Message: errMsg}})
				done = true
			}
		}
	}

	if answer != "" {
		history = append(history, agent.Message{Role: agent.RoleAssistant, Content: answer})
	}

	if runCmd != "" {
		cmdID := "cmd_0"
		emitEvent(jsonEvent{Type: "item.started", Item: &eventItem{ID: cmdID, Type: "command_execution", Status: "in_progress", Command: runCmd}})
		out, code, err := runner.RunCommand(context.Background(), workdir, runCmd)
		exitCode := code
		if err != nil {
			emitEvent(jsonEvent{Type: "item.completed", Item: &eventItem{ID: cmdID, Type: "command_execution", Status: "failed", Text: err.Error(), Command: runCmd, ExitCode: &exitCode}})
		} else {
			emitEvent(jsonEvent{Type: "item.completed", Item: &eventItem{ID: cmdID, Type: "command_execution", Status: "completed", Text: out, Command: runCmd, ExitCode: &exitCode}})
		}
	}

	if applyPatch != "" {
		patchID := "patch_0"
		data, err := os.ReadFile(applyPatch)
		if err != nil {
			emitEvent(jsonEvent{Type: "item.completed", Item: &eventItem{ID: patchID, Type: "file_change", Status: "failed", Text: err.Error(), Path: applyPatch}})
		} else {
			emitEvent(jsonEvent{Type: "item.started", Item: &eventItem{ID: patchID, Type: "file_change", Status: "in_progress", Path: applyPatch}})
			if err := runner.ApplyPatch(context.Background(), workdir, string(data)); err != nil {
				emitEvent(jsonEvent{Type: "item.completed", Item: &eventItem{ID: patchID, Type: "file_change", Status: "failed", Text: err.Error(), Path: applyPatch}})
			} else {
				emitEvent(jsonEvent{Type: "item.completed", Item: &eventItem{ID: patchID, Type: "file_change", Status: "completed", Path: applyPatch}})
			}
		}
	}

	turnUsage := calcUsage(history)
	emitEvent(jsonEvent{Type: "turn.completed", Usage: &turnUsage})

	if lastMessageFile != "" {
		if err := os.WriteFile(lastMessageFile, []byte(answer), 0o644); err != nil {
			log.Warnf("failed to write last message file: %v", err)
		}
	}

	savedID, err := session.Save(sessionID, workdir, history)
	if err != nil {
		log.Warnf("failed to save session: %v", err)
	} else {
		fmt.Fprintf(os.Stderr, "session saved: %s\n", savedID)
	}
	if jsonOutput {
		fmt.Fprintf(os.Stderr, "final: %s\n", answer)
	} else {
		fmt.Fprintln(os.Stdout, answer)
	}
}

func forwardBusEvents(ch <-chan any, emit func(jsonEvent)) {
	for evt := range ch {
		ev, ok := evt.(tools.ToolEvent)
		if !ok {
			continue
		}
		jsonEvt, ok := toolEventToJSON(ev)
		if !ok {
			continue
		}
		emit(jsonEvt)
	}
}

func toolEventToJSON(ev tools.ToolEvent) (jsonEvent, bool) {
	switch ev.Type {
	case "item.started", "item.updated", "item.completed":
		text := ev.Result.Output
		if text == "" {
			text = ev.Result.Status
		}
		status := ev.Result.Status
		if status == "" {
			switch ev.Type {
			case "item.started", "item.updated":
				status = "in_progress"
			default:
				status = "completed"
			}
		}
		item := eventItem{
			ID:      ev.Result.ID,
			Type:    string(ev.Result.Kind),
			Status:  status,
			Text:    text,
			Command: ev.Result.Command,
			Path:    ev.Result.Path,
		}
		if ev.Result.ExitCode != 0 {
			code := ev.Result.ExitCode
			item.ExitCode = &code
		}
		if ev.Result.Status == "error" || ev.Result.Error != "" {
			item.Text = ev.Result.Error
			item.Status = "failed"
		}
		return jsonEvent{Type: ev.Type, Item: &item}, true
	default:
		return jsonEvent{}, false
	}
}

func actionForKind(kind tools.ToolKind) string {
	switch kind {
	case tools.ToolCommand:
		return "command"
	case tools.ToolApplyPatch:
		return "file_change"
	case tools.ToolFileRead:
		return "file_read"
	case tools.ToolSearch:
		return "file_search"
	default:
		return string(kind)
	}
}

func calcUsage(messages []agent.Message) usage {
	var u usage
	for _, msg := range messages {
		tokens := int64(len(strings.Fields(msg.Content)))
		if msg.Role == agent.RoleAssistant {
			u.OutputTokens += tokens
		} else {
			u.InputTokens += tokens
		}
	}
	return u
}

func encode(ev jsonEvent) {
	encodeMu.Lock()
	defer encodeMu.Unlock()
	data, _ := json.Marshal(ev)
	fmt.Println(string(data))
}

func emitHuman(ev jsonEvent) {
	switch ev.Type {
	case "item.started", "item.updated", "item.completed":
		if ev.Item != nil {
			text := ev.Item.Text
			if text == "" {
				text = ev.Item.Status
			}
			if text != "" {
				fmt.Fprintf(os.Stderr, "[%s] %s\n", ev.Item.Type, strings.TrimSpace(text))
			}
		}
	case "turn.failed":
		if ev.Error != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", ev.Error.Message)
		}
	}
}
