package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/logger"
	"echo-cli/internal/tools"
)

// Options 定义引擎的可注入依赖。
type Options struct {
	Manager        *events.Manager
	ManagerConfig  events.ManagerConfig
	Client         agent.ModelClient
	Bus            *events.Bus
	Defaults       SessionDefaults
	ErrorLogPath   string
	LLMLogPath     string
	ToolTimeout    time.Duration
	RequestTimeout time.Duration
	Retries        int
}

// Engine 实现 SQ→核心→EQ 的执行流程。
type Engine struct {
	manager        *events.Manager
	contexts       *ContextManager
	client         agent.ModelClient
	bus            *events.Bus
	active         map[string]*taskHandle
	activeMu       sync.Mutex
	forwarder      context.CancelFunc
	wg             sync.WaitGroup
	toolTimeout    time.Duration
	requestTimeout time.Duration
	retries        int

	toolCtxMu sync.Mutex
	toolCtx   map[string]toolCallContext // tool call id -> submission context
}

type taskHandle struct {
	cancel context.CancelFunc
}

type toolCallContext struct {
	SubmissionID string
	SessionID    string
	Metadata     map[string]string
}

// NewEngine 构造一个新的执行引擎。
func NewEngine(opts Options) *Engine {
	manager := opts.Manager
	if manager == nil {
		cfg := opts.ManagerConfig
		if cfg.Workers == 0 {
			cfg.Workers = 2
		}
		manager = events.NewManager(cfg)
	}
	ensureErrorLogger(opts.ErrorLogPath)
	ensureLLMLogger(opts.LLMLogPath)
	toolTimeout := opts.ToolTimeout
	if toolTimeout == 0 {
		toolTimeout = 2 * time.Minute
	}
	reqTimeout := opts.RequestTimeout
	if reqTimeout == 0 {
		reqTimeout = 2 * time.Minute
	}
	return &Engine{
		manager:        manager,
		contexts:       NewContextManager(opts.Defaults),
		client:         opts.Client,
		bus:            opts.Bus,
		active:         map[string]*taskHandle{},
		toolTimeout:    toolTimeout,
		requestTimeout: reqTimeout,
		retries:        opts.Retries,
		toolCtx:        map[string]toolCallContext{},
	}
}

// Start 注册处理器并启动 SQ/EQ。
func (e *Engine) Start(ctx context.Context) {
	e.manager.RegisterHandler(events.OperationUserInput, events.HandlerFunc(e.handleUserInput))
	e.manager.RegisterHandler(events.OperationInterrupt, events.HandlerFunc(e.handleInterrupt))
	e.manager.Start(ctx)
	e.startToolForwarder(ctx)
}

// Close 关闭引擎与内部 goroutine。
func (e *Engine) Close() {
	if e.forwarder != nil {
		e.forwarder()
	}
	e.wg.Wait()
	e.manager.Close()
}

// Events 订阅 EQ。
func (e *Engine) Events() <-chan events.Event {
	return e.manager.Subscribe()
}

// SubmitUserInput 放入 SQ。
func (e *Engine) SubmitUserInput(ctx context.Context, items []events.InputMessage, inputCtx events.InputContext) (string, error) {
	return e.manager.SubmitUserInput(ctx, items, inputCtx)
}

// Submit 允许直接提交任意 Submission。
func (e *Engine) Submit(ctx context.Context, sub events.Submission) (string, error) {
	return e.manager.Submit(ctx, sub)
}

// SubmitInterrupt 便捷方法：按会话触发中断。
func (e *Engine) SubmitInterrupt(ctx context.Context, sessionID string) (string, error) {
	return e.Submit(ctx, events.Submission{
		SessionID: sessionID,
		Operation: events.Operation{Kind: events.OperationInterrupt},
	})
}

// History 返回会话历史。
func (e *Engine) History(sessionID string) []agent.Message {
	return e.contexts.History(sessionID)
}

// SeedHistory 预载入指定会话的历史，便于恢复。
func (e *Engine) SeedHistory(sessionID string, history []agent.Message) {
	e.contexts.AppendMessages(sessionID, history)
}

