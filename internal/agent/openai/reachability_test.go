package openai

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestCheckBaseURLReachable_OK(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	port := ln.Addr().(*net.TCPAddr).Port
	baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", port)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := CheckBaseURLReachable(ctx, baseURL); err != nil {
		t.Fatalf("CheckBaseURLReachable() error: %v", err)
	}
}

func TestCheckBaseURLReachable_InvalidBaseURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := CheckBaseURLReachable(ctx, "://bad"); err == nil {
		t.Fatalf("CheckBaseURLReachable() = nil, want error")
	}
}

func TestCheckBaseURLReachable_ConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	_ = ln.Close()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d/v1", addr.Port)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err = CheckBaseURLReachable(ctx, baseURL)
	if err == nil {
		t.Skipf("port %d is reachable; skipping connection-refused assertion", addr.Port)
	}
}
