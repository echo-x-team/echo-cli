package execution

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"echo-cli/internal/agent"
	echocontext "echo-cli/internal/context"
	"echo-cli/internal/events"
	"github.com/sirupsen/logrus"
)

type captureHook struct {
	mu      sync.Mutex
	entries []capturedEntry
}

type capturedEntry struct {
	Message string
	Data    logrus.Fields
}

func (h *captureHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *captureHook) Fire(e *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	data := logrus.Fields{}
	for k, v := range e.Data {
		data[k] = v
	}
	h.entries = append(h.entries, capturedEntry{
		Message: e.Message,
		Data:    data,
	})
	return nil
}

func (h *captureHook) snapshot() []capturedEntry {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]capturedEntry, len(h.entries))
	copy(out, h.entries)
	return out
}

func (h *captureHook) reset() {
	h.mu.Lock()
	h.entries = nil
	h.mu.Unlock()
}

func TestLLMLogIncludesDirectionForPromptAndStream(t *testing.T) {
	oldLLMLog := llmLog
	defer func() { llmLog = oldLLMLog }()

	l := logrus.New()
	l.SetOutput(io.Discard)
	hook := &captureHook{}
	l.AddHook(hook)
	llmLog = logrus.NewEntry(l)

	sub := events.Submission{SessionID: "sess-llm", ID: "sub-1"}
	prompt := agent.Prompt{
		Model: "gpt-test",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	}

	engine := &Engine{
		client:         fakeModelClient{chunks: []string{"hello"}, usage: &agent.TokenUsage{InputTokens: 12, OutputTokens: 5}},
		requestTimeout: 500 * time.Millisecond,
		retries:        0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	seq := 0
	if _, err := engine.runModelInteraction(ctx, sub, prompt, discardPublisher{}, &seq); err != nil {
		t.Fatalf("runModelInteraction failed: %v", err)
	}

	streamEntries := hook.snapshot()
	foundOut := false
	foundIn := false
	var outEntry *capturedEntry
	var inEntry *capturedEntry
	for _, e := range streamEntries {
		switch {
		case strings.HasPrefix(e.Message, "agent->llm "):
			foundOut = true
			tmp := e
			outEntry = &tmp
			dir, _ := e.Data["direction"].(string)
			if dir != llmDirAgentToLLM {
				t.Fatalf("expected outgoing direction=%s, got %v (msg=%q)", llmDirAgentToLLM, e.Data["direction"], e.Message)
			}
		case strings.HasPrefix(e.Message, "llm->agent "):
			foundIn = true
			tmp := e
			inEntry = &tmp
			dir, _ := e.Data["direction"].(string)
			if dir != llmDirLLMToAgent {
				t.Fatalf("expected incoming direction=%s, got %v (msg=%q)", llmDirLLMToAgent, e.Data["direction"], e.Message)
			}
		}
	}
	if !foundOut || !foundIn {
		t.Fatalf("expected both outgoing and incoming llm logs, foundOut=%t foundIn=%t entries=%d", foundOut, foundIn, len(streamEntries))
	}
	if outEntry == nil || inEntry == nil {
		t.Fatalf("expected captured entries, got out=%v in=%v", outEntry, inEntry)
	}
	if _, ok := outEntry.Data["request_tokens"]; !ok {
		t.Fatalf("expected request_tokens in outgoing log: %+v", outEntry.Data)
	}
	if _, ok := inEntry.Data["response_tokens"]; !ok {
		t.Fatalf("expected response_tokens in incoming log: %+v", inEntry.Data)
	}
	if _, ok := inEntry.Data["usage_total_tokens"]; !ok {
		t.Fatalf("expected usage_total_tokens in incoming log: %+v", inEntry.Data)
	}
	if payload, ok := outEntry.Data["request_payload"].(json.RawMessage); ok {
		if !json.Valid(payload) {
			t.Fatalf("expected outgoing payload to be valid json, got %q", string(payload))
		}
	}
	if payload, ok := inEntry.Data["response_payload"].(json.RawMessage); ok {
		if !json.Valid(payload) {
			t.Fatalf("expected incoming payload to be valid json, got %q", string(payload))
		}
	}
}

func TestLLMLogIncludesStopReasons(t *testing.T) {
	oldLLMLog := llmLog
	defer func() { llmLog = oldLLMLog }()

	l := logrus.New()
	l.SetOutput(io.Discard)
	hook := &captureHook{}
	l.AddHook(hook)
	llmLog = logrus.NewEntry(l)

	sub := events.Submission{SessionID: "sess-llm-stop", ID: "sub-stop"}
	prompt := agent.Prompt{
		Model: "gpt-test",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	}

	engine := &Engine{
		client: fakeModelClient{
			chunks:       []string{"hello"},
			stopReason:   "end_turn",
			stopSequence: "",
			finishReason: "stop",
		},
		requestTimeout: 500 * time.Millisecond,
		retries:        0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	seq := 0
	if _, err := engine.runModelInteraction(ctx, sub, prompt, discardPublisher{}, &seq); err != nil {
		t.Fatalf("runModelInteraction failed: %v", err)
	}

	var entry *capturedEntry
	for _, e := range hook.snapshot() {
		if typ, _ := e.Data["type"].(string); typ == "llm.response" {
			tmp := e
			entry = &tmp
			break
		}
	}
	if entry == nil {
		t.Fatalf("expected llm.response entry")
	}
	if got, _ := entry.Data["stop_reason"].(string); got != "end_turn" {
		t.Fatalf("expected stop_reason=end_turn, got %v", entry.Data["stop_reason"])
	}
	if got, _ := entry.Data["stop_sequence"].(string); got != "" {
		t.Fatalf("expected stop_sequence empty, got %v", entry.Data["stop_sequence"])
	}
	if got, _ := entry.Data["finish_reason"].(string); got != "stop" {
		t.Fatalf("expected finish_reason=stop, got %v", entry.Data["finish_reason"])
	}
}

type discardPublisher struct{}

func (discardPublisher) Publish(_ context.Context, _ events.Event) error { return nil }

func TestRunTaskLogsExitReasonToLLMLog(t *testing.T) {
	oldLLMLog := llmLog
	oldErrorLog := errorLog
	defer func() {
		llmLog = oldLLMLog
		errorLog = oldErrorLog
	}()

	l := logrus.New()
	l.SetOutput(io.Discard)
	hook := &captureHook{}
	l.AddHook(hook)
	llmLog = logrus.NewEntry(l)
	errorLog = logrus.NewEntry(l)

	engine := &Engine{
		contexts:       echocontext.NewContextManager(echocontext.SessionDefaults{Model: "gpt-test"}),
		client:         fakeModelClient{chunks: []string{"hello"}},
		requestTimeout: 500 * time.Millisecond,
		retries:        0,
		toolCtx:        map[string]toolCallContext{},
	}

	sub := events.Submission{SessionID: "sess-exit", ID: "sub-exit"}
	state := echocontext.TurnState{
		Context: echocontext.TurnContext{
			Model: "gpt-test",
			History: []agent.Message{
				{Role: agent.RoleUser, Content: "hi"},
			},
		},
	}

	if err := engine.runTask(context.Background(), sub, state, discardPublisher{}); err != nil {
		t.Fatalf("runTask failed: %v", err)
	}

	var found capturedEntry
	ok := false
	for _, e := range hook.snapshot() {
		if typ, _ := e.Data["type"].(string); typ == "run_task.exit" {
			found = e
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("expected run_task.exit log entry")
	}
	if reason, _ := found.Data["exit_reason"].(string); reason != "completed_final" {
		t.Fatalf("expected exit_reason=completed_final, got %v (msg=%q)", found.Data["exit_reason"], found.Message)
	}
	if stage, _ := found.Data["exit_stage"].(string); stage != "final_no_responses" {
		t.Fatalf("expected exit_stage=final_no_responses, got %v (msg=%q)", found.Data["exit_stage"], found.Message)
	}
	if dir, _ := found.Data["direction"].(string); dir != "agent" {
		t.Fatalf("expected direction=agent, got %v (msg=%q)", found.Data["direction"], found.Message)
	}
}

func TestRunTaskLogsExitReasonWhenContextCancelled(t *testing.T) {
	oldLLMLog := llmLog
	oldErrorLog := errorLog
	defer func() {
		llmLog = oldLLMLog
		errorLog = oldErrorLog
	}()

	l := logrus.New()
	l.SetOutput(io.Discard)
	hook := &captureHook{}
	l.AddHook(hook)
	llmLog = logrus.NewEntry(l)
	errorLog = logrus.NewEntry(l)

	engine := &Engine{
		contexts:       echocontext.NewContextManager(echocontext.SessionDefaults{Model: "gpt-test"}),
		client:         fakeModelClient{chunks: []string{"hello"}},
		requestTimeout: 500 * time.Millisecond,
		retries:        0,
		toolCtx:        map[string]toolCallContext{},
	}

	sub := events.Submission{SessionID: "sess-cancel", ID: "sub-cancel"}
	state := echocontext.TurnState{Context: echocontext.TurnContext{Model: "gpt-test"}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := engine.runTask(ctx, sub, state, discardPublisher{}); err == nil {
		t.Fatalf("expected cancellation error")
	}

	var found capturedEntry
	ok := false
	for _, e := range hook.snapshot() {
		if typ, _ := e.Data["type"].(string); typ == "run_task.exit" {
			found = e
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("expected run_task.exit log entry")
	}
	if reason, _ := found.Data["exit_reason"].(string); reason != "context_done" {
		t.Fatalf("expected exit_reason=context_done, got %v (msg=%q)", found.Data["exit_reason"], found.Message)
	}
	if stage, _ := found.Data["exit_stage"].(string); stage != "ctx_check" {
		t.Fatalf("expected exit_stage=ctx_check, got %v (msg=%q)", found.Data["exit_stage"], found.Message)
	}
}