func (e *Engine) handleUserInput(ctx context.Context, submission events.Submission, emit events.EventPublisher) error {
	if e.client == nil {
		return errors.New("model client not configured")
	}
	if submission.Operation.UserInput == nil {
		return errors.New("missing user input payload")
	}
	turn := submission.Operation.UserInput
	if len(turn.Items) == 0 {
		return errors.New("empty user input items")
	}

	taskCtx, cancel := context.WithCancel(ctx)
	handle := e.trackActive(submission.SessionID, cancel)
	defer e.clearActive(submission.SessionID, handle)

	state := e.contexts.PrepareTurn(submission.SessionID, turn.Context, turn.Items)
	return e.runTask(taskCtx, submission, state, emit)
}

func (e *Engine) handleInterrupt(ctx context.Context, submission events.Submission, _ events.EventPublisher) error {
	e.activeMu.Lock()
	handle := e.active[submission.SessionID]
	e.activeMu.Unlock()
	if handle != nil && handle.cancel != nil {
		handle.cancel()
	}
	return nil
}

type turnResult struct {
	responses     []ResponseInputItem
	itemsToRecord []ResponseItem
	finalContent  string
}

type modelTurnOutput struct {
	fullResponse string
	toolCalls    []tools.ToolCall
	items        []ResponseItem
}

const toolErrorOutputLimit = 400

func (e *Engine) logRunTaskError(submission events.Submission, stage string, err error, fields logger.Fields) {
	if err == nil {
		return
	}
	if fields == nil {
		fields = logger.Fields{}
	}
	if stage != "" {
		fields["stage"] = stage
	}
	if submission.SessionID != "" {
		fields["session_id"] = submission.SessionID
	}
	if submission.ID != "" {
		fields["submission_id"] = submission.ID
	}
	if submission.Operation.Kind != "" {
		fields["operation"] = submission.Operation.Kind
	}

	msg := "runTask error"
	if stage != "" {
		msg = fmt.Sprintf("runTask %s error", stage)
	}
	errorLog.WithError(err).WithFields(fields).Error(msg)
}

func collectToolCallIDs(calls []tools.ToolCall) []string {
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		if call.ID != "" {
			ids = append(ids, call.ID)
		}
	}
	return ids
}

func (e *Engine) logToolResultError(submission events.Submission, turnCtx TurnContext, seq int, result tools.ToolResult) {
	reason := strings.TrimSpace(result.Error)
	if reason == "" && result.ExitCode != 0 {
		reason = fmt.Sprintf("non-zero exit code %d", result.ExitCode)
	}
	if reason == "" && strings.EqualFold(result.Status, "error") {
		reason = "tool reported error status"
	}
	if reason == "" {
		return
	}
	fields := logger.Fields{
		"tool_id":     result.ID,
		"tool_kind":   result.Kind,
		"tool_status": result.Status,
		"sequence":    seq,
		"model":       turnCtx.Model,
	}
	if result.Command != "" {
		fields["tool_command"] = result.Command
	}
	if result.Path != "" {
		fields["tool_path"] = result.Path
	}
	if result.ExitCode != 0 {
		fields["tool_exit_code"] = result.ExitCode
	}
	if out := strings.TrimSpace(result.Output); out != "" {
		fields["tool_output_preview"] = sanitizeLogText(previewForLog(out, toolErrorOutputLimit))
	}
	e.logRunTaskError(submission, "tool_result", errors.New(reason), fields)
}

