package engine

import (
	"context"
	"errors"
	"testing"

	"echo-cli/internal/policy"
	"echo-cli/internal/tools"
)

type stubRunner struct {
	fail         bool
	sandboxBlock bool
	mode         string
}

func (s *stubRunner) RunCommand(ctx context.Context, workdir string, command string) (string, int, error) {
	if s.mode == "" {
		s.mode = "workspace-write"
	}
	if s.sandboxBlock {
		s.sandboxBlock = false
		return "", -1, tools.SandboxError{Reason: "sandbox blocked"}
	}
	if s.fail {
		return "", 1, errors.New("fail")
	}
	return "ok", 0, nil
}

func (s *stubRunner) ApplyPatch(ctx context.Context, workdir string, diff string) error {
	if s.mode == "" {
		s.mode = "workspace-write"
	}
	if s.sandboxBlock {
		s.sandboxBlock = false
		return tools.SandboxError{Reason: "sandbox blocked"}
	}
	if s.fail {
		return errors.New("fail")
	}
	return nil
}

func (s *stubRunner) WithMode(mode string) tools.Runner {
	s.mode = mode
	return s
}

type autoApprover struct {
	allow bool
}

func (a autoApprover) Approve(tools.ToolCall) bool { return a.allow }

func TestSandboxDenialTriggersApprovalAndRetry(t *testing.T) {
	runner := &stubRunner{sandboxBlock: true}
	pol := policy.Policy{SandboxMode: "read-only", ApprovalPolicy: "on-request"}
	eng := New(pol, runner, autoApprover{allow: true}, "")

	var events []tools.ToolEvent
	eng.Run(context.Background(), tools.ToolRequest{ID: "1", Kind: tools.ToolCommand, Command: "echo hi"}, func(ev tools.ToolEvent) {
		events = append(events, ev)
	})
	foundApproval := false
	finalStatus := ""
	for _, ev := range events {
		if ev.Type == "approval.requested" {
			foundApproval = true
		}
		if ev.Type == "item.completed" {
			finalStatus = ev.Result.Status
		}
	}
	if !foundApproval {
		t.Fatalf("expected approval on sandbox denial, got %+v", events)
	}
	if finalStatus != "completed" {
		t.Fatalf("expected command to succeed after approval, got status=%s", finalStatus)
	}
	if runner.mode != "danger-full-access" {
		t.Fatalf("expected runner escalated to danger-full-access, got %s", runner.mode)
	}
}
