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
	echocontext "echo-cli/internal/context"
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
	Defaults       echocontext.SessionDefaults
	ErrorLogPath   string
	LLMLogPath     string
	ToolTimeout    time.Duration
	RequestTimeout time.Duration
	Retries        int
	RetryDelay     time.Duration
}

// Engine 实现 SQ→核心→EQ 的执行流程。
type Engine struct {
	manager        *events.Manager
	contexts       *echocontext.ContextManager
	client         agent.ModelClient
	bus            *events.Bus
	active         map[string]*taskHandle
	activeMu       sync.Mutex
	forwarder      context.CancelFunc
	wg             sync.WaitGroup
	toolTimeout    time.Duration
	requestTimeout time.Duration
	retries        int
	retryDelay     time.Duration

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
	retryDelay := opts.RetryDelay
	if retryDelay == 0 {
		retryDelay = time.Second
	}
	return &Engine{
		manager:        manager,
		contexts:       echocontext.NewContextManager(opts.Defaults),
		client:         opts.Client,
		bus:            opts.Bus,
		active:         map[string]*taskHandle{},
		toolTimeout:    toolTimeout,
		requestTimeout: reqTimeout,
		retries:        opts.Retries,
		retryDelay:     retryDelay,
		toolCtx:        map[string]toolCallContext{},
	}
}