// runTask 对应 codex-rs 的 run_task：负责回合循环，内部委托 runTurn 处理单轮。
// runTurn 再拆分为模型交互 -> 工具识别 -> 工具路由 -> 工具执行四层。
func (e *Engine) runTask(ctx context.Context, submission events.Submission, state TurnState, emit events.EventPublisher) error {
	seq := 0
	turnCtx := state.Context
	toolEvents, stopTools := e.subscribeToolEvents(ctx)
	defer stopTools()
	publishedCalls := map[string]struct{}{}

	exitReason := "unknown"
	exitStage := "unknown"
	var exitErr error
	exitFinalContent := ""
	exitFinalItems := 0
	defer func() {
		fields := logger.Fields{
			"session":      submission.SessionID,
			"submission":   submission.ID,
			"model":        turnCtx.Model,
			"sequence":     seq,
			"exit_reason":  exitReason,
			"exit_stage":   exitStage,
			"final_items":  exitFinalItems,
			"final_length": len(exitFinalContent),
		}
		if exitErr != nil {
			fields["exit_error"] = sanitizeLogText(exitErr.Error())
		}
		if strings.TrimSpace(exitFinalContent) != "" {
			fields["final_preview"] = sanitizeLogText(previewForLog(exitFinalContent, 600))
		}
		llmLog.WithField("type", "run_task.exit").WithField("dir", "agent").WithFields(fields).Info("run_task exit")
	}()

	for {
		if err := ctx.Err(); err != nil {
			// Treat cancellation as an aborted turn to mirror Codex behaviour.
			e.logRunTaskError(submission, "run_task", err, logger.Fields{
				"sequence": seq,
				"model":    turnCtx.Model,
			})
			exitReason = "context_done"
			exitStage = "ctx_check"
			exitErr = err
			return err
		}

		turn, err := e.runTurn(ctx, submission, turnCtx, emit, &seq, toolEvents, publishedCalls)
		if err != nil {
			exitErr = err
			exitStage = "run_turn"
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				exitReason = "context_done"
			} else {
				exitReason = "error"
			}
			return err
		}
		e.recordConversationItems(submission.SessionID, &turnCtx, turn.itemsToRecord)

		if e.tokenLimitReached() {
			e.runInlineAutoCompactTask(ctx, turnCtx)
			continue
		}

		if len(turn.responses) == 0 {
			_ = emit.Publish(ctx, events.Event{
				Type:         events.EventAgentOutput,
				SubmissionID: submission.ID,
				SessionID:    submission.SessionID,
				Timestamp:    time.Now(),
				Payload: events.AgentOutput{
					Content:  turn.finalContent,
					Final:    true,
					Sequence: seq,
				},
				Metadata: submission.Metadata,
			})
			exitReason = "completed_final"
			exitStage = "final_no_responses"
			exitFinalContent = turn.finalContent
			exitFinalItems = len(turn.itemsToRecord)
			return nil
		}
	}

}

// runTurn 将单轮拆分为模型交互（LLM 输出）、工具识别（标记转项）、工具路由（发布 marker）、工具执行（等待结果）。
func (e *Engine) runTurn(ctx context.Context, submission events.Submission, turnCtx TurnContext, emit events.EventPublisher, seq *int, toolEvents <-chan tools.ToolEvent, publishedCalls map[string]struct{}) (turnResult, error) {
	prompt := turnCtx.BuildPrompt()
	logPrompt(submission, prompt)

	output, err := e.runModelInteraction(ctx, submission, prompt, emit, seq)
	if err != nil {
		return turnResult{}, err
	}

	processed := e.identifyTools(output)
	e.routeTools(ctx, submission, output.toolCalls, publishedCalls)

	results, err := e.executeTools(ctx, output.toolCalls, toolEvents)
	if err != nil {
		e.logRunTaskError(submission, "tool_execution", err, logger.Fields{
			"model":      turnCtx.Model,
			"sequence":   *seq,
			"tool_ids":   collectToolCallIDs(output.toolCalls),
			"tool_count": len(output.toolCalls),
		})
		return turnResult{}, err
	}
	for _, res := range results {
		e.logToolResultError(submission, turnCtx, *seq, res)
	}
	e.publishPlanUpdates(ctx, submission, results, emit)
	processed = append(processed, processedFromToolResults(results)...)

	responses, itemsToRecord := processItems(processed)

	return turnResult{
		responses:     responses,
		itemsToRecord: itemsToRecord,
		finalContent:  deriveFinalContent(output.fullResponse, itemsToRecord),
	}, nil
}

