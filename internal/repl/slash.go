package repl

import "echo-cli/internal/tui/slash"

// ResolveSlashAction 让非交互入口也能复用 slash 解析逻辑。
func ResolveSlashAction(input string, opts slash.Options) slash.Action {
	state := slash.NewState(opts)
	return state.ResolveSubmit(input)
}
