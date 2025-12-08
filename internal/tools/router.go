package tools

import "fmt"

// BuildCallFromMarker 将模型输出的 ToolCallMarker 转换为 ToolCall。
func BuildCallFromMarker(marker ToolCallMarker) (ToolCall, error) {
	if marker.Tool == "" {
		return ToolCall{}, fmt.Errorf("missing tool name")
	}
	if marker.ID == "" {
		return ToolCall{}, fmt.Errorf("missing tool id")
	}
	return ToolCall{
		ID:      marker.ID,
		Name:    marker.Tool,
		Payload: marker.Args,
	}, nil
}
