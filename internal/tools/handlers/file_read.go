package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"echo-cli/internal/tools"
)

type FileReadHandler struct{}

func (FileReadHandler) Name() string           { return "file_read" }
func (FileReadHandler) Kind() tools.ToolKind   { return tools.ToolFileRead }
func (FileReadHandler) SupportsParallel() bool { return true }
func (FileReadHandler) IsMutating(tools.Invocation) bool {
	return false
}

func (FileReadHandler) Describe(inv tools.Invocation) tools.ToolResult {
	args := struct {
		Path string `json:"path"`
	}{}
	_ = json.Unmarshal(inv.Call.Payload, &args)
	return tools.ToolResult{
		ID:   inv.Call.ID,
		Kind: tools.ToolFileRead,
		Path: args.Path,
	}
}

func (FileReadHandler) Handle(_ context.Context, inv tools.Invocation) (tools.ToolResult, error) {
	args := struct {
		Path string `json:"path"`
	}{}
	if err := json.Unmarshal(inv.Call.Payload, &args); err != nil || args.Path == "" {
		return tools.ToolResult{
			ID:     inv.Call.ID,
			Kind:   tools.ToolFileRead,
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
	return tools.ToolResult{
		ID:     inv.Call.ID,
		Kind:   tools.ToolFileRead,
		Status: status,
		Error:  errMsg,
		Output: string(data),
		Path:   args.Path,
	}, err
}
