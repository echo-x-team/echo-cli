package tools

import (
	"context"
)

// Orchestrator 负责状态事件封装（全自动执行，无审批/无沙箱）。
type Orchestrator struct{}

func NewOrchestrator() *Orchestrator { return &Orchestrator{} }

func (o *Orchestrator) Run(ctx context.Context, inv Invocation, handler Handler, emit func(ToolEvent)) ToolResult {
	base := handler.Describe(inv)
	base.ID = inv.Call.ID
	base.Kind = handler.Kind()

	emit(ToolEvent{
		Type:   "item.started",
		Result: base,
	})

	result, err := handler.Handle(ctx, inv)
	result = normalizeResult(result, err, inv, handler)

	emit(ToolEvent{
		Type:   "item.completed",
		Result: result,
	})
	return result
}

func normalizeResult(result ToolResult, err error, inv Invocation, handler Handler) ToolResult {
	result.ID = inv.Call.ID
	result.Kind = handler.Kind()

	if err != nil && result.Error == "" {
		result.Error = err.Error()
	}
	if result.Status == "" {
		if result.Error != "" {
			result.Status = "error"
		} else {
			result.Status = "completed"
		}
	}
	return result
}
