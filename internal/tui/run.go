package tui

import (
	"errors"

	"echo-cli/internal/agent"

	tea "github.com/charmbracelet/bubbletea"
)

// Result 返回 TUI 运行后的必要信息。
type Result struct {
	History      []agent.Message
	SessionID    string
	UpdateAction string
}

// Run 封装 Bubble Tea 入口，返回最终的 UI 结果。
func Run(opts Options) (Result, error) {
	programOptions := []tea.ProgramOption{}
	if !opts.CopyableOutput {
		programOptions = append(programOptions, tea.WithAltScreen())
	}
	program := tea.NewProgram(New(opts), programOptions...)
	m, err := program.Run()
	if err != nil {
		return Result{}, err
	}
	tuiModel, ok := m.(*Model)
	if !ok {
		return Result{}, errors.New("unexpected tui model")
	}
	return Result{
		History:      tuiModel.History(),
		SessionID:    tuiModel.SessionID(),
		UpdateAction: tuiModel.UpdateAction(),
	}, nil
}
