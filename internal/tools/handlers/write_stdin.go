package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"echo-cli/internal/tools"
)

type WriteStdinHandler struct{}

func (WriteStdinHandler) Name() string           { return "write_stdin" }
func (WriteStdinHandler) Kind() tools.ToolKind   { return tools.ToolCommand }
func (WriteStdinHandler) SupportsParallel() bool { return false }
func (WriteStdinHandler) IsMutating(tools.Invocation) bool {
	return true
}

func (WriteStdinHandler) Describe(inv tools.Invocation) tools.ToolResult {
	args := struct {
		SessionID string `json:"session_id"`
	}{}
	_ = json.Unmarshal(inv.Call.Payload, &args)
	return tools.ToolResult{
		ID:        inv.Call.ID,
		Kind:      tools.ToolCommand,
		Command:   "write_stdin",
		SessionID: strings.TrimSpace(args.SessionID),
	}
}

func (WriteStdinHandler) Handle(ctx context.Context, inv tools.Invocation) (tools.ToolResult, error) {
	args := struct {
		SessionID      string `json:"session_id"`
		Chars          string `json:"chars"`
		YieldTimeMs    int    `json:"yield_time_ms"`
		MaxOutputBytes int    `json:"max_output_bytes"`
	}{}
	if err := json.Unmarshal(inv.Call.Payload, &args); err != nil || strings.TrimSpace(args.SessionID) == "" {
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolCommand,
			Status: "error",
			Error:  "invalid write_stdin payload",
		}, fmt.Errorf("invalid write_stdin payload: %w", err)
	}
	if inv.UnifiedExec == nil {
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolCommand,
			Status: "error",
			Error:  "unified exec not configured",
		}, fmt.Errorf("unified exec not configured")
	}

	yield := time.Duration(args.YieldTimeMs) * time.Millisecond
	res, err := inv.UnifiedExec.WriteStdin(ctx, tools.WriteStdinSpec{
		SessionID:      args.SessionID,
		Chars:          args.Chars,
		YieldTime:      yield,
		MaxOutputBytes: args.MaxOutputBytes,
	})
	toolRes := tools.ToolResult{
		ID:        inv.Call.ID,
		Kind:      tools.ToolCommand,
		Status:    "completed",
		Output:    res.Output,
		Command:   "write_stdin",
		SessionID: res.SessionID,
	}
	if res.ExitCode != nil {
		toolRes.ExitCode = *res.ExitCode
		if toolRes.ExitCode != 0 {
			toolRes.Status = "error"
			if err == nil {
				err = fmt.Errorf("command exited with code %d", toolRes.ExitCode)
			}
			toolRes.Error = err.Error()
		}
	}
	if err != nil && toolRes.Error == "" {
		toolRes.Status = "error"
		toolRes.Error = err.Error()
	}
	return toolRes, err
}
