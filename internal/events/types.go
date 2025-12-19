package events

import "time"

// Priority 描述提交的优先级。默认使用 PriorityNormal。
type Priority int

const (
	PriorityLow Priority = iota + 1
	PriorityNormal
	PriorityHigh
)

// OperationKind 表示提交的操作类型。
type OperationKind string

const (
	OperationUserInput        OperationKind = "user_input"
	OperationInterrupt        OperationKind = "interrupt"
	OperationApprovalDecision OperationKind = "approval_decision"
)

// InputMessage 代表一次用户输入（或上下文中的历史消息）。
type InputMessage struct {
	Role    string
	Content string
}

// InputContext 为提交提供会话和额外元数据。
type InputContext struct {
	SessionID       string
	Metadata        map[string]string
	Model           string
	System          string
	OutputSchema    string
	Instructions    []string
	Language        string
	ReasoningEffort string
	ReviewMode      bool           // 是否启用审查模式
	Attachments     []InputMessage // 附件内容
}

// UserInputOperation 描述用户输入操作。
type UserInputOperation struct {
	Items   []InputMessage
	Context InputContext
}

// ApprovalDecisionOperation 描述一次审批决策（approve/deny）。
type ApprovalDecisionOperation struct {
	ApprovalID string
	Approved   bool
}

// Operation 描述一次提交的操作载荷。
type Operation struct {
	Kind             OperationKind
	UserInput        *UserInputOperation
	ApprovalDecision *ApprovalDecisionOperation
}

// Submission 代表进入 SQ 的提交。
type Submission struct {
	ID        string
	Operation Operation
	Timestamp time.Time
	Priority  Priority
	SessionID string
	Metadata  map[string]string
}

// EventType 描述 EQ 中分发的事件类型。
type EventType string

const (
	EventSubmissionAccepted EventType = "submission.accepted"
	EventTaskStarted        EventType = "task.started"
	// EventTaskSummary 在每次 runTurn 结束后发出，用于汇总本轮完成内容与问题（成功/失败/中断均会发出）。
	EventTaskSummary   EventType = "task.summary"
	EventTaskCompleted EventType = "task.completed"
	EventAgentOutput   EventType = "agent.output"
	EventError         EventType = "task.error"
	EventToolEvent     EventType = "tool.event"
	// EventPlanUpdated 表示 update_plan 工具成功后生成的新计划快照。
	EventPlanUpdated EventType = "plan.updated"
)

// AgentOutput 表示智能体的输出（可流式）。
type AgentOutput struct {
	Content  string
	Final    bool
	Sequence int
	Metadata map[string]string
}

// TaskResult 描述任务完成状态。
type TaskResult struct {
	Status string
	Error  string
}

// TaskSummary 描述一次 turn 结束后的汇总信息。
// Text 为面向用户的汇总文本（包含完成工作/问题）；结构化字段用于 exec/TUI 做更丰富的渲染或后续扩展。
type TaskSummary struct {
	Status     string `json:"status"` // completed|failed|interrupted|timeout
	Text       string `json:"text"`
	Error      string `json:"error,omitempty"`
	ExitStage  string `json:"exit_stage,omitempty"`
	ExitReason string `json:"exit_reason,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Model      string `json:"model,omitempty"`

	InputTokens       int64 `json:"input_tokens,omitempty"`
	CachedInputTokens int64 `json:"cached_input_tokens,omitempty"`
	OutputTokens      int64 `json:"output_tokens,omitempty"`
}

// Event 是 EQ 中传递的唯一消息格式。
// Payload 的具体结构由 Type 决定；详见 docs/EQ_EVENTS.md。
type Event struct {
	Type         EventType
	SubmissionID string
	SessionID    string
	Timestamp    time.Time
	Payload      any
	Metadata     map[string]string
}
