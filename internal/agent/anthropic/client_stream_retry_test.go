package anthropic

import (
	"context"
	"encoding/json"
	"testing"

	"echo-cli/internal/agent"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

type fakeMessageStream struct {
	events []anthropic.MessageStreamEventUnion
	idx    int
	err    error
}

func (s *fakeMessageStream) Next() bool {
	if s.idx >= len(s.events) {
		return false
	}
	s.idx++
	return true
}

func (s *fakeMessageStream) Current() anthropic.MessageStreamEventUnion {
	return s.events[s.idx-1]
}

func (s *fakeMessageStream) Err() error {
	return s.err
}

func mustEvent(t *testing.T, raw string) anthropic.MessageStreamEventUnion {
	t.Helper()
	var event anthropic.MessageStreamEventUnion
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	return event
}

func TestClientStreamReturnsNoEventsOnEmptyResponse(t *testing.T) {
	emptyEvents := []anthropic.MessageStreamEventUnion{
		mustEvent(t, `{"type":"message_start","message":{"id":"msg_empty","type":"message","role":"assistant","model":"glm-4.6","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`),
		mustEvent(t, `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":0,"output_tokens":0}}`),
		mustEvent(t, `{"type":"message_stop"}`),
	}

	attempts := 0
	client := &Client{
		model: "gpt-test",
		newStream: func(ctx context.Context, _ anthropic.MessageNewParams) messageStream {
			attempts++
			return &fakeMessageStream{events: emptyEvents}
		},
	}

	var got []agent.StreamEvent
	err := client.Stream(context.Background(), agent.Prompt{Model: "gpt-test"}, func(evt agent.StreamEvent) {
		got = append(got, evt)
	})
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if len(got) != 0 {
		t.Fatalf("events = %d, want 0", len(got))
	}
}

func TestClientStreamEmitsTextEvents(t *testing.T) {
	textEvents := []anthropic.MessageStreamEventUnion{
		mustEvent(t, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`),
	}
	attempts := 0
	client := &Client{
		model: "gpt-test",
		newStream: func(ctx context.Context, _ anthropic.MessageNewParams) messageStream {
			attempts++
			return &fakeMessageStream{events: textEvents}
		},
	}

	var got []agent.StreamEvent
	err := client.Stream(context.Background(), agent.Prompt{Model: "gpt-test"}, func(evt agent.StreamEvent) {
		got = append(got, evt)
	})
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if len(got) != 2 {
		t.Fatalf("events = %d, want 2", len(got))
	}
	if got[0].Type != agent.StreamEventTextDelta || got[0].Text != "hello" {
		t.Fatalf("unexpected first event: %#v", got[0])
	}
	if got[1].Type != agent.StreamEventCompleted {
		t.Fatalf("unexpected second event: %#v", got[1])
	}
}
