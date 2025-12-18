package tools

import (
	"encoding/json"
)

// ToCall 将 ToolRequest 转换为标准 ToolCall，兼容旧字段。
func (r ToolRequest) ToCall() ToolCall {
	if len(r.Payload) > 0 || r.Name != "" {
		return ToolCall{ID: r.ID, Name: r.effectiveName(), Payload: r.Payload}
	}

	var payload map[string]any
	switch r.Kind {
	case ToolCommand:
		payload = map[string]any{"command": r.Command}
	case ToolApplyPatch:
		payload = map[string]any{"patch": r.Patch, "path": r.Path}
	case ToolFileRead:
		payload = map[string]any{"path": r.Path}
	case ToolSearch:
		payload = map[string]any{"query": r.Query}
	default:
		payload = map[string]any{}
	}
	raw, _ := json.Marshal(payload)
	return ToolCall{
		ID:      r.ID,
		Name:    r.effectiveName(),
		Payload: raw,
	}
}

func (r ToolRequest) effectiveName() string {
	if r.Name != "" {
		return r.Name
	}
	switch r.Kind {
	case ToolCommand:
		return "exec_command"
	case ToolApplyPatch:
		return "apply_patch"
	case ToolFileRead:
		return "file_read"
	case ToolSearch:
		return "file_search"
	default:
		return ""
	}
}
