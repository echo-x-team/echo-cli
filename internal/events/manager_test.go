package events

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSubmissionQueueSubmitReceive(t *testing.T) {
	q := NewSubmissionQueue(2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sub1 := Submission{ID: "s1", Operation: Operation{Kind: OperationUserInput}}
	sub2 := Submission{ID: "s2", Operation: Operation{Kind: OperationUserInput}}

	if err := q.Submit(ctx, sub1); err != nil {
		t.Fatalf("submit sub1: %v", err)
	}
	if err := q.Submit(ctx, sub2); err != nil {
		t.Fatalf("submit sub2: %v", err)
	}

	got1, err := q.Receive(ctx)
	if err != nil {
		t.Fatalf("receive sub1: %v", err)
	}
	if got1.ID != "s1" {
		t.Fatalf("expected s1, got %s", got1.ID)
	}
	got2, err := q.Receive(ctx)
	if err != nil {
		t.Fatalf("receive sub2: %v", err)
	}
	if got2.ID != "s2" {
		t.Fatalf("expected s2, got %s", got2.ID)
	}

	q.Close()
	if _, err := q.Receive(ctx); !errors.Is(err, ErrSubmissionQueueClosed) {
		t.Fatalf("expected ErrSubmissionQueueClosed, got %v", err)
	}
}

func TestEventQueueFanout(t *testing.T) {
	q := NewEventQueue(4)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	sub1 := q.Subscribe()
	sub2 := q.Subscribe()

	ev := Event{Type: EventAgentOutput, SubmissionID: "s", Timestamp: time.Now()}
	if err := q.Publish(ctx, ev); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case got := <-sub1:
		if got.Type != ev.Type || got.SubmissionID != ev.SubmissionID {
			t.Fatalf("subscriber1 got %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting subscriber1")
	}
	select {
	case got := <-sub2:
		if got.Type != ev.Type || got.SubmissionID != ev.SubmissionID {
			t.Fatalf("subscriber2 got %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting subscriber2")
	}
}

func TestManagerUserInputFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mgr := NewManager(ManagerConfig{SubmissionBuffer: 4, EventBuffer: 8, Workers: 1})
	defer mgr.Close()

	mgr.RegisterHandler(OperationUserInput, HandlerFunc(func(ctx context.Context, submission Submission, emit EventPublisher) error {
		if submission.Operation.UserInput == nil {
			return errors.New("missing user input payload")
		}
		out := submission.Operation.UserInput.Items
		for i, item := range out {
			_ = emit.Publish(ctx, Event{
				Type:         EventAgentOutput,
				SubmissionID: submission.ID,
				SessionID:    submission.SessionID,
				Timestamp:    time.Now(),
				Payload: AgentOutput{
					Content:  "echo: " + item.Content,
					Sequence: i,
					Final:    i == len(out)-1,
				},
			})
		}
		return nil
	}))

	mgr.Start(ctx)

	events := mgr.Subscribe()
	id, err := mgr.SubmitUserInput(ctx, []InputMessage{{Role: "user", Content: "hello"}}, InputContext{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("submit user input: %v", err)
	}

	var outputs []AgentOutput
	seenStart := false
	seenComplete := false

	for !seenComplete {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for events, got outputs=%v", outputs)
		case ev := <-events:
			if ev.SubmissionID != id {
				continue
			}
			switch ev.Type {
			case EventTaskStarted:
				seenStart = true
			case EventAgentOutput:
				out, ok := ev.Payload.(AgentOutput)
				if !ok {
					t.Fatalf("unexpected payload type %T", ev.Payload)
				}
				outputs = append(outputs, out)
			case EventTaskCompleted:
				seenComplete = true
			}
		}
	}

	if !seenStart {
		t.Fatalf("expected start event")
	}
	if len(outputs) != 1 || outputs[0].Content != "echo: hello" || !outputs[0].Final {
		t.Fatalf("unexpected outputs %+v", outputs)
	}
}

func TestSubmitUserInputMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mgr := NewManager(ManagerConfig{SubmissionBuffer: 4, EventBuffer: 8, Workers: 1})
	defer mgr.Close()

	metaCh := make(chan map[string]string, 1)
	mgr.RegisterHandler(OperationUserInput, HandlerFunc(func(ctx context.Context, submission Submission, emit EventPublisher) error {
		copyMeta := map[string]string{}
		for k, v := range submission.Metadata {
			copyMeta[k] = v
		}
		metaCh <- copyMeta
		return nil
	}))

	mgr.Start(ctx)

	meta := map[string]string{"target": "@internal/execution"}
	events := mgr.Subscribe()
	id, err := mgr.SubmitUserInput(ctx, []InputMessage{{Role: "user", Content: "with-meta"}}, InputContext{
		SessionID: "sess-meta",
		Metadata:  meta,
	})
	if err != nil {
		t.Fatalf("submit user input with metadata: %v", err)
	}

	seenStart := false
	seenComplete := false
	var seenMeta map[string]string

	for !seenComplete {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for metadata events")
		case ev := <-events:
			if ev.SubmissionID != id {
				continue
			}
			if ev.Metadata != nil {
				seenMeta = ev.Metadata
			}
			switch ev.Type {
			case EventTaskStarted:
				seenStart = true
			case EventTaskCompleted:
				seenComplete = true
			}
		case submissionMeta := <-metaCh:
			seenMeta = submissionMeta
		}
	}

	if !seenStart {
		t.Fatalf("expected task start event with metadata")
	}
	if seenMeta["target"] != "@internal/execution" {
		t.Fatalf("unexpected metadata: %+v", seenMeta)
	}
}
