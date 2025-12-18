package tools

import (
	"context"
	"testing"

	"echo-cli/internal/policy"
)

type denyApprover struct{}

func (denyApprover) Approve(ToolCall) bool { return false }

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

func TestOrchestratorReturnsDeniedResult(t *testing.T) {
	h := &stubApplyPatchHandler{}
	o := NewOrchestrator(policy.Policy{}, denyApprover{})
	inv := Invocation{
		Call:    ToolCall{ID: "1", Name: "apply_patch", Payload: []byte(`{"patch":"x"}`)},
		Workdir: ".",
		Policy:  policy.Policy{SandboxMode: "workspace-write", ApprovalPolicy: "untrusted"},
	}

	var completed ToolResult
	res := o.Run(context.Background(), inv, h, func(ev ToolEvent) {
		if ev.Type == "item.completed" {
			completed = ev.Result
		}
	})

	if h.called {
		t.Fatalf("handler should not be called when approval denied")
	}
	if completed.Status != "error" {
		t.Fatalf("expected completed status error, got %q", completed.Status)
	}
	if completed.Error == "" {
		t.Fatalf("expected completed error to be set")
	}
	if completed.Error != "denied: requires approval" {
		t.Fatalf("unexpected completed error: %q", completed.Error)
	}
	if res.Status != completed.Status || res.Error != completed.Error {
		t.Fatalf("expected returned result to match emitted completion, got returned=%+v completed=%+v", res, completed)
	}
}