// runModelInteraction 负责模型流式交互与输出收集，仅处理「模型交互」层。
// 对齐 Codex：拉取流式事件、发布增量输出、收集工具标记与 ResponseItem。
func (e *Engine) runModelInteraction(ctx context.Context, submission events.Submission, prompt Prompt, emit events.EventPublisher, seq *int) (modelTurnOutput, error) {
	collector := newModelStreamCollector()

	err := e.streamPrompt(ctx, prompt, func(evt agent.StreamEvent) {
		switch evt.Type {
		case agent.StreamEventTextDelta:
			if evt.Text == "" {
				return
			}
			collector.OnTextDelta(evt.Text)

			_ = emit.Publish(ctx, events.Event{
				Type:         events.EventAgentOutput,
				SubmissionID: submission.ID,
				SessionID:    submission.SessionID,
				Timestamp:    time.Now(),
				Payload: events.AgentOutput{
					Content:  evt.Text,
					Sequence: *seq,
				},
				Metadata: submission.Metadata,
			})
			*seq++
		case agent.StreamEventItem:
			collector.OnItem(evt.Item)
		case agent.StreamEventCompleted:
		default:
		}
	})
	if err != nil {
		e.logRunTaskError(submission, "model_interaction", err, logger.Fields{
			"model":           prompt.Model,
			"message_count":   len(prompt.Messages),
			"sequence":        *seq,
			"response_so_far": sanitizeLogText(previewForLog(collector.builder.String(), 200)),
		})
		return modelTurnOutput{}, err
	}

	output := collector.Result()
	in := llmIn()
	if text := strings.TrimSpace(output.fullResponse); text != "" {
		in.Infof(
			"llm->agent response session=%s submission=%s model=%s len=%d content=%s",
			submission.SessionID,
			submission.ID,
			prompt.Model,
			len(output.fullResponse),
			sanitizeLogText(output.fullResponse),
		)
	} else {
		in.Infof(
			"llm->agent response session=%s submission=%s model=%s len=0",
			submission.SessionID,
			submission.ID,
			prompt.Model,
		)
	}
	if len(output.items) > 0 {
		if encoded, err := json.MarshalIndent(output.items, "", "  "); err == nil {
			in.Infof(
				"llm->agent items session=%s submission=%s model=%s count=%d payload=%s",
				submission.SessionID,
				submission.ID,
				prompt.Model,
				len(output.items),
				sanitizeLogText(string(encoded)),
			)
		} else {
			in.Warnf(
				"llm->agent items session=%s submission=%s model=%s count=%d marshal_error=%v",
				submission.SessionID,
				submission.ID,
				prompt.Model,
				len(output.items),
				err,
			)
		}
	}

	return output, nil
}

// identifyTools 负责从模型输出中识别工具调用并构造历史记录项（「工具识别」层）。
func (e *Engine) identifyTools(output modelTurnOutput) []ProcessedResponseItem {
	processed := make([]ProcessedResponseItem, 0, len(output.items)+1)
	processed = append(processed, processedFromResponseItems(output.items)...)
	if strings.TrimSpace(output.fullResponse) != "" && !hasAssistantMessageItem(output.items) {
		processed = append(processed, ProcessedResponseItem{Item: NewAssistantMessageItem(output.fullResponse)})
	}
	return processed
}

func hasAssistantMessageItem(items []ResponseItem) bool {
	for _, item := range items {
		if item.Type == ResponseItemTypeMessage && item.Message != nil && item.Message.Role == "assistant" {
			return true
		}
	}
	return false
}

// routeTools 将模型工具调用转换为 ToolCall 并投递到总线（「工具路由」层）。
func (e *Engine) routeTools(ctx context.Context, submission events.Submission, calls []tools.ToolCall, publishedCalls map[string]struct{}) {
	e.dispatchToolCalls(ctx, submission, calls, publishedCalls)
}

// executeTools 等待工具执行结果并返回（「工具执行」层）。
func (e *Engine) executeTools(ctx context.Context, calls []tools.ToolCall, toolEvents <-chan tools.ToolEvent) ([]tools.ToolResult, error) {
	return e.collectToolResults(ctx, calls, toolEvents)
}

func deriveFinalContent(fallback string, items []ResponseItem) string {
	finalContent := lastAssistantMessage(items)
	if finalContent == "" {
		return fallback
	}
	return finalContent
}

const (
	llmDirAgentToLLM = "agent->llm"
	llmDirLLMToAgent = "llm->agent"
)

func llmOut() *logger.LogEntry {
	return llmLog.WithField("dir", llmDirAgentToLLM)
}

func llmIn() *logger.LogEntry {
	return llmLog.WithField("dir", llmDirLLMToAgent)
}

