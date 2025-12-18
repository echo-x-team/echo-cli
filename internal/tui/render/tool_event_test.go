package render

import (
	"strings"
	"testing"

	"echo-cli/internal/agent"
	"echo-cli/internal/tools"
)

func TestTranscript_AppendToolBlock_NotPersisted(t *testing.T) {
	tr := NewTranscript(60)
	tr.AppendUser("hi")
	tr.AppendToolBlock("✓ command_execution completed\n  └ output:\n    ok")
	tr.FinalizeAssistant("done")

	history := tr.Messages()
	if len(history) != 2 {
		t.Fatalf("expected 2 persisted messages, got %d: %#v", len(history), history)
	}
	if history[0].Role != agent.RoleUser || !strings.Contains(history[0].Content, "hi") {
		t.Fatalf("unexpected history[0]=%#v", history[0])
	}
	if history[1].Role != agent.RoleAssistant || !strings.Contains(history[1].Content, "done") {
		t.Fatalf("unexpected history[1]=%#v", history[1])
	}

	viewLines := LinesToPlainStrings(tr.RenderViewLines(80))
	viewText := strings.Join(viewLines, "\n")
	if !strings.Contains(viewText, "command_execution") {
		t.Fatalf("expected tool block in view, got:\n%s", viewText)
	}
}

func TestFormatToolEventBlock_ItemCompletedIncludesOutput(t *testing.T) {
	got := FormatToolEventBlock(tools.ToolEvent{
		Type: "item.completed",
		Result: tools.ToolResult{
			Kind:    tools.ToolCommand,
			Status:  "completed",
			Command: "echo hi",
			Output:  "hi\nthere",
		},
	})
	if !strings.Contains(got, "output:") || !strings.Contains(got, "hi") {
		t.Fatalf("expected output in block, got:\n%s", got)
	}
}

func TestFormatToolEventBlock_FileChangeShowsDiff(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: a.txt\n@@\n-old\n+new\n*** End Patch\n"

	got := FormatToolEventBlock(tools.ToolEvent{
		Type: "item.completed",
		Result: tools.ToolResult{
			Kind:   tools.ToolApplyPatch,
			Status: "completed",
			Path:   "a.txt",
			Diff:   patch,
		},
	})
	if !strings.Contains(got, "diff:") || !strings.Contains(got, "+new") {
		t.Fatalf("expected diff in block, got:\n%s", got)
	}
	if strings.Contains(got, "output:") {
		t.Fatalf("expected file_change to label as diff, got:\n%s", got)
	}
}

func TestFormatToolEventBlock_FileChangeStartedShowsDiff(t *testing.T) {
	patch := "*** Begin Patch\n*** Add File: a.txt\n+hello\n*** End Patch\n"

	got := FormatToolEventBlock(tools.ToolEvent{
		Type: "item.started",
		Result: tools.ToolResult{
			Kind: tools.ToolApplyPatch,
			Path: "a.txt",
			Diff: patch,
		},
	})
	if !strings.Contains(got, "diff:") || !strings.Contains(got, "+hello") {
		t.Fatalf("expected diff in block, got:\n%s", got)
	}
}

func TestTranscript_LoadMessages_FiltersToolFromHistoryButKeepsInView(t *testing.T) {
	tr := NewTranscript(60)
	msgs := []agent.Message{
		{Role: agent.RoleUser, Content: "hi"},
		{Role: agent.Role("tool"), Content: "✓ command_execution completed\n  └ output:\n    ok"},
		{Role: agent.RoleAssistant, Content: "done"},
	}
	tr.LoadMessages(msgs)

	history := tr.Messages()
	if len(history) != 2 {
		t.Fatalf("expected 2 persisted conversation messages, got %d: %#v", len(history), history)
	}
	if history[0].Role != agent.RoleUser {
		t.Fatalf("expected history[0] role=user, got %#v", history[0])
	}
	if history[1].Role != agent.RoleAssistant {
		t.Fatalf("expected history[1] role=assistant, got %#v", history[1])
	}

	view := tr.ViewMessages()
	if len(view) != 3 {
		t.Fatalf("expected 3 view messages, got %d: %#v", len(view), view)
	}

	viewLines := LinesToPlainStrings(tr.RenderViewLines(80))
	viewText := strings.Join(viewLines, "\n")
	if !strings.Contains(viewText, "command_execution") {
		t.Fatalf("expected tool block in view, got:\n%s", viewText)
	}
}
