package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"echo-cli/internal/agent"
	"echo-cli/internal/logger"
)

func silenceRootLogger(t *testing.T) {
	t.Helper()
	root := logger.Root()
	prev := root.Out
	root.SetOutput(io.Discard)
	t.Cleanup(func() {
		root.SetOutput(prev)
	})
}

func TestComplete_UsesResponses_NoFallback(t *testing.T) {
	silenceRootLogger(t)

	type testCase struct {
		name       string
		statusCode int
		body       string
		wantText   string
		wantErr    bool
		wantMarker string
	}

	cases := []testCase{
		{
			name:       "success",
			statusCode: http.StatusOK,
			body: `{
  "id": "resp_test",
  "object": "response",
  "created_at": 0,
  "error": {"code": "", "message": ""},
  "model": "gpt-5.2",
  "output": [
    {
      "id": "msg_1",
      "type": "message",
      "role": "assistant",
      "status": "completed",
      "content": [{"type": "output_text", "text": "ok"}]
    }
  ]
}`,
			wantText: "ok",
		},
		{
			name:       "http_404_no_fallback",
			statusCode: http.StatusNotFound,
			body:       `{"message":"not found"}`,
			wantErr:    true,
			wantMarker: "http_404",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var responsesCalls atomic.Int64
			var chatCalls atomic.Int64

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/responses":
					responsesCalls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.statusCode)
					_, _ = w.Write([]byte(tc.body))
				case "/v1/chat/completions":
					chatCalls.Add(1)
					http.Error(w, "unexpected chat call", http.StatusTeapot)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)

			client, err := New(Options{
				APIKey:  "test",
				BaseURL: srv.URL + "/v1",
				Model:   "gpt-5.2",
				WireAPI: "responses",
			})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			t.Cleanup(cancel)

			got, err := client.Complete(ctx, agent.Prompt{
				Model: "gpt-5.2",
				Messages: []agent.Message{
					{Role: agent.RoleUser, Content: "hi"},
				},
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Complete() expected error")
				}
				if tc.wantMarker != "" && !strings.Contains(err.Error(), tc.wantMarker) {
					t.Fatalf("Complete() error = %q, want it to include %q", err.Error(), tc.wantMarker)
				}
			} else {
				if err != nil {
					t.Fatalf("Complete() error: %v", err)
				}
				if got != tc.wantText {
					t.Fatalf("Complete() = %q, want %q", got, tc.wantText)
				}
			}

			if responsesCalls.Load() != 1 {
				t.Fatalf("responses calls = %d, want %d", responsesCalls.Load(), 1)
			}
			if chatCalls.Load() != 0 {
				t.Fatalf("chat calls = %d, want %d", chatCalls.Load(), 0)
			}
		})
	}
}

