package execution

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

func TestEngineStreamsAndPersistsHistory(t *testing.T) {
	bus := events.NewBus()
	manager := events.NewManager(events.ManagerConfig{SubmissionBuffer: 8, EventBuffer: 16, Workers: 2})
	engine := NewEngine(Options{
		Manager:  manager,
		Client:   fakeModelClient{chunks: []string{"hello", " world"}},
		Bus:      bus,
		Defaults: SessionDefaults{Model: "gpt-test", System: "system"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Close()

	eventsCh := engine.Events()
	subID, err := engine.SubmitUserInput(ctx, []events.InputMessage{{Role: "user", Content: "hi"}}, events.InputContext{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("submit user input: %v", err)
	}

	var outputs []events.AgentOutput
	done := false
	deadline := time.After(2 * time.Second)
	for !done {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for events, collected %d outputs", len(outputs))
		case ev := <-eventsCh:
			if ev.SubmissionID != subID {
				continue
			}
			switch ev.Type {
			case events.EventAgentOutput:
				output, ok := ev.Payload.(events.AgentOutput)
				if !ok {
					t.Fatalf("unexpected payload type %#v", ev.Payload)
				}
				outputs = append(outputs, output)
			case events.EventError:
				t.Fatalf("unexpected error payload: %v", ev.Payload)
			case events.EventTaskCompleted:
				done = true
			}
		}
	}
	if len(outputs) == 0 {
		t.Fatalf("expected at least one agent output")
	}
	last := outputs[len(outputs)-1]
	if !last.Final {
		t.Fatalf("expected final output flag, got %+v", last)
	}
	if strings.TrimSpace(last.Content) != "hello world" {
		t.Fatalf("unexpected final content: %q", last.Content)
	}

	history := engine.History("sess-1")
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	if history[0].Role != agent.RoleUser {
		t.Fatalf("first history role mismatch: %s", history[0].Role)
	}
	if history[1].Role != agent.RoleAssistant {
		t.Fatalf("second history role mismatch: %s", history[1].Role)
	}
}

func TestEngineInterruptCancelsTurn(t *testing.T) {
	manager := events.NewManager(events.ManagerConfig{SubmissionBuffer: 8, EventBuffer: 16, Workers: 2})
	bus := events.NewBus()
	engine := NewEngine(Options{
		Manager:  manager,
		Client:   slowModelClient{delay: 150 * time.Millisecond, repeat: 5},
		Bus:      bus,
		Defaults: SessionDefaults{Model: "gpt-test"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.Start(ctx)
	defer engine.Close()

	eventsCh := engine.Events()
	subID, err := engine.SubmitUserInput(ctx, []events.InputMessage{{Role: "user", Content: "long task"}}, events.InputContext{SessionID: "sess-int"})
	if err != nil {
		t.Fatalf("submit user input: %v", err)
	}

	// 等待首个输出以确认流已开始。
	waitFirst := time.After(2 * time.Second)
	for {
		select {
		case <-waitFirst:
			t.Fatal("timeout waiting for first output")
		case ev := <-eventsCh:
			if ev.SubmissionID != subID {
				continue
			}
			if ev.Type == events.EventAgentOutput {
				goto interrupt
			}
		}
	}

interrupt:
	if _, err := engine.SubmitInterrupt(ctx, "sess-int"); err != nil {
		t.Fatalf("submit interrupt: %v", err)
	}

	deadline := time.After(2 * time.Second)
	var taskResult events.TaskResult
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for cancelled task completion")
		case ev := <-eventsCh:
			if ev.SubmissionID != subID {
				continue
			}
			switch ev.Type {
			case events.EventTaskCompleted:
				if res, ok := ev.Payload.(events.TaskResult); ok {
					taskResult = res
				}
				if taskResult.Status == "failed" || taskResult.Status == "completed" {
					goto done
				}
			}
		}
	}

done:
	if taskResult.Status != "failed" {
		t.Fatalf("expected failed status after interrupt, got %+v", taskResult)
	}
	history := engine.History("sess-int")
	if len(history) != 1 {
		t.Fatalf("expected user message only in history after interrupt, got %d", len(history))
	}
}

func TestEngineLoopsToolsAndPersistsHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	bus := events.NewBus()
	manager := events.NewManager(events.ManagerConfig{SubmissionBuffer: 8, EventBuffer: 16, Workers: 2})
	client := &toolLoopModelClient{}
	engine := NewEngine(Options{
		Manager:     manager,
		Client:      client,
		Bus:         bus,
		Defaults:    SessionDefaults{Model: "gpt-test", System: "system"},
		ToolTimeout: time.Second,
	})

	engine.Start(ctx)
	defer engine.Close()

	// Simulate tool dispatcher handling the marker.
	go func() {
		sub := bus.Subscribe()
		for evt := range sub {
			marker, ok := evt.(tools.ToolCallMarker)
			if !ok || marker.ID != "call-1" {
				continue
			}
			bus.Publish(tools.ToolEvent{
				Type: "item.completed",
				Result: tools.ToolResult{
					ID:     marker.ID,
					Kind:   tools.ToolCommand,
					Status: "completed",
					Output: "tool output",
				},
			})
			return
		}
	}()

	eventsCh := engine.Events()
	subID, err := engine.SubmitUserInput(ctx, []events.InputMessage{{Role: "user", Content: "hi"}}, events.InputContext{SessionID: "sess-tools"})
	if err != nil {
		t.Fatalf("submit user input: %v", err)
	}

	var outputs []events.AgentOutput
	var taskResult events.TaskResult
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for tool loop completion, collected %d outputs", len(outputs))
		case ev := <-eventsCh:
			if ev.SubmissionID != subID {
				continue
			}
			switch ev.Type {
			case events.EventAgentOutput:
				output, ok := ev.Payload.(events.AgentOutput)
				if !ok {
					t.Fatalf("unexpected payload type %#v", ev.Payload)
				}
				outputs = append(outputs, output)
			case events.EventError:
				t.Fatalf("unexpected error payload: %v", ev.Payload)
			case events.EventTaskCompleted:
				if res, ok := ev.Payload.(events.TaskResult); ok {
					taskResult = res
				}
				goto done
			}
		}
	}

done:
	if taskResult.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", taskResult)
	}
	if client.calls < 2 {
		t.Fatalf("expected multiple model calls with tools, got %d", client.calls)
	}
	if len(outputs) == 0 || !outputs[len(outputs)-1].Final {
		t.Fatalf("expected final agent output, got %+v", outputs)
	}
	if outputs[len(outputs)-1].Content != "final answer after tool" {
		t.Fatalf("unexpected final content: %q", outputs[len(outputs)-1].Content)
	}

	history := engine.History("sess-tools")
	if len(history) != 4 {
		t.Fatalf("expected history with tool loop messages, got %d entries", len(history))
	}
	if history[1].Role != agent.RoleAssistant || !strings.Contains(history[1].Content, `"tool":"command"`) {
		t.Fatalf("expected assistant tool call recorded, got %+v", history[1])
	}
	if history[2].Role != agent.RoleUser || !strings.Contains(history[2].Content, "tool output") {
		t.Fatalf("expected tool result recorded, got %+v", history[2])
	}
	if history[3].Role != agent.RoleAssistant || history[3].Content != "final answer after tool" {
		t.Fatalf("final assistant message mismatch: %+v", history[3])
	}
}

func TestEngineDispatchesMarkersAfterFullResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	bus := events.NewBus()
	manager := events.NewManager(events.ManagerConfig{SubmissionBuffer: 8, EventBuffer: 16, Workers: 2})
	client := &splitToolLoopModelClient{}
	engine := NewEngine(Options{
		Manager:     manager,
		Client:      client,
		Bus:         bus,
		Defaults:    SessionDefaults{Model: "gpt-test", System: "system"},
		ToolTimeout: time.Second,
	})

	engine.Start(ctx)
	defer engine.Close()

	// Simulate dispatcher sending tool results after receiving the marker.
	go func() {
		sub := bus.Subscribe()
		for evt := range sub {
			marker, ok := evt.(tools.ToolCallMarker)
			if !ok || marker.ID != "split-1" {
				continue
			}
			bus.Publish(tools.ToolEvent{
				Type: "item.completed",
				Result: tools.ToolResult{
					ID:     marker.ID,
					Kind:   tools.ToolCommand,
					Status: "completed",
					Output: "tool output",
				},
			})
			return
		}
	}()

	eventsCh := engine.Events()
	subID, err := engine.SubmitUserInput(ctx, []events.InputMessage{{Role: "user", Content: "hi"}}, events.InputContext{SessionID: "sess-split"})
	if err != nil {
		t.Fatalf("submit user input: %v", err)
	}

	var outputs []events.AgentOutput
	var taskResult events.TaskResult
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for tool loop completion, collected %d outputs", len(outputs))
		case ev := <-eventsCh:
			if ev.SubmissionID != subID {
				continue
			}
			switch ev.Type {
			case events.EventAgentOutput:
				output, ok := ev.Payload.(events.AgentOutput)
				if !ok {
					t.Fatalf("unexpected payload type %#v", ev.Payload)
				}
				outputs = append(outputs, output)
			case events.EventError:
				t.Fatalf("unexpected error payload: %v", ev.Payload)
			case events.EventTaskCompleted:
				if res, ok := ev.Payload.(events.TaskResult); ok {
					taskResult = res
				}
				goto done
			}
		}
	}

