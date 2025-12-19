package tui

import (
	"context"
	"strings"
	"testing"

	"echo-cli/internal/events"
	"echo-cli/internal/tools"

	tea "github.com/charmbracelet/bubbletea"
)

type approvalDecision struct {
	sessionID  string
	approvalID string
	approved   bool
}

type approvalGateway struct {
	decisions []approvalDecision
}

func (g *approvalGateway) SubmitUserInput(ctx context.Context, items []events.InputMessage, inputCtx events.InputContext) (string, error) {
	return "sub-id", nil
}

func (g *approvalGateway) SubmitApprovalDecision(ctx context.Context, sessionID string, approvalID string, approved bool) (string, error) {
	g.decisions = append(g.decisions, approvalDecision{sessionID: sessionID, approvalID: approvalID, approved: approved})
	return "sub-id", nil
}

func (g *approvalGateway) Events() <-chan events.Event {
	return nil
}

func TestApprovalOverlay_SubmitsDecision(t *testing.T) {
	gateway := &approvalGateway{}
	m := New(Options{})
	m.gateway = gateway

	ev := events.Event{
		Type:      events.EventToolEvent,
		SessionID: "sess-1",
		Payload: tools.ToolEvent{
			Type: "item.updated",
			Result: tools.ToolResult{
				Status:         "requires_approval",
				ApprovalID:     "tool-1",
				Command:        "npm install",
				ApprovalReason: "risk_level=high: needs review",
			},
		},
	}
	m.handleEngineEvent(ev)

	view := m.View()
	if !strings.Contains(view, "Approval required") {
		t.Fatalf("expected approval overlay, got: %s", view)
	}
	if !strings.Contains(view, "npm install") {
		t.Fatalf("expected command in overlay, got: %s", view)
	}

	cmd := m.handleApprovalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatalf("expected approval command")
	}
	cmd()

	if len(gateway.decisions) != 1 {
		t.Fatalf("expected one decision, got %d", len(gateway.decisions))
	}
	decision := gateway.decisions[0]
	if !decision.approved {
		t.Fatalf("expected approved decision, got denied")
	}
	if decision.approvalID != "tool-1" {
		t.Fatalf("expected approval id tool-1, got %q", decision.approvalID)
	}
	if decision.sessionID != "sess-1" {
		t.Fatalf("expected session id sess-1, got %q", decision.sessionID)
	}
	if m.approvalActive != nil {
		t.Fatalf("expected approval overlay to be cleared")
	}
}
