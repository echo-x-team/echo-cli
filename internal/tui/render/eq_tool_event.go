package render

import (
	"strings"

	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

// toolEventRenderer renders tool.event into the transcript as non-persisted blocks.
// This aligns with the HistoryCell approach: tool calls are first-class render blocks,
// but should not pollute persisted conversation history.
type toolEventRenderer struct{}

func (toolEventRenderer) Type() events.EventType { return events.EventToolEvent }

func (toolEventRenderer) Handle(ctx *Context, evt events.Event) {
	if ctx == nil || ctx.Transcript == nil {
		return
	}
	toolEv, ok := evt.Payload.(tools.ToolEvent)
	if !ok {
		return
	}
	switch toolEv.Type {
	case "item.started", "item.updated", "item.completed":
	default:
		return
	}
	block := FormatToolEventBlock(toolEv)
	if strings.TrimSpace(block) == "" {
		return
	}
	ctx.Emit(ctx.Transcript.AppendToolBlock(block))
}
