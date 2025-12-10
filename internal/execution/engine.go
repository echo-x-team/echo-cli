package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

// Options 定义引擎的可注入依赖。
type Options struct {
	Manager        *events.Manager
	ManagerConfig  events.ManagerConfig
	Client         agent.ModelClient
	Bus            *events.Bus
	Defaults       SessionDefaults
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
}

type taskHandle struct {
	cancel context.CancelFunc
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

// runTask 执行单个任务的完整对话循环
// 该函数实现了 LLM 交互的核心流程：生成响应 -> 检测工具调用 -> 收集工具结果 -> 生成新响应
//
// 参数说明：
//   - ctx: 上下文对象，用于控制超时和取消
//   - submission: 用户提交的任务信息，包含会话ID、输入内容等
//   - state: 当前回合的状态，包含上下文和历史记录
//   - emit: 事件发布器，用于推送输出事件
//
// 返回值：
//   - error: 执行过程中的错误，nil 表示成功完成
//
// 优化建议：
//  1. 考虑限制最大循环次数，避免无限循环
//  2. 可以添加工具调用次数统计和限制
//  3. 考虑实现响应内容的缓存机制
//  4. 可以优化内存使用，避免历史记录无限增长
func (e *Engine) runTask(ctx context.Context, submission events.Submission, state TurnState, emit events.EventPublisher) error {
	seq := 0
	turnCtx := state.Context
	toolEvents, stopTools := e.subscribeToolEvents(ctx)
	defer stopTools()
	publishedMarkers := map[string]struct{}{}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		prompt := turnCtx.BuildPrompt()
		logPrompt(submission, prompt)

		full, markers, err := e.runTurnOnce(ctx, submission, prompt, emit, &seq)
		if err != nil {
			return err
		}

		e.dispatchToolMarkers(markers, publishedMarkers)

		if full != "" {
			assistant := agent.Message{Role: agent.RoleAssistant, Content: full}
			turnCtx.History = append(turnCtx.History, assistant)
			e.contexts.AppendAssistant(submission.SessionID, full)
		}

		if len(markers) == 0 {
			_ = emit.Publish(ctx, events.Event{
				Type:         events.EventAgentOutput,
				SubmissionID: submission.ID,
				SessionID:    submission.SessionID,
				Timestamp:    time.Now(),
				Payload: events.AgentOutput{
					Content:  full,
					Final:    true,
					Sequence: seq,
				},
				Metadata: submission.Metadata,
			})
			return nil
		}

		results, err := e.collectToolResults(ctx, markers, toolEvents)
		if err != nil {
			return err
		}

		toolMsgs := formatToolResults(results)
		if len(toolMsgs) > 0 {
			turnCtx.History = append(turnCtx.History, toolMsgs...)
			e.contexts.AppendMessages(submission.SessionID, toolMsgs)
		}
	}
}

func logPrompt(submission events.Submission, prompt Prompt) {
	if encoded, err := json.Marshal(prompt); err == nil {
		log.Infof("prompt session=%s submission=%s payload=%s", submission.SessionID, submission.ID, string(encoded))
	} else {
		log.Warnf("prompt session=%s submission=%s model=%s marshal_error=%v", submission.SessionID, submission.ID, prompt.Model, err)
	}
	log.Infof("messages session=%s submission=%s model=%s count=%d", submission.SessionID, submission.ID, prompt.Model, len(prompt.Messages))
	for i, msg := range prompt.Messages {
		log.Infof("message[%d] role=%s content=%s", i, msg.Role, sanitizeLogText(msg.Content))
	}
}

// runTurnOnce 对齐 Codex 的“模型流 → 事件 → 工具调度”流程：流式收集输出、发布增量事件并返回工具标记。
func (e *Engine) runTurnOnce(ctx context.Context, submission events.Submission, prompt Prompt, emit events.EventPublisher, seq *int) (string, []tools.ToolCallMarker, error) {
	var builder strings.Builder

	err := e.streamPrompt(ctx, prompt, func(chunk string) {
		if chunk == "" {
			return
		}
		builder.WriteString(chunk)

		_ = emit.Publish(ctx, events.Event{
			Type:         events.EventAgentOutput,
			SubmissionID: submission.ID,
			SessionID:    submission.SessionID,
			Timestamp:    time.Now(),
			Payload: events.AgentOutput{
				Content:  chunk,
				Sequence: *seq,
			},
			Metadata: submission.Metadata,
		})
		*seq++
	})
	if err != nil {
		return "", nil, err
	}

	full := builder.String()
	markers, parseErr := tools.ParseMarkers(full)
	if parseErr != nil {
		log.Warnf("parse markers session=%s submission=%s err=%v", submission.SessionID, submission.ID, parseErr)
	}
	return full, markers, nil
}

func (e *Engine) streamPrompt(ctx context.Context, prompt Prompt, onChunk func(string)) error {
	messages := prompt.Messages
	model := strings.TrimSpace(prompt.Model)
	if model == "" {
		return errors.New("model not specified")
	}
	var lastErr error
	for attempt := 0; attempt <= e.retries; attempt++ {
		llmLog.Infof("-> request attempt=%d model=%s messages=%d", attempt+1, model, len(messages))
		for i, msg := range messages {
			llmLog.Infof("-> message[%d] role=%s content=%s", i, msg.Role, sanitizeLogText(msg.Content))
		}
		ctxRun, cancel := context.WithTimeout(ctx, e.requestTimeout)
		err := e.client.Stream(ctxRun, messages, model, func(chunk string) {
			onChunk(chunk)
		})
		cancel()
		if err == nil {
			llmLog.Infof("<- stream completed attempt=%d model=%s", attempt+1, model)
			return nil
		}
		llmLog.Errorf("!! error attempt=%d model=%s err=%v", attempt+1, model, err)
		lastErr = err
	}
	return lastErr
}

func sanitizeLogText(text string) string {
	text = strings.ReplaceAll(text, "\n", `\n`)
	text = strings.ReplaceAll(text, "\r", `\r`)
	return text
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
				_ = e.manager.PublishEvent(forwardCtx, events.Event{
					Type:      events.EventToolEvent,
					Timestamp: time.Now(),
					Payload:   toolEvt,
					Metadata:  meta,
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

func (e *Engine) collectToolResults(ctx context.Context, markers []tools.ToolCallMarker, events <-chan tools.ToolEvent) ([]tools.ToolResult, error) {
	if len(markers) == 0 {
		return nil, nil
	}
	if events == nil {
		return nil, errors.New("tool event stream not configured")
	}
	pending := make(map[string]tools.ToolCallMarker, len(markers))
	order := make([]string, 0, len(markers))
	for _, marker := range markers {
		pending[marker.ID] = marker
		order = append(order, marker.ID)
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
			if ev.Type == "approval.completed" && strings.Contains(ev.Reason, "denied") {
				if ev.Result.Status == "" {
					ev.Result.Status = "error"
				}
				if ev.Result.Error == "" {
					ev.Result.Error = ev.Reason
				}
				results[ev.Result.ID] = ev.Result
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

func (e *Engine) dispatchToolMarkers(markers []tools.ToolCallMarker, seen map[string]struct{}) {
	if e.bus == nil || len(markers) == 0 {
		return
	}
	for _, marker := range markers {
		if marker.Tool == "" || marker.ID == "" {
			continue
		}
		if seen != nil {
			if _, ok := seen[marker.ID]; ok {
				continue
			}
			seen[marker.ID] = struct{}{}
		}
		e.bus.Publish(marker)
	}
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