done:
	if taskResult.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", taskResult)
	}
	if client.calls < 2 {
		t.Fatalf("expected multiple model calls with tools, got %d", client.calls)
	}
	if len(outputs) == 0 || !outputs[len(outputs)-1].Final {
		t.Fatalf("expected final agent output, got %+v", outputs)
	}
	if outputs[len(outputs)-1].Content != "final after split tool" {
		t.Fatalf("unexpected final content: %q", outputs[len(outputs)-1].Content)
	}
}

func TestEngineHandlesResponseItemsFromStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	bus := events.NewBus()
	manager := events.NewManager(events.ManagerConfig{SubmissionBuffer: 8, EventBuffer: 16, Workers: 2})
	client := &responseItemModelClient{}
	engine := NewEngine(Options{
		Manager:     manager,
		Client:      client,
		Bus:         bus,
		Defaults:    SessionDefaults{Model: "gpt-test", System: "system"},
		ToolTimeout: time.Second,
	})

	engine.Start(ctx)
	defer engine.Close()

	go func() {
		sub := bus.Subscribe()
		for evt := range sub {
			marker, ok := evt.(tools.ToolCallMarker)
			if !ok || marker.ID != "call-item-1" {
				continue
			}
			bus.Publish(tools.ToolEvent{
				Type: "item.completed",
				Result: tools.ToolResult{
					ID:     marker.ID,
					Kind:   tools.ToolCommand,
					Status: "completed",
					Output: "tool output from item",
				},
			})
			return
		}
	}()

	eventsCh := engine.Events()
	subID, err := engine.SubmitUserInput(ctx, []events.InputMessage{{Role: "user", Content: "hi"}}, events.InputContext{SessionID: "sess-item"})
	if err != nil {
		t.Fatalf("submit user input: %v", err)
	}

	var outputs []events.AgentOutput
	var taskResult events.TaskResult
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for response-item stream, outputs=%v", outputs)
		case ev := <-eventsCh:
			if ev.SubmissionID != subID {
				continue
			}
			switch ev.Type {
			case events.EventAgentOutput:
				output, ok := ev.Payload.(events.AgentOutput)
				if !ok {
					t.Fatalf("unexpected payload type %#v", ev.Payload)
				}
				outputs = append(outputs, output)
			case events.EventError:
				t.Fatalf("unexpected error payload: %v", ev.Payload)
			case events.EventTaskCompleted:
				if res, ok := ev.Payload.(events.TaskResult); ok {
					taskResult = res
				}
				goto done
			}
		}
	}

