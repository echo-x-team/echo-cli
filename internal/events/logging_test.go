package events

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"echo-cli/internal/logger"
	"github.com/sirupsen/logrus"
)

func TestSubmissionQueueLogsJSONPayload(t *testing.T) {
	buf := &bytes.Buffer{}
	q := NewSubmissionQueue(1)
	q.SetLogger(newBufferLogger(buf))

	sub := Submission{
		ID: "s1",
		Operation: Operation{
			Kind:      OperationUserInput,
			UserInput: &UserInputOperation{Items: []InputMessage{{Role: "user", Content: "ping"}}},
		},
	}

	if err := q.Submit(context.Background(), sub); err != nil {
		t.Fatalf("submit: %v", err)
	}

	payload := extractField(buf.String(), "payload")
	if payload == "" {
		t.Fatalf("expected payload field in log, got %q", buf.String())
	}
	if !json.Valid([]byte(payload)) {
		t.Fatalf("payload not valid json: %s", payload)
	}
}

func TestEventQueueLogsJSONPayload(t *testing.T) {
	buf := &bytes.Buffer{}
	q := NewEventQueue(1)
	q.SetLogger(newBufferLogger(buf))

	ev := Event{
		Type:         EventAgentOutput,
		SubmissionID: "s1",
		SessionID:    "sess",
		Payload: AgentOutput{
			Content:  "pong",
			Final:    true,
			Sequence: 2,
		},
	}

	if err := q.Publish(context.Background(), ev); err != nil {
		t.Fatalf("publish: %v", err)
	}

	payload := extractField(buf.String(), "payload")
	if payload == "" {
		t.Fatalf("expected payload field in log, got %q", buf.String())
	}
	if !json.Valid([]byte(payload)) {
		t.Fatalf("payload not valid json: %s", payload)
	}
}

func newBufferLogger(buf *bytes.Buffer) *logger.LogEntry {
	l := logrus.New()
	l.SetFormatter(logger.PlainFormatter{})
	l.SetOutput(buf)
	return logrus.NewEntry(l)
}

func extractField(line, key string) string {
	idx := strings.Index(line, key+"=")
	if idx == -1 {
		return ""
	}
	start := idx + len(key) + 1
	end := strings.Index(line[start:], " ")
	if end == -1 {
		end = len(line)
	} else {
		end += start
	}
	return strings.TrimSpace(line[start:end])
}
