package tools

import "context"

// DispatchRequest is an in-memory bus payload that carries a fully-built ToolCall
// plus the context for cancellation/deadlines.
type DispatchRequest struct {
	Ctx  context.Context
	Call ToolCall
}