func logPrompt(submission events.Submission, prompt Prompt) {
	out := llmOut()
	if encoded, err := json.MarshalIndent(prompt, "", "  "); err == nil {
		out.Infof("agent->llm prompt session=%s submission=%s payload=%s", submission.SessionID, submission.ID, sanitizeLogText(string(encoded)))
	} else {
		out.Warnf("agent->llm prompt session=%s submission=%s model=%s marshal_error=%v", submission.SessionID, submission.ID, prompt.Model, err)
	}
	out.Infof("agent->llm messages session=%s submission=%s model=%s count=%d", submission.SessionID, submission.ID, prompt.Model, len(prompt.Messages))
	for i, msg := range prompt.Messages {
		out.Infof("agent->llm message[%d] role=%s content=%s", i, msg.Role, sanitizeLogText(msg.Content))
	}
}

type modelStreamCollector struct {
	builder   strings.Builder
	toolCalls []tools.ToolCall
	items     []ResponseItem
	seenCalls map[string]struct{}
}

func newModelStreamCollector() *modelStreamCollector {
	return &modelStreamCollector{
		seenCalls: make(map[string]struct{}),
	}
}

func (c *modelStreamCollector) OnTextDelta(chunk string) {
	c.builder.WriteString(chunk)
}

func (c *modelStreamCollector) OnItem(raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var item ResponseItem
	if err := json.Unmarshal(raw, &item); err != nil {
		log.Warnf("parse stream item: %v", err)
		return
	}
	c.items = append(c.items, item)
	c.addToolCall(toolCallFromResponseItem(item))
	if text := textFromResponseItem(item); strings.TrimSpace(text) != "" {
		c.builder.WriteString(text)
	}
}

func (c *modelStreamCollector) Result() modelTurnOutput {
	return modelTurnOutput{
		fullResponse: c.builder.String(),
		toolCalls:    c.toolCalls,
		items:        c.items,
	}
}

func (c *modelStreamCollector) addToolCall(call tools.ToolCall, ok bool) {
	if !ok || call.Name == "" || call.ID == "" {
		return
	}
	if _, seen := c.seenCalls[call.ID]; seen {
		return
	}
	c.seenCalls[call.ID] = struct{}{}
	c.toolCalls = append(c.toolCalls, call)
}

func toolCallFromResponseItem(item ResponseItem) (tools.ToolCall, bool) {
	if item.Type != ResponseItemTypeFunctionCall || item.FunctionCall == nil {
		return tools.ToolCall{}, false
	}
	if item.FunctionCall.Name == "" || item.FunctionCall.CallID == "" {
		return tools.ToolCall{}, false
	}
	return tools.ToolCall{
		ID:      item.FunctionCall.CallID,
		Name:    item.FunctionCall.Name,
		Payload: normalizeRawJSON(item.FunctionCall.Arguments),
	}, true
}

func textFromResponseItem(item ResponseItem) string {
	switch item.Type {
	case ResponseItemTypeMessage:
		if item.Message == nil {
			return ""
		}
		return flattenContentItems(item.Message.Content)
	case ResponseItemTypeReasoning:
		if item.Reasoning == nil {
			return ""
		}
		return flattenReasoning(*item.Reasoning)
	default:
		return ""
	}
}

func normalizeRawJSON(text string) json.RawMessage {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return json.RawMessage("null")
	}
	data := []byte(trimmed)
	if json.Valid(data) {
		return json.RawMessage(data)
	}
	encoded, err := json.Marshal(trimmed)
	if err != nil {
		return json.RawMessage("null")
	}
	return json.RawMessage(encoded)
}

// prettyPayloadBytesForLog 尝试对 JSON bytes 做缩进美化，并在日志中保持单行输出。
// limit <= 0 表示不截断。
func prettyPayloadBytesForLog(raw []byte, limit int) string {
	data := bytes.TrimSpace(raw)
	if len(data) == 0 {
		data = []byte("null")
	}
	if !json.Valid(data) {
		quoted, err := json.Marshal(string(data))
		if err != nil {
			return "null"
		}
		data = quoted
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		pretty := string(data)
		if limit > 0 {
			pretty = previewForLog(pretty, limit)
		}
		return sanitizeLogText(pretty)
	}
	pretty := buf.String()
	if limit > 0 {
		pretty = previewForLog(pretty, limit)
	}
	return sanitizeLogText(pretty)
}

