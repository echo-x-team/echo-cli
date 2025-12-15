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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.NotFound(w, r)
			return
		}
		if got := strings.TrimSpace(r.Header.Get("Authorization")); got != "Bearer test-key" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "pong"})
	}))
	t.Cleanup(srv.Close)

	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(`
model = "gpt-4o-mini"
model_provider = "openai"

[model_providers.openai]
api_key = "test-key"
base_url = "`+srv.URL+`"
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

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

	var stdout bytes.Buffer
	run := exec.Command(binPath, "ping", "--config", cfgPath)
	run.Dir = tmp
	run.Env = append(os.Environ(),
		"HOME="+tmp,
		"OPENAI_API_KEY=",
	)
	run.Stdout = &stdout
	run.Stderr = &stdout
	if err := run.Run(); err != nil {
		t.Fatalf("ping run error: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "ok: pong") {
		t.Fatalf("ping output = %q, want it to include %q", stdout.String(), "ok: pong")
	}
}
