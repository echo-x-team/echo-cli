package handlers

import "echo-cli/internal/tools"

// Default returns the built-in tool handlers.
func Default() []tools.Handler {
	return []tools.Handler{
		ExecCommandHandler{},
		WriteStdinHandler{},
		ApplyPatchHandler{},
		FileReadHandler{},
		FileSearchHandler{},
		PlanHandler{},
	}
}
