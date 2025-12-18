package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"echo-cli/internal/events"
	"echo-cli/internal/instructions"
)

type stubGateway struct {
	lastInput   []events.InputMessage
	inputCtx    events.InputContext
	submissions int
}

func (g *stubGateway) SubmitUserInput(ctx context.Context, items []events.InputMessage, inputCtx events.InputContext) (string, error) {
	g.lastInput = append([]events.InputMessage{}, items...)
	g.inputCtx = inputCtx
	g.submissions++
	return "sub-id", nil
}

func (g *stubGateway) SubmitApprovalDecision(ctx context.Context, sessionID string, approvalID string, approved bool) (string, error) {
	g.submissions++
	return "sub-id", nil
}

func (g *stubGateway) Events() <-chan events.Event {
	return nil
}

func TestHandleInitCommandSkipsWhenDocExists(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, instructions.ProjectDocFilename)
	if err := os.WriteFile(target, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write AGENTS: %v", err)
	}

	m := New(Options{Workdir: tmp})
	cmd := m.handleInitCommand()
	if cmd != nil {
		t.Fatalf("expected no command when AGENTS exists")
	}
	if m.pending {
		t.Fatalf("pending should remain false")
	}
	if len(m.messages) == 0 {
		t.Fatalf("expected info message about skipping /init")
	}
	last := m.messages[len(m.messages)-1].Content
	if !strings.Contains(last, "Skipping /init") {
		t.Fatalf("unexpected skip message: %q", last)
	}
}

func TestHandleInitCommandStartsStream(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	for _, dir := range []string{"cmd", "internal", "pkg", "vendor"} {
		if err := os.Mkdir(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	gateway := &stubGateway{}
	m := New(Options{Workdir: tmp})
	m.gateway = gateway

	m.handleInitCommand()
	if !m.pending {
		t.Fatalf("model should be pending after submitting init prompt")
	}
	if gateway.submissions != 1 {
		t.Fatalf("expected one submission, got %d", gateway.submissions)
	}
	if len(gateway.lastInput) != 1 {
		t.Fatalf("expected single message submission, got %d", len(gateway.lastInput))
	}

	content := gateway.lastInput[0].Content
	if !strings.Contains(content, "仓库扫描:") {
		t.Fatalf("missing repository scan section: %q", content)
	}
	if !strings.Contains(content, "Go 模块: example.com/demo") {
		t.Fatalf("missing module line: %q", content)
	}
	if !strings.Contains(content, "顶层目录: cmd, internal, pkg") {
		t.Fatalf("unexpected directories line: %q", content)
	}
	if !strings.Contains(content, "重要文件: README.md, go.mod") {
		t.Fatalf("unexpected files line: %q", content)
	}
	if gateway.inputCtx.Metadata["target"] != "@internal/execution" {
		t.Fatalf("unexpected target metadata: %+v", gateway.inputCtx.Metadata)
	}
}

func TestHandleInitCommandBlocksWhenPending(t *testing.T) {
	tmp := t.TempDir()
	m := New(Options{Workdir: tmp})
	m.pending = true
	gateway := &stubGateway{}
	m.gateway = gateway

	cmd := m.handleInitCommand()
	if cmd != nil {
		t.Fatalf("expected no command when pending")
	}
	if gateway.submissions != 0 {
		t.Fatalf("no submission expected while pending, got %d", gateway.submissions)
	}
	if len(m.messages) == 0 {
		t.Fatalf("expected warning message when pending")
	}
	if !strings.Contains(m.messages[len(m.messages)-1].Content, "Cannot run /init") {
		t.Fatalf("unexpected pending message: %q", m.messages[len(m.messages)-1].Content)
	}
}
