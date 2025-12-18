package tools

import (
	"encoding/json"
)

type ToolKind string

const (
	ToolCommand    ToolKind = "command_execution"
	ToolApplyPatch ToolKind = "file_change"
	ToolFileRead   ToolKind = "file_read"
	ToolSearch     ToolKind = "file_search"
	ToolPlanUpdate ToolKind = "plan_update"
)

// ToolCall 表示一次工具调用的标准化结构。
// Name 与 ID 对应模型输出的工具名与 call id，Payload 是原始 JSON 参数。
type ToolCall struct {
	ID      string
	Name    string
	Payload json.RawMessage
}

// ToolRequest 兼容旧接口，并用于将 UI / 测试输入转换为 ToolCall。
// 新字段 Name/Payload 优先；旧字段 Command/Patch/Path/Query 会被封装为 Payload。
type ToolRequest struct {
	ID      string
	Name    string
	Kind    ToolKind
	Payload json.RawMessage
	Command string
	Patch   string
	Path    string
	Query   string
}

type ToolResult struct {
	ID     string
	Kind   ToolKind
	Status string // started|updated|completed|error
	Output string
	// Diff 用于 file_change(apply_patch) 的变更内容展示（例如 unified diff 或 begin_patch 格式）。
	Diff     string
	Error    string
	ExitCode int
	Path     string
	Command  string
	Plan     []PlanItem
	// Explanation 是 update_plan 的可选说明。
	Explanation string
}

type ToolEvent struct {
	Type   string // item.started|item.updated|item.completed
	Result ToolResult
}
