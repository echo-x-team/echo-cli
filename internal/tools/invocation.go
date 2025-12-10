package tools

import (
	"context"

	"echo-cli/internal/policy"
)

// Runner 提供最小化的执行接口，避免与 sandbox 包产生循环依赖。
type Runner interface {
	RunCommand(ctx context.Context, workdir string, command string) (string, int, error)
	ApplyPatch(ctx context.Context, workdir string, diff string) error
	WithMode(mode string) Runner
}

// Invocation 提供 handler 执行所需的上下文。
type Invocation struct {
	Call    ToolCall
	Workdir string
	Policy  policy.Policy
	Runner  Runner
}
