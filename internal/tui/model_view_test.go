package tui

import (
	"fmt"
	"strings"
	"testing"

	"echo-cli/internal/agent"
)

func TestFlushTranscriptUsesViewportHeight(t *testing.T) {
	m := New(Options{})
	m.resize(80, 30)

	longText := strings.Repeat("hello ", 20)
	for i := 0; i < 30; i++ {
		m.messages = append(m.messages, agent.Message{Role: agent.RoleUser, Content: fmt.Sprintf("%s%d", longText, i)})
	}
	m.refreshTranscript()

	lines, _ := m.renderTranscriptLines()
	expectedHeight := m.conversationHeight(m.conversationWidth())
	if len(lines) <= expectedHeight {
		t.Fatalf("rendered lines (%d) should exceed viewport height (%d) for test setup", len(lines), expectedHeight)
	}

	m.flushTranscript()

	if m.viewport.Height != expectedHeight {
		t.Fatalf("viewport height = %d, expected %d", m.viewport.Height, expectedHeight)
	}
}
