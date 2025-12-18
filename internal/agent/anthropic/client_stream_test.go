package anthropic

import (
	"encoding/json"
	"testing"

	"echo-cli/internal/agent"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

func TestToolUseStreamState_EmitsToolCallAfterInputJSONDelta(t *testing.T) {
	state := newToolUseStreamState()

	var start anthropic.ContentBlockStartEvent
	if err := json.Unmarshal([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"file_read","input":{}}}`), &start); err != nil {
		t.Fatalf("unmarshal start: %v", err)
	}
	var delta1 anthropic.ContentBlockDeltaEvent
	if err := json.Unmarshal([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"README"}}`), &delta1); err != nil {
		t.Fatalf("unmarshal delta1: %v", err)
	}
	var delta2 anthropic.ContentBlockDeltaEvent
	if err := json.Unmarshal([]byte(`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":".md\"}"}}`), &delta2); err != nil {
		t.Fatalf("unmarshal delta2: %v", err)
	}

	var gotItems [][]byte
	onEvent := func(evt agent.StreamEvent) {
		if evt.Type == agent.StreamEventItem {
			gotItems = append(gotItems, []byte(evt.Item))
		}
	}

	state.Handle(start, onEvent)
	state.Handle(delta1, onEvent)
	state.Handle(delta2, onEvent)
	state.Handle(anthropic.ContentBlockStopEvent{Index: 0}, onEvent)

	if len(gotItems) != 1 {
		t.Fatalf("items = %d, want 1", len(gotItems))
	}

	var item struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		CallID    string `json:"call_id"`
	}
	if err := json.Unmarshal(gotItems[0], &item); err != nil {
		t.Fatalf("unmarshal item: %v", err)
	}
	if item.Type != "function_call" || item.Name != "file_read" || item.CallID != "toolu_1" {
		t.Fatalf("unexpected item: %#v", item)
	}
	if item.Arguments != `{"path":"README.md"}` {
		t.Fatalf("arguments = %q, want %q", item.Arguments, `{"path":"README.md"}`)
	}
}

func TestToolUseStreamState_FlushesPendingToolUseOnMessageStop(t *testing.T) {
	state := newToolUseStreamState()

	var start anthropic.ContentBlockStartEvent
	if err := json.Unmarshal([]byte(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_2","name":"file_search","input":{}}}`), &start); err != nil {
		t.Fatalf("unmarshal start: %v", err)
	}
	var delta anthropic.ContentBlockDeltaEvent
	if err := json.Unmarshal([]byte(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"README\"}"}}`), &delta); err != nil {
		t.Fatalf("unmarshal delta: %v", err)
	}

	var gotItems [][]byte
	var gotCompleted int
	onEvent := func(evt agent.StreamEvent) {
		switch evt.Type {
		case agent.StreamEventItem:
			gotItems = append(gotItems, []byte(evt.Item))
		case agent.StreamEventCompleted:
			gotCompleted++
		}
	}

	state.Handle(start, onEvent)
	state.Handle(delta, onEvent)
	state.Handle(anthropic.MessageStopEvent{}, onEvent)

	if gotCompleted != 1 {
		t.Fatalf("completed = %d, want 1", gotCompleted)
	}
	if len(gotItems) != 1 {
		t.Fatalf("items = %d, want 1", len(gotItems))
	}
	var item struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		CallID    string `json:"call_id"`
	}
	if err := json.Unmarshal(gotItems[0], &item); err != nil {
		t.Fatalf("unmarshal item: %v", err)
	}
	if item.Name != "file_search" || item.CallID != "toolu_2" || item.Arguments != `{"query":"README"}` {
		t.Fatalf("unexpected item: %#v", item)
	}
}
