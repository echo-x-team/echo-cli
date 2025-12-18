package main

import "testing"

func TestApplyRuntimeKVOverrides_ToolTimeoutSeconds(t *testing.T) {
	cfg := defaultRuntimeConfig()
	if cfg.ToolTimeoutSecs == 0 {
		t.Fatalf("expected default ToolTimeoutSecs > 0")
	}

	got := applyRuntimeKVOverrides(cfg, []string{"tool_timeout_seconds=900"})
	if got.ToolTimeoutSecs != 900 {
		t.Fatalf("expected ToolTimeoutSecs=900, got %d", got.ToolTimeoutSecs)
	}
}
