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
	// seq 用于跟踪输出流序列号，确保客户端能够正确排序和显示内容
	seq := 0
	// turnCtx 包含当前回合的对话上下文和历史记录
	turnCtx := state.Context
	// 订阅工具执行事件，用于异步接收工具执行结果
	// toolEvents 是一个通道，用于接收工具执行的事件通知
	toolEvents, stopTools := e.subscribeToolEvents(ctx)
	// 确保在函数退出时清理资源，避免 goroutine 泄露
	defer stopTools()

	// 主循环：持续进行对话，直到不再有工具调用
	// 这个循环实现了类似 ReAct 模式的推理-行动循环
	for {
		// builder 用于积累 LLM 返回的完整响应内容
		var builder strings.Builder

		// 构建发送给 LLM 的提示词，包含系统提示、历史对话和当前用户输入
		prompt := turnCtx.BuildPrompt()

		// 记录发送给 LLM 的完整提示词，便于调试和日志分析
		if encoded, err := json.Marshal(prompt); err == nil {
			log.Infof("prompt session=%s submission=%s payload=%s", submission.SessionID, submission.ID, string(encoded))
		} else {
			log.Warnf("prompt session=%s submission=%s model=%s marshal_error=%v", submission.SessionID, submission.ID, prompt.Model, err)
		}
		log.Infof("messages session=%s submission=%s model=%s count=%d", submission.SessionID, submission.ID, prompt.Model, len(prompt.Messages))
		for i, msg := range prompt.Messages {
			log.Infof("message[%d] role=%s content=%s", i, msg.Role, sanitizeLogText(msg.Content))
		}

		// 流式调用 LLM，实时接收并处理响应内容
		// 优点：用户体验更好，可以看到实时的生成过程
		err := e.streamPrompt(ctx, prompt, func(chunk string) {
			// 跳过空块，避免发送无意义的事件
			if chunk == "" {
				return
			}

			// 累积完整的响应内容
			builder.WriteString(chunk)

			// 实时发布输出事件，让前端能够流式显示内容
			// 注意：这里忽略发布错误，因为我们更关注 LLM 的响应
			_ = emit.Publish(ctx, events.Event{
				Type:         events.EventAgentOutput,
				SubmissionID: submission.ID,        // 关联到原始提交
				SessionID:    submission.SessionID, // 关联到会话
				Timestamp:    time.Now(),
				Payload: events.AgentOutput{
					Content:  chunk, // 当前响应块
					Sequence: seq,   // 序列号，用于排序
				},
				Metadata: submission.Metadata, // 保留原始元数据
			})

			// 递增序列号，为下一个块做准备
			seq++
		})

		// 如果 LLM 调用出错，直接返回错误
		// 可能的错误：网络问题、API限制、模型不可用等
		if err != nil {
			return err
		}

		// 获取 LLM 返回的完整响应内容
		full := builder.String()

		// 如果有内容，将其添加到对话历史中
		// 这样下一次 LLM 调用时能够看到本轮的响应
		if full != "" {
			turnCtx.History = append(turnCtx.History, agent.Message{Role: agent.RoleAssistant, Content: full})
			// 同时更新全局上下文管理器，保持状态同步
			e.contexts.AppendAssistant(submission.SessionID, full)
		}

		// 解析响应中的工具调用标记
		// 工具调用格式类似于：<function_calls>...<function_calls>
		// 如果检测到工具调用，需要执行工具并收集结果
		markers, _ := tools.ParseMarkers(full)

		// 如果没有工具调用，说明对话已完成
		if len(markers) == 0 {
			// 发送最终输出事件，标记响应结束
			_ = emit.Publish(ctx, events.Event{
				Type:         events.EventAgentOutput,
				SubmissionID: submission.ID,
				SessionID:    submission.SessionID,
				Timestamp:    time.Now(),
				Payload: events.AgentOutput{
					Content:  full, // 完整的最终响应
					Final:    true, // 标记为最终输出
					Sequence: seq,  // 最后的序列号
				},
				Metadata: submission.Metadata,
			})
			// 对话完成，退出循环
			return nil
		}

		// 等待并收集所有工具的执行结果
		// 这是一个阻塞操作，会等待所有工具完成或超时
		results, err := e.collectToolResults(ctx, markers, toolEvents)
		if err != nil {
			// 工具执行失败，返回错误
			// 可能的原因：工具超时、执行错误、工具不存在等
			return err
		}

		// 将工具执行结果格式化为消息格式
		// 这样可以将工具结果作为新的用户消息发送给 LLM
		toolMsgs := formatToolResults(results)

		// 如果有工具结果，将其添加到对话历史
		// 这让 LLM 能够基于工具结果进行下一步推理
		if len(toolMsgs) > 0 {
			turnCtx.History = append(turnCtx.History, toolMsgs...)
			// 更新全局上下文管理器
			e.contexts.AppendMessages(submission.SessionID, toolMsgs)

			// 继续下一轮循环，让 LLM 基于工具结果生成新的响应
			// 这实现了类似 ReAct 的推理模式：思考 -> 行动 -> 观察 -> 思考
		}
	}
}

func (e *Engine) streamPrompt(ctx context.Context, prompt Prompt, onChunk func(string)) error {
	messages := prompt.Messages
	model := strings.TrimSpace(prompt.Model)
	if model == "" {
		return errors.New("model not specified")
	}
	var lastErr error
	for attempt := 0; attempt <= e.retries; attempt++ {
		logger.Request(model, agent.ToLLMMessages(messages), attempt+1)
		ctxRun, cancel := context.WithTimeout(ctx, e.requestTimeout)
		chunkIdx := 0
		err := e.client.Stream(ctxRun, messages, model, func(chunk string) {
			onChunk(chunk)
			e.publishToolMarkers(chunk)
			if chunk == "" {
				return
			}
			logger.StreamChunk(model, chunk, chunkIdx)
			chunkIdx++
		})
		cancel()
		if err == nil {
			logger.StreamComplete(model, attempt+1)
			return nil
		}
		logger.Error(model, err, attempt+1)
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

func (e *Engine) publishToolMarkers(chunk string) {
	if e.bus == nil {
		return
	}
	markers, err := tools.ParseMarkers(chunk)
	if err != nil || len(markers) == 0 {
		return
	}
	for _, marker := range markers {
		e.bus.Publish(marker)
	}
}

func formatToolResults(results []tools.ToolResult) []agent.Message {
	msgs := make([]agent.Message, 0, len(results))
	for _, res := range results {
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
