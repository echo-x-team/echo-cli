package handlers

import (
	"context"
	"strings"

	"echo-cli/internal/search"
	"echo-cli/internal/tools"
)

type FileSearchHandler struct{}

func (FileSearchHandler) Name() string           { return "file_search" }
func (FileSearchHandler) Kind() tools.ToolKind   { return tools.ToolSearch }
func (FileSearchHandler) SupportsParallel() bool { return true }
func (FileSearchHandler) IsMutating(tools.Invocation) bool {
	return false
}

func (FileSearchHandler) Describe(inv tools.Invocation) tools.ToolResult {
	return tools.ToolResult{
		ID:   inv.Call.ID,
		Kind: tools.ToolSearch,
	}
}

func (FileSearchHandler) Handle(_ context.Context, inv tools.Invocation) (tools.ToolResult, error) {
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
	return tools.ToolResult{
		ID:     inv.Call.ID,
		Kind:   tools.ToolSearch,
		Status: status,
		Error:  errMsg,
		Output: strings.Join(paths, "\n"),
	}, err
}
