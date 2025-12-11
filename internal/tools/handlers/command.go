package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"echo-cli/internal/tools"
)

type CommandHandler struct{}

func (CommandHandler) Name() string           { return "command" }
func (CommandHandler) Kind() tools.ToolKind   { return tools.ToolCommand }
func (CommandHandler) SupportsParallel() bool { return false }
func (CommandHandler) IsMutating(tools.Invocation) bool {
	return true
}

func (CommandHandler) Describe(inv tools.Invocation) tools.ToolResult {
	args := struct {
		Command string `json:"command"`
	}{}
	_ = json.Unmarshal(inv.Call.Payload, &args)
	return tools.ToolResult{
		ID:      inv.Call.ID,
		Kind:    tools.ToolCommand,
		Command: args.Command,
	}
}

func (CommandHandler) Handle(ctx context.Context, inv tools.Invocation) (tools.ToolResult, error) {
	args := struct {
		Command string `json:"command"`
	}{}
	if err := json.Unmarshal(inv.Call.Payload, &args); err != nil || args.Command == "" {
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolCommand,
			Error:  "invalid command payload",
			Status: "error",
		}, fmt.Errorf("invalid command payload: %w", err)
	}

	if inv.Runner == nil {
		return tools.ToolResult{
			ID:      inv.Call.ID,
			Kind:    tools.ToolCommand,
			Status:  "error",
			Error:   "runner not configured",
			Command: args.Command,
		}, fmt.Errorf("runner not configured")
	}

	out, code, err := inv.Runner.RunCommand(ctx, inv.Workdir, args.Command)
	status := "completed"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	return tools.ToolResult{
		ID:       inv.Call.ID,
		Kind:     tools.ToolCommand,
		Status:   status,
		Output:   out,
		Error:    errMsg,
		ExitCode: code,
		Command:  args.Command,
	}, err
}
