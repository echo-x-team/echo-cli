package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	path := args.Path
	if path == "" {
		if summary, err := tools.SummarizePatch(inv.Workdir, args.Patch); err == nil && summary.Primary != "" {
			path = summary.Primary
		}
	}

	diff := ""
	if strings.TrimSpace(args.Patch) != "" {
		if preview, _, err := tools.PreviewPatchDiff(context.Background(), inv.Workdir, args.Patch); err == nil {
			diff = preview
		}
		if diff == "" {
			diff = truncatePatchForEvent(args.Patch)
		}
	}
	return tools.ToolResult{
		ID:   inv.Call.ID,
		Kind: tools.ToolApplyPatch,
		Path: path,
		Diff: diff,
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

	workdir := inv.Workdir
	if workdir == "" {
		workdir = "."
	}

	summary, _ := tools.SummarizePatch(workdir, args.Patch)
	paths := summary.Paths
	path := args.Path
	if path == "" {
		path = summary.Primary
	}
	// Fallback for callers that still send a separate path field.
	if len(paths) == 0 && strings.TrimSpace(args.Path) != "" {
		paths = []string{strings.TrimSpace(args.Path)}
	}

	before := make(map[string][]byte, len(paths))
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(workdir, rel))
		if err == nil {
			before[rel] = data
			continue
		}
		if os.IsNotExist(err) {
			before[rel] = nil
			continue
		}
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolApplyPatch,
			Status: "error",
			Error:  err.Error(),
			Path:   path,
			Diff:   truncatePatchForEvent(args.Patch),
		}, err
	}

	err := inv.Runner.ApplyPatch(ctx, inv.Workdir, args.Patch)
	status := "completed"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}

	diff := ""
	if err == nil && len(paths) > 0 {
		after := make(map[string][]byte, len(paths))
		for _, rel := range paths {
			data, readErr := os.ReadFile(filepath.Join(workdir, rel))
			if readErr == nil {
				after[rel] = data
				continue
			}
			if os.IsNotExist(readErr) {
				after[rel] = nil
				continue
			}
			// If we can't read the post-image, fall back to the raw patch (truncated).
			diff = truncatePatchForEvent(args.Patch)
			break
		}
		if diff == "" {
			if d, dErr := tools.UnifiedDiffForFiles(ctx, paths, before, after); dErr == nil {
				diff = d
			} else {
				diff = truncatePatchForEvent(args.Patch)
			}
		}
	} else {
		diff = truncatePatchForEvent(args.Patch)
	}

	return tools.ToolResult{
		ID:     inv.Call.ID,
		Kind:   tools.ToolApplyPatch,
		Status: status,
		Error:  errMsg,
		Path:   path,
		Diff:   diff,
	}, err
}

func truncatePatchForEvent(patch string) string {
	const max = 12_000
	patch = strings.TrimSpace(patch)
	if patch == "" {
		return ""
	}
	if len(patch) <= max {
		return patch
	}
	return patch[:max] + "\n... (truncated)"
}
