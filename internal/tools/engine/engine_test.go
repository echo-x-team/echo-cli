package engine

import (
	"context"
	"errors"
	"testing"

	"echo-cli/internal/policy"
	"echo-cli/internal/tools"
)

type stubRunner struct {
	fail bool
}

func (s *stubRunner) RunCommand(ctx context.Context, workdir string, command string) (string, int, error) {
	if s.fail {
		return "", 1, errors.New("fail")
	}
	return "ok", 0, nil
}

func (s *stubRunner) ApplyPatch(ctx context.Context, workdir string, diff string) error {
	if s.fail {
		return errors.New("fail")
	}
	return nil
}

type autoApprover struct {
	allow bool
}

func (a autoApprover) Approve(tools.ToolCall) bool { return a.allow }

func TestOnFailureApprovalFlow(t *testing.T) {
	runner := &stubRunner{fail: true}
	pol := policy.Policy{SandboxMode: "danger-full-access", ApprovalPolicy: "on-failure"}
	eng := New(pol, runner, autoApprover{allow: true}, "")

	var events []tools.ToolEvent
	eng.Run(context.Background(), tools.ToolRequest{ID: "1", Kind: tools.ToolCommand, Command: "echo hi"}, func(ev tools.ToolEvent) {
		events = append(events, ev)
	})
	if len(events) == 0 || events[len(events)-1].Result.Status != "error" {
		t.Fatalf("expected failure event, got %+v", events)
	}

	runner.fail = false
	events = nil
	eng.Run(context.Background(), tools.ToolRequest{ID: "2", Kind: tools.ToolCommand, Command: "echo hi"}, func(ev tools.ToolEvent) {
		events = append(events, ev)
	})
	foundApproval := false
	for _, ev := range events {
		if ev.Type == "approval.requested" {
			foundApproval = true
		}
	}
	if !foundApproval {
		t.Fatalf("expected approval after failure, got %+v", events)
	}
}
