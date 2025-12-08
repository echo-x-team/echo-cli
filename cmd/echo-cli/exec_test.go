package main

import (
	"testing"

	"echo-cli/internal/tools"
)

func TestToolEventToJSONApproval(t *testing.T) {
	ev := tools.ToolEvent{
		Type: "approval.requested",
		Result: tools.ToolResult{
			ID:   "1",
			Kind: tools.ToolCommand,
		},
		Reason: "requires approval",
	}
	jsonEvt, ok := toolEventToJSON(ev)
	if !ok {
		t.Fatalf("expected conversion")
	}
	if jsonEvt.Approval == nil || jsonEvt.Approval.Action != "command" || jsonEvt.Approval.Status != "requested" {
		t.Fatalf("unexpected approval event %+v", jsonEvt.Approval)
	}
}
