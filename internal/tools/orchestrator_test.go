package tools

import (
	"context"
	"testing"
	"time"
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

type stubExecCommandHandler struct {
	called bool
}

func (h *stubExecCommandHandler) Name() string               { return "exec_command" }
func (h *stubExecCommandHandler) Kind() ToolKind             { return ToolCommand }
func (h *stubExecCommandHandler) SupportsParallel() bool     { return false }
func (h *stubExecCommandHandler) IsMutating(Invocation) bool { return true }
func (h *stubExecCommandHandler) Describe(Invocation) ToolResult {
	return ToolResult{Command: "rm -rf /tmp/whatever"}
}
func (h *stubExecCommandHandler) Handle(context.Context, Invocation) (ToolResult, error) {
	h.called = true
	return ToolResult{Status: "completed"}, nil
}

type stubReviewer struct {
	review CommandReview
	err    error
}

func (r stubReviewer) Review(context.Context, string, string) (CommandReview, error) {
	return r.review, r.err
}

func TestOrchestrator_RequiresApprovalBeforeExecCommand(t *testing.T) {
	approvals := NewApprovalStore()
	o := NewOrchestratorWith(OrchestratorOptions{
		Reviewer:  stubReviewer{review: CommandReview{RiskLevel: "high", Description: "destructive"}},
		Approvals: approvals,
	})
	h := &stubExecCommandHandler{}
	inv := Invocation{Call: ToolCall{ID: "tool-1", Name: "exec_command", Payload: []byte(`{"command":"rm -rf /tmp/whatever"}`)}}

	seenRequire := false
	go func() {
		time.Sleep(50 * time.Millisecond)
		approvals.Resolve(ApprovalDecision{ApprovalID: "tool-1", Approved: true})
	}()

	res := o.Run(context.Background(), inv, h, func(ev ToolEvent) {
		if ev.Type == "item.updated" && ev.Result.Status == "requires_approval" {
			seenRequire = true
		}
	})
	if !seenRequire {
		t.Fatalf("expected requires_approval update event")
	}
	if !h.called {
		t.Fatalf("expected handler to run after approval")
	}
	if res.Status != "completed" {
		t.Fatalf("expected completed result, got %+v", res)
	}
}

func TestOrchestrator_DeniedApprovalStopsExecCommand(t *testing.T) {
	approvals := NewApprovalStore()
	o := NewOrchestratorWith(OrchestratorOptions{
		Reviewer:  stubReviewer{review: CommandReview{RiskLevel: "high", Description: "destructive"}},
		Approvals: approvals,
	})
	h := &stubExecCommandHandler{}
	inv := Invocation{Call: ToolCall{ID: "tool-2", Name: "exec_command", Payload: []byte(`{"command":"rm -rf /tmp/whatever"}`)}}

	go func() {
		time.Sleep(50 * time.Millisecond)
		approvals.Resolve(ApprovalDecision{ApprovalID: "tool-2", Approved: false})
	}()

	res := o.Run(context.Background(), inv, h, func(ToolEvent) {})
	if h.called {
		t.Fatalf("handler should not run when approval denied")
	}
	if res.Status != "error" || res.Error == "" {
		t.Fatalf("expected error result when denied, got %+v", res)
	}
}
