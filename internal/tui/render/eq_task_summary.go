package render

import (
	"strings"

	"echo-cli/internal/events"
)

type taskSummaryRenderer struct{}

func (taskSummaryRenderer) Type() events.EventType { return events.EventTaskSummary }

func (taskSummaryRenderer) Handle(ctx *Context, evt events.Event) {
	if ctx == nil || ctx.Transcript == nil {
		return
	}
	summary, ok := evt.Payload.(events.TaskSummary)
	if !ok {
		return
	}
	text := strings.TrimSpace(summary.Text)
	if text == "" {
		return
	}
	ctx.Emit(ctx.Transcript.AppendToolBlock(text))
}
