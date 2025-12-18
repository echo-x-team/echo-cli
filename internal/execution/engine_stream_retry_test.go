package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"echo-cli/internal/agent"
)

type timedFlakyStreamClient struct {
	errs      []error
	callTimes []time.Time
}

func (c *timedFlakyStreamClient) Complete(_ context.Context, _ agent.Prompt) (string, error) {
	return "", nil
}

func (c *timedFlakyStreamClient) Stream(ctx context.Context, _ agent.Prompt, onEvent func(agent.StreamEvent)) error {
	c.callTimes = append(c.callTimes, time.Now())
	callIndex := len(c.callTimes) - 1
	if callIndex < len(c.errs) && c.errs[callIndex] != nil {
		return c.errs[callIndex]
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: "ok"})
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	return nil
}

func TestStreamPromptRetriesOnceOnInternalNetworkFailure(t *testing.T) {
	client := &timedFlakyStreamClient{
		errs: []error{
			errors.New(`received error while streaming: {"type":"error","error":{"type":"api_error","message":"Internal Network Failure"}}`),
		},
	}

	engine := &Engine{
		client:         client,
		requestTimeout: time.Second,
		retries:        0,
		retryDelay:     20 * time.Millisecond,
	}

	prompt := Prompt{Model: "gpt-test"}
	if err := engine.streamPrompt(context.Background(), prompt, func(agent.StreamEvent) {}); err != nil {
		t.Fatalf("streamPrompt failed: %v", err)
	}
	if got := len(client.callTimes); got != 2 {
		t.Fatalf("expected 2 stream attempts, got %d", got)
	}
	if gap := client.callTimes[1].Sub(client.callTimes[0]); gap < engine.retryDelay {
		t.Fatalf("expected retry delay >= %s, got %s", engine.retryDelay, gap)
	}
}

func TestStreamPromptDoesNotRetryWhenRetriesZeroAndErrorNotInternalNetworkFailure(t *testing.T) {
	client := &timedFlakyStreamClient{
		errs: []error{errors.New("boom")},
	}

	engine := &Engine{
		client:         client,
		requestTimeout: time.Second,
		retries:        0,
		retryDelay:     20 * time.Millisecond,
	}

	prompt := Prompt{Model: "gpt-test"}
	if err := engine.streamPrompt(context.Background(), prompt, func(agent.StreamEvent) {}); err == nil {
		t.Fatalf("expected error")
	}
	if got := len(client.callTimes); got != 1 {
		t.Fatalf("expected 1 stream attempt, got %d", got)
	}
}
