package tools

import (
	"context"
	"testing"
)

type stubApplyPatchHandler struct {
	called bool
}

func (h *stubApplyPatchHandler) Name() string               { return "apply_patch" }
func (h *stubApplyPatchHandler) Kind() ToolKind             { return ToolApplyPatch }
func (h *stubApplyPatchHandler) SupportsParallel() bool     { return true }
func (h *stubApplyPatchHandler) IsMutating(Invocation) bool { return true }
func (h *stubApplyPatchHandler) Describe(Invocation) ToolResult {
	return ToolResult{}
}
func (h *stubApplyPatchHandler) Handle(context.Context, Invocation) (ToolResult, error) {
	h.called = true
	return ToolResult{Status: "completed"}, nil
}

func TestOrchestratorEmitsStartedAndCompleted(t *testing.T) {
	h := &stubApplyPatchHandler{}
	o := NewOrchestrator()
	inv := Invocation{
		Call:    ToolCall{ID: "1", Name: "apply_patch", Payload: []byte(`{"patch":"x"}`)},
		Workdir: ".",
	}

	seenStarted := false
	var completed ToolResult
	res := o.Run(context.Background(), inv, h, func(ev ToolEvent) {
		if ev.Type == "item.started" {
			seenStarted = true
		}
		if ev.Type == "item.completed" {
			completed = ev.Result
		}
	})

	if !h.called {
		t.Fatalf("handler should be called")
	}
	if !seenStarted {
		t.Fatalf("expected item.started to be emitted")
	}
	if completed.Status != "completed" {
		t.Fatalf("expected completed status completed, got %q", completed.Status)
	}
	if res.Status != completed.Status {
		t.Fatalf("expected returned result to match emitted completion, got returned=%+v completed=%+v", res, completed)
	}
}