func TestComplete_HTTP400LogsRawResponseBody(t *testing.T) {
	silenceRootLogger(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"bad request from proxy"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(Options{
		APIKey:  "test",
		BaseURL: srv.URL + "/v1",
		Model:   "gpt-5.2",
		WireAPI: "responses",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	_, err = client.Complete(ctx, agent.Prompt{
		Model: "gpt-5.2",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	})
	if err == nil {
		t.Fatalf("Complete() expected error")
	}
	if !strings.Contains(err.Error(), `http_400`) {
		t.Fatalf("Complete() error = %q, want it to include http_400 marker", err.Error())
	}
	if !strings.Contains(err.Error(), `{"message":"bad request from proxy"}`) {
		t.Fatalf("Complete() error = %q, want it to include raw response body", err.Error())
	}
}

func TestStream_ResponsesSSE(t *testing.T) {
	silenceRootLogger(t)

	var responsesCalls atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			responsesCalls.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			flusher, _ := w.(http.Flusher)
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\"}\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(Options{
		APIKey:  "test",
		BaseURL: srv.URL + "/v1",
		Model:   "gpt-5.2",
		WireAPI: "responses",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	var deltas []string
	var completed bool
	err = client.Stream(ctx, agent.Prompt{
		Model: "gpt-5.2",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	}, func(ev agent.StreamEvent) {
		switch ev.Type {
		case agent.StreamEventTextDelta:
			deltas = append(deltas, ev.Text)
		case agent.StreamEventCompleted:
			completed = true
		}
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if got := strings.Join(deltas, ""); got != "ok" {
		t.Fatalf("Stream deltas = %q, want %q", got, "ok")
	}
	if !completed {
		t.Fatalf("Stream did not emit completed event")
	}
	if responsesCalls.Load() != 1 {
		t.Fatalf("responses calls = %d, want %d", responsesCalls.Load(), 1)
	}
}

func TestNew_BaseURLWithoutV1UsesV1Path(t *testing.T) {
	silenceRootLogger(t)

	var responsesCalls atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			responsesCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "id": "resp_test",
  "object": "response",
  "created_at": 0,
  "error": {"code": "server_error", "message": ""},
  "incomplete_details": {},
  "instructions": "",
  "metadata": {},
  "model": "gpt-5.2",
  "output": [
    {
      "id": "msg_1",
      "type": "message",
      "role": "assistant",
      "status": "completed",
      "content": [{"type": "output_text", "text": "ok"}]
    }
  ],
  "parallel_tool_calls": false,
  "temperature": 1,
  "tool_choice": "auto",
  "tools": [],
  "top_p": 1
}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(Options{
		APIKey:  "test",
		BaseURL: srv.URL, // no /v1
		Model:   "gpt-5.2",
		WireAPI: "responses",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	got, err := client.Complete(ctx, agent.Prompt{
		Model: "gpt-5.2",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("Complete() = %q, want %q", got, "ok")
	}
	if responsesCalls.Load() != 1 {
		t.Fatalf("responses calls = %d, want %d", responsesCalls.Load(), 1)
	}
}

func TestResponses_RequestIncludesOutputSchemaAndReasoningEffort(t *testing.T) {
	silenceRootLogger(t)

	var sawTextControls bool
	var sawReasoningEffort bool
	var sawInstructions bool
	var sawInput bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			body, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()

			var payload map[string]any
			_ = json.Unmarshal(body, &payload)

			if inst, ok := payload["instructions"].(string); ok && strings.Contains(inst, "推理强度：high") {
				sawInstructions = true
			}
			if input, ok := payload["input"].([]any); ok && len(input) > 0 {
				sawInput = true
			}
			if reasoning, ok := payload["reasoning"].(map[string]any); ok {
				if effort, ok := reasoning["effort"].(string); ok && effort == "high" {
					sawReasoningEffort = true
				}
			}
			if text, ok := payload["text"].(map[string]any); ok {
				if format, ok := text["format"].(map[string]any); ok {
					if typ, ok := format["type"].(string); ok && typ == "json_schema" {
						if strict, ok := format["strict"].(bool); ok && strict {
							if schema, ok := format["schema"].(map[string]any); ok {
								if schemaType, ok := schema["type"].(string); ok && schemaType == "object" {
									sawTextControls = true
								}
							}
						}
					}
				}
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "id": "resp_test",
  "object": "response",
  "created_at": 0,
  "error": {"code": "", "message": ""},
  "model": "gpt-5.2",
  "output": [
    {
      "id": "msg_1",
      "type": "message",
      "role": "assistant",
      "status": "completed",
      "content": [{"type": "output_text", "text": "ok"}]
    }
  ]
}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(Options{
		APIKey:  "test",
		BaseURL: srv.URL, // no /v1 to also cover normalizeBaseURL
		Model:   "gpt-5.2",
		WireAPI: "responses",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	got, err := client.Complete(ctx, agent.Prompt{
		Model:        "gpt-5.2",
		OutputSchema: `{"type":"object","properties":{"ok":{"type":"boolean"}},"required":["ok"],"additionalProperties":false}`,
		Messages: []agent.Message{
			{Role: agent.RoleSystem, Content: "推理强度：high"},
			{Role: agent.RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("Complete() = %q, want %q", got, "ok")
	}
	if !sawInstructions {
		t.Fatalf("expected instructions to include reasoning effort prompt")
	}
	if !sawInput {
		t.Fatalf("expected input items in responses request")
	}
	if !sawReasoningEffort {
		t.Fatalf("expected reasoning.effort=high in responses request")
	}
	if !sawTextControls {
		t.Fatalf("expected text.format json_schema controls in responses request")
	}
}

func TestNormalizeBaseURL_EnsuresV1AndStripsEndpointSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "https://api.openai.com", want: "https://api.openai.com/v1"},
		{in: "https://api.openai.com/", want: "https://api.openai.com/v1"},
		{in: "https://example.com/openai", want: "https://example.com/openai/v1"},
		{in: "https://example.com/openai/v1", want: "https://example.com/openai/v1"},
		{in: "https://example.com/openai/v1/", want: "https://example.com/openai/v1"},
		{in: "https://example.com/openai/v1/responses", want: "https://example.com/openai/v1"},
		{in: "https://example.com/openai/v1/responses/", want: "https://example.com/openai/v1"},
		{in: "https://example.com/v1/v1", want: "https://example.com/v1"},
		{in: "https://example.com/openai?foo=bar", want: "https://example.com/openai/v1?foo=bar"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeBaseURL(tc.in); got != tc.want {
				t.Fatalf("normalizeBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestComplete_BaseURLWithPathPrefix_AppendsV1(t *testing.T) {
	silenceRootLogger(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/proxy/v1/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "id": "resp_test",
  "object": "response",
  "created_at": 0,
  "error": {"code": "", "message": ""},
  "model": "gpt-5.2",
  "output": [
    {
      "id": "msg_1",
      "type": "message",
      "role": "assistant",
      "status": "completed",
      "content": [{"type": "output_text", "text": "ok"}]
    }
  ]
}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(Options{
		APIKey:  "test",
		BaseURL: srv.URL + "/proxy",
		Model:   "gpt-5.2",
		WireAPI: "responses",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	got, err := client.Complete(ctx, agent.Prompt{
		Model: "gpt-5.2",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("Complete() = %q, want %q", got, "ok")
	}
}

func TestComplete_BaseURLWithEndpointSuffix_StripsToBase(t *testing.T) {
	silenceRootLogger(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/responses":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "id": "resp_test",
  "object": "response",
  "created_at": 0,
  "error": {"code": "", "message": ""},
  "model": "gpt-5.2",
  "output": [
    {
      "id": "msg_1",
      "type": "message",
      "role": "assistant",
      "status": "completed",
      "content": [{"type": "output_text", "text": "ok"}]
    }
  ]
}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(Options{
		APIKey:  "test",
		BaseURL: srv.URL + "/v1/responses",
		Model:   "gpt-5.2",
		WireAPI: "responses",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	got, err := client.Complete(ctx, agent.Prompt{
		Model: "gpt-5.2",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("Complete() = %q, want %q", got, "ok")
	}
}
