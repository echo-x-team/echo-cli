package repl

import (
	"context"
	"testing"
	"time"

	"echo-cli/internal/events"
)

func TestGatewaySubmitAndReceiveEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	manager := events.NewManager(events.ManagerConfig{SubmissionBuffer: 4, EventBuffer: 4, Workers: 1})
	manager.RegisterHandler(events.OperationUserInput, events.HandlerFunc(func(ctx context.Context, submission events.Submission, emit events.EventPublisher) error {
		_ = emit.Publish(ctx, events.Event{
			Type:         events.EventAgentOutput,
			SubmissionID: submission.ID,
			SessionID:    submission.SessionID,
			Payload:      events.AgentOutput{Content: "ok", Final: true},
		})
		return nil
	}))
	manager.Start(ctx)
	defer manager.Close()

	gateway := NewGateway(manager)
	eventsCh := gateway.Events()
	subID, err := gateway.SubmitUserInput(ctx, []events.InputMessage{{Role: "user", Content: "ping"}}, events.InputContext{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	seenOutput := false
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting events")
		case ev := <-eventsCh:
			if ev.SubmissionID != subID {
				continue
			}
			switch ev.Type {
			case events.EventAgentOutput:
				seenOutput = true
			case events.EventTaskCompleted:
				if !seenOutput {
					t.Fatalf("task completed before output observed")
				}
				return
			}
		}
	}
}
