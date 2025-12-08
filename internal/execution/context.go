package execution

import (
	"sync"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
)

// SessionDefaults 定义新的会话默认上下文。
type SessionDefaults struct {
	Model           string
	System          string
	OutputSchema    string
	Instructions    []string
	ReasoningEffort string
	ReviewMode      bool
	Language        string
}

type sessionState struct {
	model           string
	system          string
	outputSchema    string
	instructions    []string
	reasoningEffort string
	reviewMode      bool
	language        string
	history         []agent.Message
}

// TurnContext 聚合生成提示词所需的上下文数据。
type TurnContext struct {
	Model           string
	System          string
	Instructions    []string
	OutputSchema    string
	Language        string
	ReasoningEffort string
	ReviewMode      bool            // 是否启用审查模式
	Attachments     []agent.Message // 附件内容（文件、图片等）
	History         []agent.Message // 纯对话历史（不包括系统注入的内容）
}

// TurnState 描述一次回合所需的上下文与模型信息。
type TurnState struct {
	Model   string
	Context TurnContext
}

// ContextManager 负责维护会话上下文与历史。
type ContextManager struct {
	mu       sync.Mutex
	defaults SessionDefaults
	sessions map[string]*sessionState
}

// NewContextManager 创建上下文管理器。
func NewContextManager(defaults SessionDefaults) *ContextManager {
	return &ContextManager{
		defaults: SessionDefaults{
			Model:           defaults.Model,
			System:          defaults.System,
			OutputSchema:    defaults.OutputSchema,
			Instructions:    cloneStrings(defaults.Instructions),
			ReasoningEffort: defaults.ReasoningEffort,
			ReviewMode:      defaults.ReviewMode,
			Language:        defaults.Language,
		},
		sessions: map[string]*sessionState{},
	}
}

// PrepareTurn 根据 InputContext 与用户输入更新历史并返回提示上下文。
func (m *ContextManager) PrepareTurn(sessionID string, ctx events.InputContext, items []events.InputMessage) TurnState {
	m.mu.Lock()
	state := m.ensureSession(sessionID, ctx)

	// 构建纯对话历史（不包含系统注入的内容）
	history := make([]agent.Message, 0, len(state.history)+len(items))
	history = append(history, state.history...) // existing history first

	// 转换当前用户输入为消息
	userMessages := toAgentMessages(items)
	history = append(history, userMessages...)

	// 更新会话状态的历史记录
	state.history = append(state.history, userMessages...)

	// 从会话状态获取默认值
	model := state.model
	system := state.system
	outputSchema := state.outputSchema
	instructions := cloneStrings(state.instructions)
	reasoningEffort := state.reasoningEffort
	reviewMode := state.reviewMode
	language := state.language

	// InputContext 中的值会覆盖会话默认值
	if ctx.Model != "" {
		model = ctx.Model
	}
	if ctx.System != "" {
		system = ctx.System
	}
	if ctx.OutputSchema != "" {
		outputSchema = ctx.OutputSchema
	}
	if ctx.Instructions != nil {
		instructions = cloneStrings(ctx.Instructions)
	}
	if ctx.ReasoningEffort != "" {
		reasoningEffort = ctx.ReasoningEffort
	}
	if ctx.Language != "" {
		language = ctx.Language
	}
	// 注意：ReviewMode 采用或逻辑，只要任一处为 true 就启用
	if ctx.ReviewMode {
		reviewMode = true
	}

	// 转换附件为 agent.Message 格式
	attachments := toAgentMessages(ctx.Attachments)

	m.mu.Unlock()

	return TurnState{
		Model: model,
		Context: TurnContext{
			Model:           model,
			System:          system,
			OutputSchema:    outputSchema,
			Instructions:    instructions,
			ReasoningEffort: reasoningEffort,
			ReviewMode:      reviewMode,
			Language:        language,
			Attachments:     attachments,
			History:         history, // 纯对话历史，不包含系统注入的内容
		},
	}
}

// AppendAssistant 将助手输出写入历史。
func (m *ContextManager) AppendAssistant(sessionID string, content string) {
	if content == "" {
		return
	}
	m.AppendMessages(sessionID, []agent.Message{{Role: agent.RoleAssistant, Content: content}})
}

// AppendMessages 追加任意角色的消息到历史。
func (m *ContextManager) AppendMessages(sessionID string, msgs []agent.Message) {
	if len(msgs) == 0 {
		return
	}
	m.mu.Lock()
	state := m.ensureSession(sessionID, events.InputContext{})
	state.history = append(state.history, msgs...)
	m.mu.Unlock()
}

// History 返回会话历史的拷贝。
func (m *ContextManager) History(sessionID string) []agent.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.sessions[sessionID]
	if state == nil {
		return nil
	}
	history := make([]agent.Message, len(state.history))
	copy(history, state.history)
	return history
}

func (m *ContextManager) ensureSession(sessionID string, ctx events.InputContext) *sessionState {
	state, ok := m.sessions[sessionID]
	if ok {
		return state
	}

	// 从默认值开始初始化
	model := m.defaults.Model
	system := m.defaults.System
	outputSchema := m.defaults.OutputSchema
	instructions := cloneStrings(m.defaults.Instructions)
	reasoningEffort := m.defaults.ReasoningEffort
	reviewMode := m.defaults.ReviewMode
	language := m.defaults.Language

	// InputContext 中的值会覆盖默认值
	if ctx.Model != "" {
		model = ctx.Model
	}
	if ctx.System != "" {
		system = ctx.System
	}
	if ctx.OutputSchema != "" {
		outputSchema = ctx.OutputSchema
	}
	if ctx.Instructions != nil {
		instructions = cloneStrings(ctx.Instructions)
	}
	if ctx.ReasoningEffort != "" {
		reasoningEffort = ctx.ReasoningEffort
	}
	if ctx.Language != "" {
		language = ctx.Language
	}
	if ctx.ReviewMode {
		reviewMode = true
	}

	state = &sessionState{
		model:           model,
		system:          system,
		outputSchema:    outputSchema,
		instructions:    instructions,
		reasoningEffort: reasoningEffort,
		reviewMode:      reviewMode,
		language:        language,
	}
	m.sessions[sessionID] = state
	return state
}

func toAgentMessages(items []events.InputMessage) []agent.Message {
	msgs := make([]agent.Message, 0, len(items))
	for _, item := range items {
		msgs = append(msgs, agent.Message{
			Role:    agent.Role(item.Role),
			Content: item.Content,
		})
	}
	return msgs
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
