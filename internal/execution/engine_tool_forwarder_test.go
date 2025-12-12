package execution

import (
	"context"
	"testing"
	"time"

	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

func TestToolForwarderAttachesSubmissionContextAndClearsOnComplete(t *testing.T) {
	bus := events.NewBus()
	manager := events.NewManager(events.ManagerConfig{Workers: 1})
	eng := NewEngine(Options{Manager: manager, Bus: bus})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eng.startToolForwarder(ctx)

	sub := events.Submission{
		ID:        "sub-1",
		SessionID: "sess-1",
		Metadata:  map[string]string{"foo": "bar"},
	}
	eng.registerToolCallContext(sub, "call-1")

	ch := manager.Subscribe()

	bus.Publish(tools.ToolEvent{
		Type: "item.started",
		Result: tools.ToolResult{
			ID:   "call-1",
			Kind: tools.ToolCommand,
		},
	})

	var ev events.Event
	select {
	case ev = <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for forwarded tool.event")
	}

	if ev.Type != events.EventToolEvent {
		t.Fatalf("expected tool.event, got %s", ev.Type)
	}
	if ev.SubmissionID != "sub-1" || ev.SessionID != "sess-1" {
		t.Fatalf("expected submission/session attached, got sub=%q sess=%q", ev.SubmissionID, ev.SessionID)
	}
	if ev.Metadata["tool_kind"] != string(tools.ToolCommand) || ev.Metadata["foo"] != "bar" {
		t.Fatalf("unexpected metadata: %#v", ev.Metadata)
	}

	// Completing should clear the mapping.
	bus.Publish(tools.ToolEvent{
		Type: "item.completed",
		Result: tools.ToolResult{
			ID:   "call-1",
			Kind: tools.ToolCommand,
		},
	})

	// Drain the completed event.
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for completed tool.event")
	}

	subID, sessID, _ := eng.lookupToolCallContext("call-1")
	if subID != "" || sessID != "" {
		t.Fatalf("expected context cleared after completion, got sub=%q sess=%q", subID, sessID)
	}
}