func (e *Engine) streamPrompt(ctx context.Context, prompt Prompt, onEvent func(agent.StreamEvent)) error {
	out := llmOut()
	in := llmIn()
	messages := prompt.Messages
	model := strings.TrimSpace(prompt.Model)
	if model == "" {
		return errors.New("model not specified")
	}
	var lastErr error
	for attempt := 0; attempt <= e.retries; attempt++ {
		out.Infof(
			"agent->llm request attempt=%d model=%s messages=%d tools=%d parallel_tools=%t output_schema_len=%d",
			attempt+1,
			model,
			len(messages),
			len(prompt.Tools),
			prompt.ParallelToolCalls,
			len(strings.TrimSpace(prompt.OutputSchema)),
		)
		// 首次请求的提示词已由 logPrompt 记录，这里仅在重试时重复打印。
		if attempt > 0 {
			for i, msg := range messages {
				out.Infof("agent->llm message[%d] role=%s content=%s", i, msg.Role, sanitizeLogText(msg.Content))
			}
		}
		ctxRun, cancel := context.WithTimeout(ctx, e.requestTimeout)
		err := e.client.Stream(ctxRun, prompt, func(evt agent.StreamEvent) {
			switch evt.Type {
			case agent.StreamEventTextDelta:
				in.Debugf("llm->agent stream chunk type=text len=%d preview=%s", len(evt.Text), sanitizeLogText(previewForLog(evt.Text, 200)))
			case agent.StreamEventItem:
				in.Debugf("llm->agent stream chunk type=item len=%d payload=%s", len(evt.Item), prettyPayloadBytesForLog(evt.Item, 200))
			case agent.StreamEventCompleted:
				in.Debugf("llm->agent stream chunk type=completed")
			default:
				in.Debugf("llm->agent stream chunk type=%s", evt.Type)
			}
			onEvent(evt)
		})
		cancel()
		if err == nil {
			in.Infof("llm->agent stream completed attempt=%d model=%s", attempt+1, model)
			return nil
		}
		in.Errorf("llm->agent error attempt=%d model=%s err=%v", attempt+1, model, err)
		lastErr = err
	}
	return lastErr
}

func sanitizeLogText(text string) string {
	text = strings.ReplaceAll(text, "\n", `\n`)
	text = strings.ReplaceAll(text, "\r", `\r`)
	return text
}

func previewForLog(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	if limit < 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}

func (e *Engine) startToolForwarder(ctx context.Context) {
	if e.bus == nil {
		return
	}
	forwardCtx, cancel := context.WithCancel(ctx)
	e.forwarder = cancel
	ch := e.bus.Subscribe()
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-forwardCtx.Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				toolEvt, ok := evt.(tools.ToolEvent)
				if !ok {
					continue
				}
				meta := map[string]string{"tool_kind": string(toolEvt.Result.Kind)}
				// Best-effort: attach submission/session so EQ consumers can group tool events.
				subID, sessID, subMeta := e.lookupToolCallContext(toolEvt.Result.ID)
				if sessID == "" {
					// If we can't associate to a session, still publish a tool.event for auditing,
					// but session-scoped renderers will typically ignore it.
				}
				if subMeta != nil {
					// Preserve submission metadata while keeping tool_kind explicit.
					for k, v := range subMeta {
						if _, exists := meta[k]; !exists {
							meta[k] = v
						}
					}
				}
				_ = e.manager.PublishEvent(forwardCtx, events.Event{
					Type: events.EventToolEvent,
					// Tool events are correlated by tool call id; these IDs are optional but
					// enable session/submission filtering and grouping for terminal renderers.
					SubmissionID: subID,
					SessionID:    sessID,
					Timestamp:    time.Now(),
					Payload:      toolEvt,
					Metadata:     meta,
				})

				if toolEvt.Type == "item.completed" {
					e.clearToolCallContext(toolEvt.Result.ID)
				}
			}
		}
	}()
}

