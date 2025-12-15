package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCheckResponsesEndpoint_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			http.Error(w, "bad path", http.StatusNotFound)
			return
		}
		if got := strings.TrimSpace(r.Header.Get("Authorization")); got != "Bearer test-key" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		var decoded struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(decoded.Model) == "" {
			http.Error(w, "missing model", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"output_text": "pong"})
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := CheckResponsesEndpoint(ctx, srv.URL, "test-key", "gpt-4o-mini")
	if err != nil {
		t.Fatalf("CheckResponsesEndpoint() error: %v", err)
	}
	if got != "pong" {
		t.Fatalf("CheckResponsesEndpoint() = %q, want %q", got, "pong")
	}
}

func TestCheckResponsesEndpoint_HTTPError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "bad key"},
		})
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := CheckResponsesEndpoint(ctx, srv.URL, "test-key", "gpt-4o-mini")
	if err == nil {
		t.Fatalf("CheckResponsesEndpoint() = nil, want error")
	}
}

func TestCheckResponsesEndpoint_EmptyText(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := CheckResponsesEndpoint(ctx, srv.URL, "test-key", "gpt-4o-mini")
	if err == nil {
		t.Fatalf("CheckResponsesEndpoint() = nil, want error")
	}
}
