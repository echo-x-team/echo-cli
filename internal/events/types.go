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
	OperationUserInput OperationKind = "user_input"
	OperationInterrupt OperationKind = "interrupt"
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

// Operation 描述一次提交的操作载荷。
type Operation struct {
	Kind      OperationKind
	UserInput *UserInputOperation
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
	EventTaskCompleted      EventType = "task.completed"
	EventAgentOutput        EventType = "agent.output"
	EventError              EventType = "task.error"
	EventToolEvent          EventType = "tool.event"
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

// Event 是 EQ 中传递的消息。
type Event struct {
	Type         EventType
	SubmissionID string
	SessionID    string
	Timestamp    time.Time
	Payload      any
	Metadata     map[string]string
}