func (e *Engine) trackActive(sessionID string, cancel context.CancelFunc) *taskHandle {
	key := sessionID
	e.activeMu.Lock()
	if current := e.active[key]; current != nil && current.cancel != nil {
		current.cancel()
	}
	handle := &taskHandle{cancel: cancel}
	e.active[key] = handle
	e.activeMu.Unlock()
	return handle
}

func (e *Engine) clearActive(sessionID string, handle *taskHandle) {
	key := sessionID
	e.activeMu.Lock()
	if cur := e.active[key]; cur == handle {
		delete(e.active, key)
	}
	e.activeMu.Unlock()
}

func (e *Engine) subscribeToolEvents(ctx context.Context) (<-chan tools.ToolEvent, func()) {
	if e.bus == nil {
		return nil, func() {}
	}
	busCh := e.bus.Subscribe()
	out := make(chan tools.ToolEvent, 64)
	stop := make(chan struct{})
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case evt, ok := <-busCh:
				if !ok {
					return
				}
				toolEvt, ok := evt.(tools.ToolEvent)
				if !ok {
					continue
				}
				select {
				case out <- toolEvt:
				case <-ctx.Done():
					return
				case <-stop:
					return
				}
			}
		}
	}()
	return out, func() { close(stop) }
}

func (e *Engine) collectToolResults(ctx context.Context, calls []tools.ToolCall, events <-chan tools.ToolEvent) ([]tools.ToolResult, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	if events == nil {
		return nil, errors.New("tool event stream not configured")
	}
	pending := make(map[string]tools.ToolCall, len(calls))
	order := make([]string, 0, len(calls))
	for _, call := range calls {
		pending[call.ID] = call
		order = append(order, call.ID)
	}

	waitCtx := ctx
	cancel := func() {}
	if e.toolTimeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, e.toolTimeout)
	}
	defer cancel()

	results := make(map[string]tools.ToolResult, len(pending))
	for len(results) < len(pending) {
		select {
		case <-waitCtx.Done():
			return nil, waitCtx.Err()
		case ev, ok := <-events:
			if !ok {
				return nil, errors.New("tool event stream closed")
			}
			if _, ok := pending[ev.Result.ID]; !ok {
				continue
			}
			if ev.Type != "item.completed" {
				continue
			}
			results[ev.Result.ID] = ev.Result
		}
	}

	ordered := make([]tools.ToolResult, 0, len(order))
	for _, id := range order {
		if res, ok := results[id]; ok {
			ordered = append(ordered, res)
		}
	}
	return ordered, nil
}

func (e *Engine) dispatchToolCalls(ctx context.Context, submission events.Submission, calls []tools.ToolCall, seen map[string]struct{}) {
	if e.bus == nil || len(calls) == 0 {
		return
	}
	for _, call := range calls {
		if call.Name == "" || call.ID == "" {
			continue
		}
		if seen != nil {
			if _, ok := seen[call.ID]; ok {
				continue
			}
			seen[call.ID] = struct{}{}
		}
		e.registerToolCallContext(submission, call.ID)
		e.bus.Publish(tools.DispatchRequest{Ctx: ctx, Call: call})
	}
}

func (e *Engine) registerToolCallContext(submission events.Submission, callID string) {
	if strings.TrimSpace(callID) == "" {
		return
	}
	e.toolCtxMu.Lock()
	e.toolCtx[callID] = toolCallContext{
		SubmissionID: submission.ID,
		SessionID:    submission.SessionID,
		Metadata:     cloneMetadataMap(submission.Metadata),
	}
	e.toolCtxMu.Unlock()
}

func (e *Engine) lookupToolCallContext(callID string) (submissionID, sessionID string, meta map[string]string) {
	if strings.TrimSpace(callID) == "" {
		return "", "", nil
	}
	e.toolCtxMu.Lock()
	ctx, ok := e.toolCtx[callID]
	e.toolCtxMu.Unlock()
	if !ok {
		return "", "", nil
	}
	return ctx.SubmissionID, ctx.SessionID, cloneMetadataMap(ctx.Metadata)
}

func (e *Engine) clearToolCallContext(callID string) {
	if strings.TrimSpace(callID) == "" {
		return
	}
	e.toolCtxMu.Lock()
	delete(e.toolCtx, callID)
	e.toolCtxMu.Unlock()
}

