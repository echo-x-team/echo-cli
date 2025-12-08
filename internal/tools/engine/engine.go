package engine

import (
	"context"

	"echo-cli/internal/policy"
	"echo-cli/internal/tools"
)

type Engine struct {
	runtime *tools.Runtime
}

func New(pol policy.Policy, runner tools.Runner, approver tools.Approver, workdir string) *Engine {
	return &Engine{
		runtime: tools.NewRuntime(pol, runner, approver, workdir),
	}
}

func (e *Engine) Run(ctx context.Context, req tools.ToolRequest, emit func(tools.ToolEvent)) {
	call := req.ToCall()
	if call.Name == "" {
		emit(tools.ToolEvent{
			Type: "item.completed",
			Result: tools.ToolResult{
				ID:     call.ID,
				Status: "error",
				Error:  "unknown tool",
			},
		})
		return
	}
	_, _ = e.runtime.Dispatch(ctx, call, emit)
}
