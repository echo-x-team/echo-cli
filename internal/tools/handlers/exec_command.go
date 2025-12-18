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
			toolRes.Error = enrichCommandError(err, toolRes.ExitCode, toolRes.Output)
		}
	}
	if err != nil && toolRes.Error == "" {
		toolRes.Status = "error"
		toolRes.Error = enrichCommandError(err, toolRes.ExitCode, toolRes.Output)
	}
	return toolRes, err
}

func chooseWorkdir(invWorkdir, override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	return invWorkdir
}

func enrichCommandError(err error, exitCode int, output string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	out := strings.ToLower(output)
	if exitCode == 127 {
		// 常见：Node/Vite 项目未安装 node_modules，导致脚本里的 vite 找不到。
		if strings.Contains(out, "vite: not found") || strings.Contains(out, "vite: command not found") {
			return msg + "（可能未安装依赖：先在项目目录运行 `npm install`/`pnpm install`/`yarn` 再重试）"
		}
		// Generic guidance for command-not-found scenarios.
		if strings.Contains(out, "not found") || strings.Contains(out, "command not found") {
			return msg + "（命令未找到：请确认已安装依赖/工具并且 PATH 正确）"
		}
	}
	return msg
}
