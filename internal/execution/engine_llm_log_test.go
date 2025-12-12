package execution

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"echo-cli/internal/agent"
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
	prompt := Prompt{
		Model: "gpt-test",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	}

	logPrompt(sub, prompt)
	promptEntries := hook.snapshot()
	if len(promptEntries) == 0 {
		t.Fatalf("expected prompt logs")
	}
	for _, e := range promptEntries {
		dir, _ := e.Data["dir"].(string)
		if dir != llmDirAgentToLLM {
			t.Fatalf("expected dir=%s, got %v (msg=%q)", llmDirAgentToLLM, e.Data["dir"], e.Message)
		}
		if !strings.HasPrefix(e.Message, "agent->llm ") {
			t.Fatalf("expected agent->llm prefix, got %q", e.Message)
		}
	}

	hook.reset()

	engine := &Engine{
		client:         fakeModelClient{chunks: []string{"hello"}},
		requestTimeout: 500 * time.Millisecond,
		retries:        0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := engine.streamPrompt(ctx, prompt, func(agent.StreamEvent) {}); err != nil {
		t.Fatalf("streamPrompt failed: %v", err)
	}

	streamEntries := hook.snapshot()
	foundOut := false
	foundIn := false
	for _, e := range streamEntries {
		switch {
		case strings.HasPrefix(e.Message, "agent->llm "):
			foundOut = true
			dir, _ := e.Data["dir"].(string)
			if dir != llmDirAgentToLLM {
				t.Fatalf("expected outgoing dir=%s, got %v (msg=%q)", llmDirAgentToLLM, e.Data["dir"], e.Message)
			}
		case strings.HasPrefix(e.Message, "llm->agent "):
			foundIn = true
			dir, _ := e.Data["dir"].(string)
			if dir != llmDirLLMToAgent {
				t.Fatalf("expected incoming dir=%s, got %v (msg=%q)", llmDirLLMToAgent, e.Data["dir"], e.Message)
			}
		}
	}
	if !foundOut || !foundIn {
		t.Fatalf("expected both outgoing and incoming llm logs, foundOut=%t foundIn=%t entries=%d", foundOut, foundIn, len(streamEntries))
	}
}
