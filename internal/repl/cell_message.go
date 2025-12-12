package repl

import (
	"echo-cli/internal/agent"
	tuirender "echo-cli/internal/tui/render"
)

type messageCell struct {
	msg agent.Message
}

func (c messageCell) ID() string { return "" }

func newUserCell(text string) HistoryCell {
	return messageCell{msg: agent.Message{Role: agent.RoleUser, Content: text}}
}

func newAssistantCell(text string) HistoryCell {
	return messageCell{msg: agent.Message{Role: agent.RoleAssistant, Content: text}}
}

func (c messageCell) Render(width int) []tuirender.Line {
	return tuirender.RenderMessages([]agent.Message{c.msg}, width)
}