func cloneMetadataMap(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func formatToolResults(results []tools.ToolResult) []agent.Message {
	msgs := make([]agent.Message, 0, len(results))
	for _, res := range results {
		if res.Kind == tools.ToolPlanUpdate {
			msgs = append(msgs, agent.Message{Role: agent.RoleUser, Content: formatPlanResult(res)})
			continue
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Tool %s (%s)", res.Kind, res.ID))
		if res.Command != "" {
			sb.WriteString("\ncommand: " + res.Command)
		}
		if res.Path != "" {
			sb.WriteString("\npath: " + res.Path)
		}
		if res.ExitCode != 0 {
			sb.WriteString(fmt.Sprintf("\nexit_code: %d", res.ExitCode))
		}
		switch {
		case res.Error != "":
			sb.WriteString("\nerror: " + res.Error)
		case res.Output != "":
			sb.WriteString("\noutput:\n" + res.Output)
		case res.Status != "":
			sb.WriteString("\nstatus: " + res.Status)
		}
		msgs = append(msgs, agent.Message{Role: agent.RoleUser, Content: sb.String()})
	}
	return msgs
}

func formatPlanResult(res tools.ToolResult) string {
	if res.Error != "" {
		return fmt.Sprintf("Plan update failed: %s", res.Error)
	}

	var sb strings.Builder
	sb.WriteString("Plan update")
	if strings.TrimSpace(res.Explanation) != "" {
		sb.WriteString("\nexplanation: " + strings.TrimSpace(res.Explanation))
	}

	if len(res.Plan) == 0 {
		sb.WriteString("\nplan: (empty)")
		return sb.String()
	}

	for _, item := range res.Plan {
		icon := "•"
		switch item.Status {
		case "completed":
			icon = "✓"
		case "in_progress":
			icon = "→"
		}
		sb.WriteString(fmt.Sprintf("\n- [%s] %s", icon, item.Step))
	}
	return sb.String()
}

// publishPlanUpdates emits a dedicated EQ event for successful update_plan tool results.
// This mirrors codex-rs EventMsg::PlanUpdate: the plan is treated as the latest snapshot.
func (e *Engine) publishPlanUpdates(ctx context.Context, submission events.Submission, results []tools.ToolResult, emit events.EventPublisher) {
	for _, res := range results {
		if res.Kind != tools.ToolPlanUpdate {
			continue
		}
		if res.Error != "" || res.Status == "error" {
			continue
		}
		_ = emit.Publish(ctx, events.Event{
			Type:         events.EventPlanUpdated,
			SubmissionID: submission.ID,
			SessionID:    submission.SessionID,
			Timestamp:    time.Now(),
			Payload: tools.UpdatePlanArgs{
				Explanation: strings.TrimSpace(res.Explanation),
				Plan:        res.Plan,
			},
			Metadata: submission.Metadata,
		})
	}
}

// processItems mirrors the codex.rs process_items helper: collect tool responses for the next turn
// and return the list of items that should be recorded into the conversation history.
func processItems(items []ProcessedResponseItem) ([]ResponseInputItem, []ResponseItem) {
	responses := make([]ResponseInputItem, 0, len(items))
	record := make([]ResponseItem, 0, len(items))
	for _, item := range items {
		record = append(record, item.Item)
		if item.Response != nil {
			responses = append(responses, *item.Response)
		}
	}
	return responses, record
}

func (e *Engine) recordConversationItems(sessionID string, turnCtx *TurnContext, items []ResponseItem) {
	if len(items) == 0 {
		return
	}
	e.contexts.AppendResponseItems(sessionID, items)
	turnCtx.ResponseHistory = append(turnCtx.ResponseHistory, items...)
	turnCtx.History = append(turnCtx.History, responseItemsToAgentMessages(items)...)
}

// tokenLimitReached is a stub for now; hook in real token accounting when available.
func (e *Engine) tokenLimitReached() bool {
	return false
}

// runInlineAutoCompactTask is a no-op placeholder to keep control flow aligned with the Codex reference.
func (e *Engine) runInlineAutoCompactTask(ctx context.Context, turnCtx TurnContext) {
	_ = ctx
	log.Infof("token limit reached for model=%s; auto-compaction not implemented", turnCtx.Model)
}
