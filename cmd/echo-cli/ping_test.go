package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPingCommand_BinaryRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}

	expectedModel := "glm4.6.test"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		if got := strings.TrimSpace(r.Header.Get("X-Api-Key")); got != "test-key" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if got, _ := payload["model"].(string); strings.TrimSpace(got) != expectedModel {
			http.Error(w, "unexpected model", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_1",
			"type":  "message",
			"role":  "assistant",
			"model": expectedModel,
			"content": []map[string]any{
				{"type": "text", "text": "pong", "citations": []any{}},
			},
			"stop_reason":   "end_turn",
			"stop_sequence": "",
			"usage": map[string]any{
				"cache_creation": map[string]any{
					"ephemeral_1h_input_tokens": 0,
					"ephemeral_5m_input_tokens": 0,
				},
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
				"input_tokens":                1,
				"output_tokens":               1,
				"server_tool_use": map[string]any{
					"web_search_requests": 0,
				},
				"service_tier": "standard",
			},
		})
	}))
	t.Cleanup(srv.Close)

	tmp := t.TempDir()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		t.Fatalf("missing go.mod at %s: %v", repoRoot, err)
	}

	binPath := filepath.Join(tmp, "echo-cli")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/echo-cli")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "HOME="+tmp)
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build error: %v\n%s", err, string(out))
	}

	tests := []struct {
		name string
		url  string
	}{
		{name: "base", url: srv.URL},
		{name: "base_with_v1_suffix", url: srv.URL + "/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgPath := filepath.Join(tmp, "config-"+tt.name+".toml")
			if err := os.WriteFile(cfgPath, []byte(`
url = "`+tt.url+`"
token = "test-key"
model = "`+expectedModel+`"
`), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			var stdout bytes.Buffer
			run := exec.Command(binPath, "ping", "--config", cfgPath)
			run.Dir = tmp
			run.Env = append(os.Environ(),
				"HOME="+tmp,
				"ANTHROPIC_BASE_URL=",
				"ANTHROPIC_AUTH_TOKEN=",
			)
			run.Stdout = &stdout
			run.Stderr = &stdout
			if err := run.Run(); err != nil {
				t.Fatalf("ping run error: %v\n%s", err, stdout.String())
			}
			if !strings.Contains(stdout.String(), "ok: pong") {
				t.Fatalf("ping output = %q, want it to include %q", stdout.String(), "ok: pong")
			}
		})
	}
}
