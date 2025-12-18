package execution

import (
	"io"
	"strings"
	"testing"

	"echo-cli/internal/events"
	"echo-cli/internal/tools"
	"github.com/sirupsen/logrus"
)

func TestToolResultErrorLogIncludesPayloadAndPatchContext(t *testing.T) {
	oldErrorLog := errorLog
	defer func() { errorLog = oldErrorLog }()

	l := logrus.New()
	l.SetOutput(io.Discard)
	hook := &captureHook{}
	l.AddHook(hook)
	errorLog = logrus.NewEntry(l)

	engine := &Engine{}
	sub := events.Submission{
		SessionID: "sess-1",
		ID:        "sub-1",
		Operation: events.Operation{Kind: events.OperationUserInput},
	}
	turnCtx := TurnContext{Model: "gpt-test"}

	call := &tools.ToolCall{
		ID:      "call-1",
		Name:    "apply_patch",
		Payload: []byte(`{"patch":"*** Begin Patch\n*** Add File: x.txt\n+hi"}`),
	}
	result := tools.ToolResult{
		ID:     "call-1",
		Kind:   tools.ToolApplyPatch,
		Status: "error",
		Error:  "invalid patch: missing *** End Patch",
		Diff:   "*** Begin Patch\n*** Add File: x.txt\n+hi",
	}

	engine.logToolResultError(sub, turnCtx, 10, call, result)

	var found capturedEntry
	ok := false
	for _, e := range hook.snapshot() {
		if e.Message == "runTask tool_result error" {
			found = e
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("expected tool_result error log entry, got %d entries", len(hook.snapshot()))
	}
	preview, _ := found.Data["tool_payload_preview"].(string)
	if !strings.Contains(preview, "*** Begin Patch") {
		t.Fatalf("expected tool_payload_preview to include patch header, got %q", preview)
	}
	hasEnd, ok := found.Data["tool_patch_has_end_patch"].(bool)
	if !ok {
		t.Fatalf("expected tool_patch_has_end_patch bool, got %T", found.Data["tool_patch_has_end_patch"])
	}
	if hasEnd {
		t.Fatalf("expected tool_patch_has_end_patch=false")
	}
	if head, _ := found.Data["tool_patch_head_preview"].(string); !strings.Contains(head, "*** Begin Patch") {
		t.Fatalf("expected tool_patch_head_preview, got %q", head)
	}
	if tail, _ := found.Data["tool_patch_tail_preview"].(string); tail == "" {
		t.Fatalf("expected tool_patch_tail_preview")
	}
}
