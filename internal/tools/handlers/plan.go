package handlers

import (
	"context"
	"fmt"
	"strings"

	"echo-cli/internal/tools"
)

type PlanHandler struct{}

func (PlanHandler) Name() string           { return "update_plan" }
func (PlanHandler) Kind() tools.ToolKind   { return tools.ToolPlanUpdate }
func (PlanHandler) SupportsParallel() bool { return true }
func (PlanHandler) IsMutating(tools.Invocation) bool {
	return false
}

func (PlanHandler) Describe(inv tools.Invocation) tools.ToolResult {
	return tools.ToolResult{
		ID:   inv.Call.ID,
		Kind: tools.ToolPlanUpdate,
	}
}

func (PlanHandler) Handle(_ context.Context, inv tools.Invocation) (tools.ToolResult, error) {
	if len(inv.Call.Payload) == 0 {
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolPlanUpdate,
			Status: "error",
			Error:  "missing plan payload",
		}, fmt.Errorf("missing plan payload")
	}
	args, err := tools.DecodePlanArgs(inv.Call.Payload)
	if err != nil {
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolPlanUpdate,
			Status: "error",
			Error:  err.Error(),
		}, err
	}

	return tools.ToolResult{
		ID:          inv.Call.ID,
		Kind:        tools.ToolPlanUpdate,
		Status:      "completed",
		Output:      "Plan updated",
		Plan:        args.Plan,
		Explanation: strings.TrimSpace(args.Explanation),
	}, nil
}
