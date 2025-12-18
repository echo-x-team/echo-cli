package anthropic

import (
	"encoding/json"
	"testing"

	"echo-cli/internal/agent"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

func TestBuildMessageParamsRegistersToolsAndEncodesToolBlocks(t *testing.T) {
	prompt := agent.Prompt{
		Model: "claude-test",
		Tools: agent.DefaultTools(),
		Messages: []agent.Message{
			{Role: agent.RoleSystem, Content: "system"},
			{
				Role: agent.RoleAssistant,
				ToolUse: &agent.ToolUse{
					ID:    "toolu_1",
					Name:  "command",
					Input: json.RawMessage(`{"command":"echo hi"}`),
				},
				Content: "debug text should not be sent as text block",
			},
			{
				Role: agent.RoleUser,
				ToolResult: &agent.ToolResult{
					ToolUseID: "toolu_1",
					Content:   "ok",
					IsError:   false,
				},
			},
		},
	}

	params := buildMessageParams(prompt, anthropic.Model("claude-test"))

	if len(params.Tools) != len(prompt.Tools) {
		t.Fatalf("tools count = %d, want %d", len(params.Tools), len(prompt.Tools))
	}
	for i, tool := range params.Tools {
		if tool.OfTool == nil {
			t.Fatalf("tools[%d] missing OfTool", i)
		}
		if tool.OfTool.Name == "" {
			t.Fatalf("tools[%d] missing name", i)
		}
		if len(tool.OfTool.InputSchema.Required) == 0 {
			t.Fatalf("tools[%d] missing required schema fields", i)
		}
	}

	if len(params.System) != 1 || params.System[0].Text != "system" {
		t.Fatalf("system = %#v, want single system block", params.System)
	}
	if len(params.Messages) != 2 {
		t.Fatalf("messages count = %d, want 2", len(params.Messages))
	}
	if got := params.Messages[0].Role; got != anthropic.MessageParamRoleAssistant {
		t.Fatalf("messages[0].role = %s, want assistant", got)
	}
	if len(params.Messages[0].Content) != 1 || params.Messages[0].Content[0].OfToolUse == nil {
		t.Fatalf("messages[0] should contain tool_use block, got %#v", params.Messages[0].Content)
	}
	if params.Messages[0].Content[0].OfToolUse.ID != "toolu_1" || params.Messages[0].Content[0].OfToolUse.Name != "command" {
		t.Fatalf("unexpected tool_use payload: %#v", params.Messages[0].Content[0].OfToolUse)
	}

	if got := params.Messages[1].Role; got != anthropic.MessageParamRoleUser {
		t.Fatalf("messages[1].role = %s, want user", got)
	}
	if len(params.Messages[1].Content) != 1 || params.Messages[1].Content[0].OfToolResult == nil {
		t.Fatalf("messages[1] should contain tool_result block, got %#v", params.Messages[1].Content)
	}
	toolResult := params.Messages[1].Content[0].OfToolResult
	if toolResult.ToolUseID != "toolu_1" {
		t.Fatalf("tool_result.tool_use_id = %q, want toolu_1", toolResult.ToolUseID)
	}
	if len(toolResult.Content) != 1 || toolResult.Content[0].OfText == nil || toolResult.Content[0].OfText.Text != "ok" {
		t.Fatalf("tool_result.content = %#v, want text ok", toolResult.Content)
	}
}
