package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"echo-cli/internal/tools"
)

type ExecCommandHandler struct{}

func (ExecCommandHandler) Name() string           { return "exec_command" }
func (ExecCommandHandler) Kind() tools.ToolKind   { return tools.ToolCommand }
func (ExecCommandHandler) SupportsParallel() bool { return false }
func (ExecCommandHandler) IsMutating(tools.Invocation) bool {
	return true
}

func (ExecCommandHandler) Describe(inv tools.Invocation) tools.ToolResult {
	args := struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}{}
	_ = json.Unmarshal(inv.Call.Payload, &args)
	cmd := strings.TrimSpace(args.Command)
	if cmd == "" {
		cmd = "(empty)"
	}
	return tools.ToolResult{
		ID:      inv.Call.ID,
		Kind:    tools.ToolCommand,
		Command: cmd,
	}
}

func (ExecCommandHandler) Handle(ctx context.Context, inv tools.Invocation) (tools.ToolResult, error) {
	args := struct {
		Command        string `json:"command"`
		Workdir        string `json:"workdir"`
		YieldTimeMs    int    `json:"yield_time_ms"`
		MaxOutputBytes int    `json:"max_output_bytes"`
	}{}
	if err := json.Unmarshal(inv.Call.Payload, &args); err != nil || strings.TrimSpace(args.Command) == "" {
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolCommand,
			Status: "error",
			Error:  "invalid exec_command payload",
		}, fmt.Errorf("invalid exec_command payload: %w", err)
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
	spec := tools.ExecCommandSpec{
		Command:        args.Command,
		Workdir:        chooseWorkdir(inv.Workdir, args.Workdir),
		BaseEnv:        os.Environ(),
		YieldTime:      yield,
		MaxOutputBytes: args.MaxOutputBytes,
	}
	res, err := inv.UnifiedExec.ExecCommand(ctx, spec)
	toolRes := tools.ToolResult{
		ID:        inv.Call.ID,
		Kind:      tools.ToolCommand,
		Status:    "completed",
		Output:    res.Output,
		Command:   args.Command,
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

func chooseWorkdir(invWorkdir, override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	return invWorkdir
}
