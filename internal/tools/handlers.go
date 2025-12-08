package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"echo-cli/internal/search"
)

type CommandHandler struct{}

type ApplyPatchHandler struct{}

type FileReadHandler struct{}

type FileSearchHandler struct{}

type PlanHandler struct{}

func (CommandHandler) Name() string           { return "command" }
func (CommandHandler) Kind() ToolKind         { return ToolCommand }
func (CommandHandler) SupportsParallel() bool { return false }
func (CommandHandler) IsMutating(Invocation) bool {
	return true
}

func (CommandHandler) Describe(inv Invocation) ToolResult {
	args := struct {
		Command string `json:"command"`
	}{}
	_ = json.Unmarshal(inv.Call.Payload, &args)
	return ToolResult{
		ID:      inv.Call.ID,
		Kind:    ToolCommand,
		Command: args.Command,
	}
}

func (CommandHandler) Handle(ctx context.Context, inv Invocation) (ToolResult, error) {
	args := struct {
		Command string `json:"command"`
	}{}
	if err := json.Unmarshal(inv.Call.Payload, &args); err != nil || args.Command == "" {
		return ToolResult{
			ID:     inv.Call.ID,
			Kind:   ToolCommand,
			Error:  "invalid command payload",
			Status: "error",
		}, fmt.Errorf("invalid command payload: %w", err)
	}

	if inv.Runner == nil {
		return ToolResult{
			ID:      inv.Call.ID,
			Kind:    ToolCommand,
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
	return ToolResult{
		ID:       inv.Call.ID,
		Kind:     ToolCommand,
		Status:   status,
		Output:   out,
		Error:    errMsg,
		ExitCode: code,
		Command:  args.Command,
	}, err
}

func (ApplyPatchHandler) Name() string           { return "apply_patch" }
func (ApplyPatchHandler) Kind() ToolKind         { return ToolApplyPatch }
func (ApplyPatchHandler) SupportsParallel() bool { return false }
func (ApplyPatchHandler) IsMutating(Invocation) bool {
	return true
}

func (ApplyPatchHandler) Describe(inv Invocation) ToolResult {
	args := struct {
		Patch string `json:"patch"`
		Path  string `json:"path"`
	}{}
	_ = json.Unmarshal(inv.Call.Payload, &args)
	return ToolResult{
		ID:   inv.Call.ID,
		Kind: ToolApplyPatch,
		Path: args.Path,
	}
}

func (ApplyPatchHandler) Handle(ctx context.Context, inv Invocation) (ToolResult, error) {
	args := struct {
		Patch string `json:"patch"`
		Path  string `json:"path"`
	}{}
	if err := json.Unmarshal(inv.Call.Payload, &args); err != nil || args.Patch == "" {
		return ToolResult{
			ID:     inv.Call.ID,
			Kind:   ToolApplyPatch,
			Status: "error",
			Error:  "invalid patch payload",
			Path:   args.Path,
		}, fmt.Errorf("invalid patch payload: %w", err)
	}

	if inv.Runner == nil {
		return ToolResult{
			ID:     inv.Call.ID,
			Kind:   ToolApplyPatch,
			Status: "error",
			Error:  "runner not configured",
			Path:   args.Path,
		}, fmt.Errorf("runner not configured")
	}

	err := inv.Runner.ApplyPatch(ctx, inv.Workdir, args.Patch)
	status := "completed"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	return ToolResult{
		ID:     inv.Call.ID,
		Kind:   ToolApplyPatch,
		Status: status,
		Error:  errMsg,
		Path:   args.Path,
	}, err
}

func (FileReadHandler) Name() string           { return "file_read" }
func (FileReadHandler) Kind() ToolKind         { return ToolFileRead }
func (FileReadHandler) SupportsParallel() bool { return true }
func (FileReadHandler) IsMutating(Invocation) bool {
	return false
}

func (FileReadHandler) Describe(inv Invocation) ToolResult {
	args := struct {
		Path string `json:"path"`
	}{}
	_ = json.Unmarshal(inv.Call.Payload, &args)
	return ToolResult{
		ID:   inv.Call.ID,
		Kind: ToolFileRead,
		Path: args.Path,
	}
}

func (FileReadHandler) Handle(_ context.Context, inv Invocation) (ToolResult, error) {
	args := struct {
		Path string `json:"path"`
	}{}
	if err := json.Unmarshal(inv.Call.Payload, &args); err != nil || args.Path == "" {
		return ToolResult{
			ID:     inv.Call.ID,
			Kind:   ToolFileRead,
			Status: "error",
			Error:  "invalid file_read payload",
		}, fmt.Errorf("invalid file_read payload: %w", err)
	}
	target := args.Path
	if !filepath.IsAbs(target) && inv.Workdir != "" {
		target = filepath.Join(inv.Workdir, target)
	}
	data, err := os.ReadFile(target)
	status := "completed"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	return ToolResult{
		ID:     inv.Call.ID,
		Kind:   ToolFileRead,
		Status: status,
		Error:  errMsg,
		Output: string(data),
		Path:   args.Path,
	}, err
}

func (FileSearchHandler) Name() string           { return "file_search" }
func (FileSearchHandler) Kind() ToolKind         { return ToolSearch }
func (FileSearchHandler) SupportsParallel() bool { return true }
func (FileSearchHandler) IsMutating(Invocation) bool {
	return false
}

func (FileSearchHandler) Describe(inv Invocation) ToolResult {
	return ToolResult{
		ID:   inv.Call.ID,
		Kind: ToolSearch,
	}
}

func (FileSearchHandler) Handle(_ context.Context, inv Invocation) (ToolResult, error) {
	root := inv.Workdir
	if root == "" {
		root = "."
	}
	paths, err := search.FindFiles(root, 200)
	status := "completed"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	return ToolResult{
		ID:     inv.Call.ID,
		Kind:   ToolSearch,
		Status: status,
		Error:  errMsg,
		Output: strings.Join(paths, "\n"),
	}, err
}

func (PlanHandler) Name() string           { return "update_plan" }
func (PlanHandler) Kind() ToolKind         { return ToolPlanUpdate }
func (PlanHandler) SupportsParallel() bool { return true }
func (PlanHandler) IsMutating(Invocation) bool {
	return false
}

func (PlanHandler) Describe(inv Invocation) ToolResult {
	return ToolResult{
		ID:   inv.Call.ID,
		Kind: ToolPlanUpdate,
	}
}

func (PlanHandler) Handle(_ context.Context, inv Invocation) (ToolResult, error) {
	if len(inv.Call.Payload) == 0 {
		return ToolResult{
			ID:     inv.Call.ID,
			Kind:   ToolPlanUpdate,
			Status: "error",
			Error:  "missing plan payload",
		}, fmt.Errorf("missing plan payload")
	}
	args, err := decodePlanArgs(inv.Call.Payload)
	if err != nil {
		return ToolResult{
			ID:     inv.Call.ID,
			Kind:   ToolPlanUpdate,
			Status: "error",
			Error:  err.Error(),
		}, err
	}

	return ToolResult{
		ID:          inv.Call.ID,
		Kind:        ToolPlanUpdate,
		Status:      "completed",
		Output:      "Plan updated",
		Plan:        args.Plan,
		Explanation: strings.TrimSpace(args.Explanation),
	}, nil
}
