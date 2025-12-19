package execution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestSetupLLMLogUsesPrettyJSONFormatter(t *testing.T) {
	oldLLMLog := llmLog

	llmLogMu.Lock()
	oldConfigured := llmLogConfigured
	oldCloser := llmLogCloser
	oldPath := llmLogPath
	llmLogConfigured = false
	llmLogCloser = nil
	llmLogPath = ""
	llmLogMu.Unlock()

	t.Cleanup(func() {
		CloseLLMLog()
		llmLogMu.Lock()
		llmLog = oldLLMLog
		llmLogConfigured = oldConfigured
		llmLogCloser = oldCloser
		llmLogPath = oldPath
		llmLogMu.Unlock()
	})

	path := filepath.Join(t.TempDir(), "llm.log")
	closer, _, err := SetupLLMLog(path)
	if err != nil {
		t.Fatalf("SetupLLMLog failed: %v", err)
	}
	if closer != nil {
		t.Cleanup(func() { _ = closer.Close() })
	}

	formatter, ok := llmLog.Logger.Formatter.(*logrus.JSONFormatter)
	if !ok {
		t.Fatalf("expected *logrus.JSONFormatter, got %T", llmLog.Logger.Formatter)
	}
	if !formatter.PrettyPrint {
		t.Fatalf("expected PrettyPrint=true")
	}

	llmLog.WithField("request_payload", json.RawMessage(`{"a":1,"b":2}`)).Info("hello")
	_ = closer.Close()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read llm log: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "\n") {
		t.Fatalf("expected pretty JSON with newlines, got %q", text)
	}
	if !strings.Contains(text, `"request_payload": {`) {
		t.Fatalf("expected request_payload object, got %q", text)
	}
	if !strings.Contains(text, `"a": 1`) {
		t.Fatalf("expected nested payload to be indented/expanded, got %q", text)
	}
}
