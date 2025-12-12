package repl

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func TestEQRenderer_RendersToolEventsAsCells(t *testing.T) {
	var buf bytes.Buffer
	r := NewEQRenderer(EQRendererOptions{
		SessionID: "sess-1",
		Width:     60,
		Writer:    &buf,
	})

	op := events.Operation{
		Kind: events.OperationUserInput,
		UserInput: &events.UserInputOperation{
			Items: []events.InputMessage{{Role: "user", Content: "hi"}},
		},
	}
	r.Handle(events.Event{
		Type:         events.EventSubmissionAccepted,
		SubmissionID: "sub-1",
		SessionID:    "sess-1",
		Timestamp:    time.Now(),
		Payload:      op,
	})

	r.Handle(events.Event{
		Type:         events.EventToolEvent,
		SubmissionID: "sub-1",
		SessionID:    "sess-1",
		Timestamp:    time.Now(),
		Payload: tools.ToolEvent{
			Type: "item.started",
			Result: tools.ToolResult{
				ID:      "call-1",
				Kind:    tools.ToolCommand,
				Command: "ls -la",
			},
		},
	})

	r.Handle(events.Event{
		Type:         events.EventToolEvent,
		SubmissionID: "sub-1",
		SessionID:    "sess-1",
		Timestamp:    time.Now(),
		Payload: tools.ToolEvent{
			Type: "item.completed",
			Result: tools.ToolResult{
				ID:      "call-1",
				Kind:    tools.ToolCommand,
				Command: "ls -la",
				Output:  "ok\nline2",
				Status:  "completed",
			},
		},
	})

	r.Handle(events.Event{
		Type:         events.EventAgentOutput,
		SubmissionID: "sub-1",
		SessionID:    "sess-1",
		Timestamp:    time.Now(),
		Payload: events.AgentOutput{
			Content: "hello ",
			Final:   false,
		},
	})
	r.Handle(events.Event{
		Type:         events.EventAgentOutput,
		SubmissionID: "sub-1",
		SessionID:    "sess-1",
		Timestamp:    time.Now(),
		Payload: events.AgentOutput{
			Content: "hello world",
			Final:   true,
		},
	})

	out := stripANSI(buf.String())

	if !strings.Contains(out, "› hi") {
		t.Fatalf("expected user cell, got:\n%s", out)
	}
	if !strings.Contains(out, "> running ls -la") {
		t.Fatalf("expected tool started cell, got:\n%s", out)
	}
	if !strings.Contains(out, "command_execution completed") {
		// The exact prefix is implementation-defined, but kind should be visible.
		t.Fatalf("expected tool completed cell with kind, got:\n%s", out)
	}
	if !strings.Contains(out, "output:") || !strings.Contains(out, "ok") {
		t.Fatalf("expected tool output details, got:\n%s", out)
	}
	if !strings.Contains(out, "• hello world") {
		t.Fatalf("expected assistant final cell, got:\n%s", out)
	}
}
