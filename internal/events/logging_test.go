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

	out := buf.String()
	if !strings.Contains(out, "payload=") {
		t.Fatalf("expected payload field in log, got %q", out)
	}
	// 缩进 JSON 会带换行，直接检查关键字段存在即可。
	if !strings.Contains(out, "\"Kind\"") {
		t.Fatalf("expected json payload in log, got %q", out)
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

	out := buf.String()
	if !strings.Contains(out, "payload=") {
		t.Fatalf("expected payload field in log, got %q", out)
	}
	if !strings.Contains(out, "\"Content\"") {
		t.Fatalf("expected json payload in log, got %q", out)
	}
}

func TestEncodePayload_StringIsRaw(t *testing.T) {
	if got := encodePayload("user_input"); got != "user_input" {
		t.Fatalf("expected raw string payload, got %q", got)
	}
}

func TestEncodePayload_ObjectIsPrettyJSON(t *testing.T) {
	got := encodePayload(map[string]any{"a": 1, "b": map[string]any{"c": 2}})
	if !json.Valid([]byte(got)) {
		t.Fatalf("expected valid json, got %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected pretty json with newlines, got %q", got)
	}
}

func TestEncodePayload_JSONStringWithEscapedNewlines(t *testing.T) {
	in := "{\\n  \"a\": 1,\\n  \"b\": 2\\n}"
	got := encodePayload(in)
	if strings.Contains(got, `\\n`) || strings.Contains(got, `\n`) {
		t.Fatalf("expected escaped newlines to be unescaped, got %q", got)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("expected valid json after unescape/pretty, got %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected output to contain real newlines, got %q", got)
	}
}

func newBufferLogger(buf *bytes.Buffer) *logger.LogEntry {
	l := logrus.New()
	l.SetFormatter(logger.PlainFormatter{})
	l.SetOutput(buf)
	return logrus.NewEntry(l)
}
