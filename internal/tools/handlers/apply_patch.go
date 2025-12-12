package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"echo-cli/internal/tools"
)

type ApplyPatchHandler struct{}

func (ApplyPatchHandler) Name() string           { return "apply_patch" }
func (ApplyPatchHandler) Kind() tools.ToolKind   { return tools.ToolApplyPatch }
func (ApplyPatchHandler) SupportsParallel() bool { return false }
func (ApplyPatchHandler) IsMutating(tools.Invocation) bool {
	return true
}

func (ApplyPatchHandler) Describe(inv tools.Invocation) tools.ToolResult {
	args := struct {
		Patch string `json:"patch"`
		Path  string `json:"path"`
	}{}
	_ = json.Unmarshal(inv.Call.Payload, &args)
	return tools.ToolResult{
		ID:   inv.Call.ID,
		Kind: tools.ToolApplyPatch,
		Path: args.Path,
		Diff: args.Patch,
	}
}

func (ApplyPatchHandler) Handle(ctx context.Context, inv tools.Invocation) (tools.ToolResult, error) {
	args := struct {
		Patch string `json:"patch"`
		Path  string `json:"path"`
	}{}
	if err := json.Unmarshal(inv.Call.Payload, &args); err != nil || args.Patch == "" {
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolApplyPatch,
			Status: "error",
			Error:  "invalid patch payload",
			Path:   args.Path,
			Diff:   args.Patch,
		}, fmt.Errorf("invalid patch payload: %w", err)
	}

	if inv.Runner == nil {
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolApplyPatch,
			Status: "error",
			Error:  "runner not configured",
			Path:   args.Path,
			Diff:   args.Patch,
		}, fmt.Errorf("runner not configured")
	}

	err := inv.Runner.ApplyPatch(ctx, inv.Workdir, args.Patch)
	status := "completed"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	return tools.ToolResult{
		ID:     inv.Call.ID,
		Kind:   tools.ToolApplyPatch,
		Status: status,
		Error:  errMsg,
		Path:   args.Path,
		Diff:   args.Patch,
	}, err
}
