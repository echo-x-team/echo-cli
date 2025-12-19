package tools

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"echo-cli/internal/logger"
	"github.com/sirupsen/logrus"
)

func withBufferedToolsLogger(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := new(bytes.Buffer)
	l := logrus.New()
	l.SetOutput(buf)
	l.SetFormatter(logger.PlainFormatter{})
	l.SetReportCaller(false)

	toolsLogMu.Lock()
	prevLog := toolsLog
	prevConfigured := toolsLogConfigured
	prevCloser := toolsLogCloser
	prevPath := toolsLogPath

	toolsLog = logrus.NewEntry(l).WithField("component", "tools")
	toolsLogConfigured = true
	toolsLogCloser = nil
	toolsLogPath = "(test)"
	toolsLogMu.Unlock()

	t.Cleanup(func() {
		toolsLogMu.Lock()
		toolsLog = prevLog
		toolsLogConfigured = prevConfigured
		toolsLogCloser = prevCloser
		toolsLogPath = prevPath
		toolsLogMu.Unlock()
	})

	return buf
}

func TestToolsLogIncludesPayloadOnCallAndResult(t *testing.T) {
	buf := withBufferedToolsLogger(t)

	call := ToolCall{
		ID:      "1",
		Name:    "command",
		Payload: []byte("{\n  \"command\": \"echo hi\"\n}"),
	}

	logToolRequest(call, ToolCommand, true, "wd")
	logToolResult(call, ToolCommand, ToolResult{ID: "1", Kind: ToolCommand, Status: "error", Error: "boom\nfail", ExitCode: 7}, "wd", 120*time.Millisecond)

	out := buf.String()
	if !strings.Contains(out, "tool_call id=1 name=command") {
		t.Fatalf("missing tool_call log, got:\n%s", out)
	}
	if !strings.Contains(out, "tool_result id=1 name=command") {
		t.Fatalf("missing tool_result log, got:\n%s", out)
	}
	if !strings.Contains(out, "payload={\\n  \"command\": \"echo hi\"\\n}") {
		t.Fatalf("missing payload in log, got:\n%s", out)
	}
	if !strings.Contains(out, "error=boom\\nfail") {
		t.Fatalf("expected sanitized error in log, got:\n%s", out)
	}
	if !strings.Contains(out, "duration_ms=120") {
		t.Fatalf("expected duration in log, got:\n%s", out)
	}
}
