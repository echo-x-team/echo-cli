package context

import (
	"strings"
	"testing"
)

func TestProcessResponseItemsForHistory_KeepsGhostSnapshot(t *testing.T) {
	t.Parallel()

	items := []ResponseItem{
		NewUserMessageItem("hello"),
		{
			Type: ResponseItemTypeGhostSnapshot,
			GhostSnapshot: &GhostSnapshotResponseItem{
				GhostCommit: GhostCommit{ID: "g1"},
			},
		},
	}

	out := processResponseItemsForHistory(items)
	if len(out) != 2 {
		t.Fatalf("expected 2 items, got %d", len(out))
	}
	if out[1].Type != ResponseItemTypeGhostSnapshot || out[1].GhostSnapshot == nil || out[1].GhostSnapshot.GhostCommit.ID != "g1" {
		t.Fatalf("unexpected ghost snapshot: %#v", out[1])
	}
}

func TestProcessResponseItemsForHistory_FormatsTruncatedToolOutput(t *testing.T) {
	t.Setenv("ECHO_TOOL_OUTPUT_TOKEN_LIMIT", "1")
	content := "line1\nline2\nline3\n"

	items := []ResponseItem{
		{
			Type: ResponseItemTypeFunctionCall,
			FunctionCall: &FunctionCallResponseItem{
				Name:      "exec_command",
				Arguments: "{}",
				CallID:    "c1",
			},
		},
		{
			Type: ResponseItemTypeFunctionCallOutput,
			FunctionCallOutput: &FunctionCallOutputResponseItem{
				CallID: "c1",
				Output: FunctionCallOutputPayload{Content: content},
			},
		},
	}

	out := processResponseItemsForHistory(items)
	if len(out) != 2 || out[1].FunctionCallOutput == nil {
		t.Fatalf("unexpected output: %#v", out)
	}
	got := out[1].FunctionCallOutput.Output.Content
	if !strings.HasPrefix(got, "Total output lines: 3\n\n") {
		t.Fatalf("expected formatted prefix, got %q", got)
	}
	if !strings.Contains(got, "tokens truncated") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
}

func TestRemoveFirstItem_RemovesLocalShellPairWhenOutputRemoved(t *testing.T) {
	t.Parallel()

	items := []ResponseItem{
		{
			Type: ResponseItemTypeFunctionCallOutput,
			FunctionCallOutput: &FunctionCallOutputResponseItem{
				CallID: "c1",
				Output: FunctionCallOutputPayload{Content: "ok"},
			},
		},
		{
			Type: ResponseItemTypeLocalShellCall,
			LocalShellCall: &LocalShellCallResponseItem{
				CallID: "c1",
				Status: LocalShellStatusCompleted,
				Action: LocalShellAction{Type: "exec", Command: []string{"echo ok"}},
			},
		},
		NewUserMessageItem("next"),
	}

	out := RemoveFirstItem(items)
	if len(out) != 1 {
		t.Fatalf("expected 1 item after removing pair, got %d", len(out))
	}
	if out[0].Type != ResponseItemTypeMessage || out[0].Message == nil || strings.TrimSpace(out[0].Message.Role) != "user" {
		t.Fatalf("unexpected remaining item: %#v", out[0])
	}
	if FlattenContentItems(out[0].Message.Content) != "next" {
		t.Fatalf("unexpected remaining content: %#v", out[0])
	}
}