done:
	if taskResult.Status != "completed" {
		t.Fatalf("expected completed task, got %+v", taskResult)
	}
	if client.calls < 2 {
		t.Fatalf("expected multiple model calls with response items, got %d", client.calls)
	}
	if len(outputs) == 0 || !outputs[len(outputs)-1].Final {
		t.Fatalf("expected final agent output, got %+v", outputs)
	}
	if outputs[len(outputs)-1].Content != "final via item" {
		t.Fatalf("unexpected final content: %q", outputs[len(outputs)-1].Content)
	}
}

type fakeModelClient struct {
	chunks []string
}

func (c fakeModelClient) Complete(_ context.Context, _ agent.Prompt) (string, error) {
	return strings.Join(c.chunks, ""), nil
}

func (c fakeModelClient) Stream(ctx context.Context, _ agent.Prompt, onEvent func(agent.StreamEvent)) error {
	for _, chunk := range c.chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: chunk})
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	return nil
}

type slowModelClient struct {
	delay  time.Duration
	repeat int
}

func (c slowModelClient) Complete(_ context.Context, _ agent.Prompt) (string, error) {
	return "", nil
}

func (c slowModelClient) Stream(ctx context.Context, _ agent.Prompt, onEvent func(agent.StreamEvent)) error {
	for i := 0; i < c.repeat; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: "tick"})
		time.Sleep(c.delay)
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	return nil
}

type toolLoopModelClient struct {
	calls int
}

func (c *toolLoopModelClient) Complete(_ context.Context, _ agent.Prompt) (string, error) {
	return "", nil
}

func (c *toolLoopModelClient) Stream(ctx context.Context, _ agent.Prompt, onEvent func(agent.StreamEvent)) error {
	c.calls++
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if c.calls == 1 {
		onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: `{"tool":"command","id":"call-1","args":{"command":"echo hi"}}`})
		onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
		return nil
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: "final answer after tool"})
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	return nil
}

type splitToolLoopModelClient struct {
	calls int
}

func (c *splitToolLoopModelClient) Complete(_ context.Context, _ agent.Prompt) (string, error) {
	return "", nil
}

func (c *splitToolLoopModelClient) Stream(ctx context.Context, _ agent.Prompt, onEvent func(agent.StreamEvent)) error {
	c.calls++
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if c.calls == 1 {
		chunks := []string{"```tool\n", `{"tool":"command","id":"split-1","args":{"command":"echo hi"}}`, "\n```"}
		for _, ch := range chunks {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: ch})
		}
		onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
		return nil
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: "final after split tool"})
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	return nil
}

type responseItemModelClient struct {
	calls int
}

func (c *responseItemModelClient) Complete(_ context.Context, _ agent.Prompt) (string, error) {
	return "", nil
}

func (c *responseItemModelClient) Stream(ctx context.Context, _ agent.Prompt, onEvent func(agent.StreamEvent)) error {
	c.calls++
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if c.calls == 1 {
		item := ResponseItem{
			Type: ResponseItemTypeFunctionCall,
			FunctionCall: &FunctionCallResponseItem{
				Name:      "command",
				Arguments: `{"command":"echo hi"}`,
				CallID:    "call-item-1",
			},
		}
		raw, _ := json.Marshal(item)
		onEvent(agent.StreamEvent{Type: agent.StreamEventItem, Item: raw})
		onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
		return nil
	}
	onEvent(agent.StreamEvent{Type: agent.StreamEventTextDelta, Text: "final via item"})
	onEvent(agent.StreamEvent{Type: agent.StreamEventCompleted})
	return nil
}