// Start 注册处理器并启动 SQ/EQ。
func (e *Engine) Start(ctx context.Context) {
	e.manager.RegisterHandler(events.OperationUserInput, events.HandlerFunc(e.handleUserInput))
	e.manager.RegisterHandler(events.OperationInterrupt, events.HandlerFunc(e.handleInterrupt))
	e.manager.RegisterHandler(events.OperationApprovalDecision, events.HandlerFunc(e.handleApprovalDecision))
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

func (e *Engine) handleApprovalDecision(ctx context.Context, submission events.Submission, _ events.EventPublisher) error {
	if submission.Operation.ApprovalDecision == nil {
		return errors.New("missing approval decision payload")
	}
	if e.bus == nil {
		return errors.New("tool bus not configured")
	}
	dec := submission.Operation.ApprovalDecision
	if strings.TrimSpace(dec.ApprovalID) == "" {
		return errors.New("approval id required")
	}
	e.bus.Publish(tools.ApprovalDecision{
		ApprovalID: strings.TrimSpace(dec.ApprovalID),
		Approved:   dec.Approved,
	})
	return nil
}

type turnResult struct {
	responses     []echocontext.ResponseInputItem
	itemsToRecord []echocontext.ResponseItem
	finalContent  string
}

type modelTurnOutput struct {
	fullResponse string
	toolCalls    []tools.ToolCall
	items        []echocontext.ResponseItem
}

const toolErrorOutputLimit = 400
const toolPayloadPreviewLimit = 2000
const toolDiffPreviewLimit = 2000
const toolPatchHeadPreviewLimit = 2000
const toolPatchTailPreviewLimit = 800

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

func findToolCall(calls []tools.ToolCall, id string) *tools.ToolCall {
	if id == "" {
		return nil
	}
	for i := range calls {
		if calls[i].ID == id {
			return &calls[i]
		}
	}
	return nil
}

func (e *Engine) logToolResultError(submission events.Submission, turnCtx echocontext.TurnContext, seq int, call *tools.ToolCall, result tools.ToolResult) {
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
	if call != nil {
		fields["tool_call_name"] = call.Name
		fields["tool_payload_len"] = len(call.Payload)
		if payload := bytes.TrimSpace(call.Payload); len(payload) != 0 {
			fields["tool_payload_preview"] = prettyPayloadBytesForLog(payload, toolPayloadPreviewLimit)
		}
		if result.Kind == tools.ToolApplyPatch {
			var args struct {
				Patch string `json:"patch"`
				Path  string `json:"path"`
			}
			_ = json.Unmarshal(call.Payload, &args)
			patch := strings.TrimSpace(args.Patch)
			if patch != "" {
				fields["tool_patch_len"] = len(patch)
				fields["tool_patch_has_end_patch"] = strings.Contains(patch, "*** End Patch")
				fields["tool_patch_head_preview"] = sanitizeLogText(previewForLog(patch, toolPatchHeadPreviewLimit))
				fields["tool_patch_tail_preview"] = sanitizeLogText(tailPreviewForLog(patch, toolPatchTailPreviewLimit))
			}
			if p := strings.TrimSpace(args.Path); p != "" && result.Path == "" {
				fields["tool_path"] = p
			}
		}
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
	if diff := strings.TrimSpace(result.Diff); diff != "" {
		fields["tool_diff_len"] = len(diff)
		fields["tool_diff_preview"] = sanitizeLogText(previewForLog(diff, toolDiffPreviewLimit))
	}
	if out := strings.TrimSpace(result.Output); out != "" {
		fields["tool_output_preview"] = sanitizeLogText(previewForLog(out, toolErrorOutputLimit))
	}
	e.logRunTaskError(submission, "tool_result", errors.New(reason), fields)
}

// runTask 对应 codex-rs 的 run_task：负责回合循环，内部委托 runTurn 处理单轮。
// runTurn 再拆分为模型交互 -> 工具识别 -> 工具路由 -> 工具执行四层。
func (e *Engine) runTask(ctx context.Context, submission events.Submission, state echocontext.TurnState, emit events.EventPublisher) error {
	start := time.Now()
	summaryAcc := newTaskSummaryAccumulator(start)

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
		// Always emit a turn/task summary event, even when the task fails or is interrupted.
		publishCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = emit.Publish(publishCtx, events.Event{
			Type:         events.EventTaskSummary,
			SubmissionID: submission.ID,
			SessionID:    submission.SessionID,
			Timestamp:    time.Now(),
			Payload: summaryAcc.Build(
				submission,
				turnCtx,
				exitReason,
				exitStage,
				exitFinalContent,
				exitErr,
			),
			Metadata: submission.Metadata,
		})
	}()

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

		turn, toolResults, err := e.runTurn(ctx, submission, turnCtx, emit, &seq, toolEvents, publishedCalls)
		if err != nil {
			exitErr = err
			exitStage = "run_turn"
			var se stageError
			if errors.As(err, &se) && strings.TrimSpace(se.Stage) != "" {
				exitStage = se.Stage
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				exitReason = "context_done"
			} else {
				exitReason = "error"
			}
			return err
		}
		summaryAcc.ObserveToolResults(toolResults)
		e.recordConversationItems(submission.SessionID, &turnCtx, turn.itemsToRecord)

		if e.tokenLimitReached(turnCtx) {
			updated, compacted := e.runInlineAutoCompactTask(ctx, submission.SessionID, turnCtx)
			if compacted {
				turnCtx = updated
			}
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
func (e *Engine) runTurn(ctx context.Context, submission events.Submission, turnCtx echocontext.TurnContext, emit events.EventPublisher, seq *int, toolEvents <-chan tools.ToolEvent, publishedCalls map[string]struct{}) (turnResult, []tools.ToolResult, error) {
	prompt := turnCtx.BuildPrompt()

	output, err := e.runModelInteraction(ctx, submission, prompt, emit, seq)
	if err != nil {
		return turnResult{}, nil, stageError{Stage: "model_interaction", Err: err}
	}

	processed := e.identifyTools(output)

	toolCtx := ctx
	toolCancel := func() {}
	if e.toolTimeout > 0 {
		toolCtx, toolCancel = context.WithTimeout(ctx, e.toolTimeout)
	}
	defer toolCancel()

	e.routeTools(toolCtx, submission, output.toolCalls, publishedCalls)

	results, err := e.executeTools(toolCtx, output.toolCalls, toolEvents)
	if err != nil {
		e.logRunTaskError(submission, "tool_execution", err, logger.Fields{
			"model":      turnCtx.Model,
			"sequence":   *seq,
			"tool_ids":   collectToolCallIDs(output.toolCalls),
			"tool_count": len(output.toolCalls),
		})
		return turnResult{}, nil, stageError{Stage: "tool_execution", Err: err}
	}
	for _, res := range results {
		call := findToolCall(output.toolCalls, res.ID)
		e.logToolResultError(submission, turnCtx, *seq, call, res)
	}
	e.publishPlanUpdates(ctx, submission, results, emit)
	processed = append(processed, processedFromToolResults(results)...)

	responses, itemsToRecord := processItems(processed)

	return turnResult{
		responses:     responses,
		itemsToRecord: itemsToRecord,
		finalContent:  deriveFinalContent(output.fullResponse, itemsToRecord),
	}, results, nil
}

// runModelInteraction 负责模型流式交互与输出收集，仅处理「模型交互」层。
// 对齐 Codex：拉取流式事件、发布增量输出、收集工具标记与 ResponseItem。
func (e *Engine) runModelInteraction(ctx context.Context, submission events.Submission, prompt agent.Prompt, emit events.EventPublisher, seq *int) (modelTurnOutput, error) {
	collector := newModelStreamCollector()
	seqStart := *seq

	err := e.streamPrompt(ctx, submission, prompt, func(evt agent.StreamEvent) {
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
	model := strings.TrimSpace(prompt.Model)
	encoded := encodeLLMLogJSON(llmResponseLogPayload{
		Model:    model,
		Response: output.fullResponse,
		Items:    output.items,
	})
	fields := logger.Fields{
		"type":                 "llm.response",
		"session":              submission.SessionID,
		"submission":           submission.ID,
		"model":                model,
		"sequence_start":       seqStart,
		"sequence_end":         *seq,
		"response_items":       len(output.items),
		"response_text_bytes":  len(output.fullResponse),
		"response_text_tokens": int64(echocontext.ApproxTokenCount(output.fullResponse)),
		"response_bytes":       encoded.Bytes,
		"response_tokens":      encoded.Tokens,
	}
	if encoded.Err == nil {
		fields["response_payload"] = encoded.Payload
	} else {
		fields["marshal_error"] = encoded.Err.Error()
	}
	in.WithFields(fields).Info("llm->agent response")

	return output, nil
}

// identifyTools 负责从模型输出中识别工具调用并构造历史记录项（「工具识别」层）。
func (e *Engine) identifyTools(output modelTurnOutput) []ProcessedResponseItem {
	processed := make([]ProcessedResponseItem, 0, len(output.items)+1)
	processed = append(processed, processedFromResponseItems(output.items)...)
	if strings.TrimSpace(output.fullResponse) != "" && !hasAssistantMessageItem(output.items) {
		processed = append(processed, ProcessedResponseItem{Item: echocontext.NewAssistantMessageItem(output.fullResponse)})
	}
	return processed
}

func hasAssistantMessageItem(items []echocontext.ResponseItem) bool {
	for _, item := range items {
		if item.Type == echocontext.ResponseItemTypeMessage && item.Message != nil && item.Message.Role == "assistant" {
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

func deriveFinalContent(fallback string, items []echocontext.ResponseItem) string {
	finalContent := echocontext.LastAssistantMessage(items)
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

type llmResponseLogPayload struct {
	Model    string                     `json:"model,omitempty"`
	Response string                     `json:"response,omitempty"`
	Items    []echocontext.ResponseItem `json:"items,omitempty"`
}

type llmEncodedLogJSON struct {
	Payload json.RawMessage
	Bytes   int
	Tokens  int64
	Err     error
}

func encodeLLMLogJSON(value any) llmEncodedLogJSON {
	raw, err := json.Marshal(value)
	if err != nil {
		return llmEncodedLogJSON{Err: err}
	}
	return llmEncodedLogJSON{
		Payload: json.RawMessage(raw),
		Bytes:   len(raw),
		Tokens:  approxTokensFromBytes(len(raw)),
	}
}

func approxTokensFromBytes(n int) int64 {
	if n <= 0 {
		return 0
	}
	// keep consistent with internal/context.ApproxTokenCount: ceil(len_bytes/4)
	return int64((n + 4 - 1) / 4)
}

type modelStreamCollector struct {
	builder   strings.Builder
	toolCalls []tools.ToolCall
	items     []echocontext.ResponseItem
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
	var item echocontext.ResponseItem
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

func toolCallFromResponseItem(item echocontext.ResponseItem) (tools.ToolCall, bool) {
	if item.Type != echocontext.ResponseItemTypeFunctionCall || item.FunctionCall == nil {
		return tools.ToolCall{}, false
	}
	if item.FunctionCall.Name == "" || item.FunctionCall.CallID == "" {
		return tools.ToolCall{}, false
	}
	return tools.ToolCall{
		ID:      item.FunctionCall.CallID,
		Name:    item.FunctionCall.Name,
		Payload: echocontext.NormalizeRawJSON(item.FunctionCall.Arguments),
	}, true
}

func textFromResponseItem(item echocontext.ResponseItem) string {
	switch item.Type {
	case echocontext.ResponseItemTypeMessage:
		if item.Message == nil {
			return ""
		}
		return echocontext.FlattenContentItems(item.Message.Content)
	case echocontext.ResponseItemTypeReasoning:
		if item.Reasoning == nil {
			return ""
		}
		return echocontext.FlattenReasoning(*item.Reasoning)
	default:
		return ""
	}
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

func (e *Engine) streamPrompt(ctx context.Context, submission events.Submission, prompt agent.Prompt, onEvent func(agent.StreamEvent)) error {
	out := llmOut()
	in := llmIn()
	messages := prompt.Messages
	model := strings.TrimSpace(prompt.Model)
	if model == "" {
		return errors.New("model not specified")
	}
	encoded := encodeLLMLogJSON(prompt)
	retryDelay := e.retryDelay
	if retryDelay <= 0 {
		retryDelay = time.Second
	}
	maxRetries := e.retries
	if maxRetries < 0 {
		maxRetries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		fields := logger.Fields{
			"type":              "llm.request",
			"session":           submission.SessionID,
			"submission":        submission.ID,
			"model":             model,
			"attempt":           attempt + 1,
			"messages":          len(messages),
			"tools":             len(prompt.Tools),
			"parallel_tools":    prompt.ParallelToolCalls,
			"output_schema_len": len(strings.TrimSpace(prompt.OutputSchema)),
			"request_bytes":     encoded.Bytes,
			"request_tokens":    encoded.Tokens,
		}
		if encoded.Err == nil {
			fields["request_payload"] = encoded.Payload
		} else {
			fields["marshal_error"] = encoded.Err.Error()
		}
		out.WithFields(fields).Info("agent->llm request")

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
			return nil
		}
		in.Errorf("llm->agent error attempt=%d model=%s err=%v", attempt+1, model, err)
		lastErr = err

		// Anthropic 偶发返回 api_error: "Internal Network Failure"。
		// 该错误通常是临时网络抖动，等待 1s 后重试一次可显著提升成功率。
		if isInternalNetworkFailure(err) && maxRetries < 1 {
			maxRetries = 1
		}
		if attempt >= maxRetries {
			break
		}
		if isInternalNetworkFailure(err) {
			in.Warnf("llm->agent retrying after internal network failure sleep=%s attempt=%d model=%s", retryDelay, attempt+1, model)
			timer := time.NewTimer(retryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return lastErr
}

func isInternalNetworkFailure(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "internal network failure")
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

func tailPreviewForLog(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit < 3 {
		return text[len(text)-limit:]
	}
	return "..." + text[len(text)-(limit-3):]
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
				if toolEvt.Type == "item.completed" {
					e.clearToolCallContext(toolEvt.Result.ID)
				}
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
		if _, hasDeadline := ctx.Deadline(); hasDeadline {
			// Respect the existing deadline (typically already e.toolTimeout from runTurn).
			// This keeps a single source of truth for tool cancellation timing.
		} else {
			waitCtx, cancel = context.WithTimeout(ctx, e.toolTimeout)
		}
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
func processItems(items []ProcessedResponseItem) ([]echocontext.ResponseInputItem, []echocontext.ResponseItem) {
	responses := make([]echocontext.ResponseInputItem, 0, len(items))
	record := make([]echocontext.ResponseItem, 0, len(items))
	for _, item := range items {
		record = append(record, item.Item)
		if item.Response != nil {
			responses = append(responses, *item.Response)
		}
	}
	return responses, record
}

func (e *Engine) recordConversationItems(sessionID string, turnCtx *echocontext.TurnContext, items []echocontext.ResponseItem) {
	if len(items) == 0 {
		return
	}
	e.contexts.AppendResponseItems(sessionID, items)
	turnCtx.ResponseHistory = append(turnCtx.ResponseHistory, items...)
	turnCtx.History = append(turnCtx.History, echocontext.ResponseItemsToAgentMessages(items)...)
}

func (e *Engine) tokenLimitReached(turnCtx echocontext.TurnContext) bool {
	window, ok := echocontext.ContextWindowForModel(turnCtx.Model)
	if !ok || window <= 0 {
		return false
	}
	limit := echocontext.DefaultAutoCompactLimit(window)
	if limit <= 0 {
		return false
	}
	estimated := echocontext.EstimatePromptTokens(turnCtx.BuildPrompt())
	return estimated >= limit
}

func (e *Engine) runInlineAutoCompactTask(ctx context.Context, sessionID string, turnCtx echocontext.TurnContext) (echocontext.TurnContext, bool) {
	if e.client == nil {
		return turnCtx, false
	}
	newHistory, trimmed, _, err := echocontext.CompactConversationHistory(ctx, e.client, turnCtx, turnCtx.ResponseHistory)
	if err != nil {
		log.Warnf("auto-compaction failed model=%s trimmed=%d err=%v", turnCtx.Model, trimmed, err)
		return turnCtx, false
	}
	e.contexts.ReplaceHistory(sessionID, newHistory)
	turnCtx.ResponseHistory = newHistory
	turnCtx.History = echocontext.ResponseItemsToAgentMessages(newHistory)
	log.Infof("auto-compaction completed model=%s trimmed=%d new_items=%d", turnCtx.Model, trimmed, len(newHistory))
	return turnCtx, true
}
