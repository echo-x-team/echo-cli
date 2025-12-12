package tui

import (
	"strings"
	"testing"

	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

func TestPlanSectionRendersLatestPlanOnly(t *testing.T) {
	m := New(Options{})
	m.resize(80, 24)

	heightBefore := m.conversationHeight(m.conversationWidth())
	msgsBefore := len(m.messages)

	first := events.Event{
		Type:      events.EventPlanUpdated,
		SessionID: "",
		Payload: tools.UpdatePlanArgs{
			Explanation: "because",
			Plan: []tools.PlanItem{
				{Step: "step one", Status: "pending"},
			},
		},
	}
	m.handleEngineEvent(first)

	if len(m.messages) != msgsBefore {
		t.Fatalf("plan.updated should not append to transcript: got %d msgs, want %d", len(m.messages), msgsBefore)
	}
	view1 := m.View()
	if !strings.Contains(view1, "Updated Plan") || !strings.Contains(view1, "step one") {
		t.Fatalf("expected plan section to render first update, got view:\n%s", view1)
	}

	heightAfterFirst := m.conversationHeight(m.conversationWidth())
	if heightAfterFirst >= heightBefore {
		t.Fatalf("expected conversation height to shrink after plan appears: before=%d after=%d", heightBefore, heightAfterFirst)
	}

	second := events.Event{
		Type:      events.EventPlanUpdated,
		SessionID: "",
		Payload: tools.UpdatePlanArgs{
			Explanation: "updated",
			Plan: []tools.PlanItem{
				{Step: "step two", Status: "in_progress"},
			},
		},
	}
	m.handleEngineEvent(second)

	view2 := m.View()
	if strings.Contains(view2, "step one") {
		t.Fatalf("expected old plan to be replaced by new plan, got view:\n%s", view2)
	}
	if !strings.Contains(view2, "step two") {
		t.Fatalf("expected new plan to render, got view:\n%s", view2)
	}
}
