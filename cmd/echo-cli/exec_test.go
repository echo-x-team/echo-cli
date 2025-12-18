package main

import (
	"testing"

	"echo-cli/internal/tools"
)

func TestToolEventToJSONItem(t *testing.T) {
	ev := tools.ToolEvent{
		Type: "item.completed",
		Result: tools.ToolResult{
			ID:      "1",
			Kind:    tools.ToolCommand,
			Status:  "completed",
			Output:  "ok",
			Command: "echo ok",
		},
	}
	jsonEvt, ok := toolEventToJSON(ev)
	if !ok {
		t.Fatalf("expected conversion")
	}
	if jsonEvt.Item == nil || jsonEvt.Item.Type != string(tools.ToolCommand) || jsonEvt.Item.Status != "completed" {
		t.Fatalf("unexpected item event %+v", jsonEvt.Item)
	}
}
