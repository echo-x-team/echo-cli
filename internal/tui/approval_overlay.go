package tui

import (
	"context"
	"fmt"
	"strings"

	"echo-cli/internal/tools"
	tuirender "echo-cli/internal/tui/render"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type approvalRequest struct {
	ID        string
	Command   string
	Reason    string
	SessionID string
}

func (m *Model) enqueueApprovalRequest(result tools.ToolResult, sessionID string) {
	approvalID := strings.TrimSpace(result.ApprovalID)
	if approvalID == "" {
		return
	}
	if m.hasApprovalID(approvalID) {
		return
	}
	req := approvalRequest{
		ID:        approvalID,
		Command:   strings.TrimSpace(result.Command),
		Reason:    strings.TrimSpace(result.ApprovalReason),
		SessionID: strings.TrimSpace(sessionID),
	}
	if m.approvalActive == nil {
		m.approvalActive = &req
		return
	}
	m.approvalQueue = append(m.approvalQueue, req)
}

func (m *Model) hasApprovalID(approvalID string) bool {
	if approvalID == "" {
		return false
	}
	if m.approvalActive != nil && m.approvalActive.ID == approvalID {
		return true
	}
	for _, req := range m.approvalQueue {
		if req.ID == approvalID {
			return true
		}
	}
	return false
}

func (m *Model) advanceApprovalQueue() {
	if len(m.approvalQueue) == 0 {
		m.approvalActive = nil
		return
	}
	next := m.approvalQueue[0]
	m.approvalQueue = m.approvalQueue[1:]
	m.approvalActive = &next
}

func (m *Model) approvalView(width int) string {
	if m.approvalActive == nil {
		return ""
	}
	if width <= 0 {
		width = 80
	}
	contentWidth := maxInt(20, width)

	titleStyle := lipgloss.NewStyle().Bold(true)
	hintStyle := lipgloss.NewStyle().Bold(true)

	lines := []string{titleStyle.Render("Approval required")}
	command := strings.TrimSpace(m.approvalActive.Command)
	if command != "" {
		lines = append(lines, "", "Command:")
		lines = append(lines, indentApprovalLines(tuirender.WrapText(command, contentWidth-2))...)
	}
	reason := strings.TrimSpace(m.approvalActive.Reason)
	if reason != "" {
		lines = append(lines, "", "Reason:")
		lines = append(lines, indentApprovalLines(tuirender.WrapText(reason, contentWidth-2))...)
	}
	lines = append(lines, "", hintStyle.Render("[y] approve • [n] deny"))
	return lipgloss.NewStyle().Width(contentWidth).Render(strings.Join(lines, "\n"))
}

func indentApprovalLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, "  "+line)
	}
	return out
}

func (m *Model) handleApprovalKey(msg tea.KeyMsg) tea.Cmd {
	if m.approvalActive == nil {
		return nil
	}
	key := strings.ToLower(msg.String())
	switch key {
	case "y":
		cmd, ok := m.submitApprovalDecision(*m.approvalActive, true)
		if ok {
			m.advanceApprovalQueue()
		}
		return cmd
	case "n":
		cmd, ok := m.submitApprovalDecision(*m.approvalActive, false)
		if ok {
			m.advanceApprovalQueue()
		}
		return cmd
	case "ctrl+c", "q":
		return tea.Quit
	}
	return nil
}

func (m *Model) submitApprovalDecision(req approvalRequest, approved bool) (tea.Cmd, bool) {
	if m.gateway == nil {
		m.appendAssistantMessage("gateway not configured; cannot submit approval decision.")
		return nil, false
	}
	approvalID := strings.TrimSpace(req.ID)
	if approvalID == "" {
		m.appendAssistantMessage("missing approval_id")
		return nil, false
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(m.eqCtx.SessionID)
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(m.resumeSessionID)
	}
	if sessionID == "" {
		m.appendAssistantMessage("session id not set; cannot submit approval decision.")
		return nil, false
	}
	return func() tea.Msg {
		if _, err := m.gateway.SubmitApprovalDecision(context.Background(), sessionID, approvalID, approved); err != nil {
			return systemMsg{Text: fmt.Sprintf("submit approval decision failed: %v", err)}
		}
		return nil
	}, true
}
