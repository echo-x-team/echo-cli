package render

import "echo-cli/internal/events"

// taskTerminalRenderer clears ActiveSub when a task ends or errors.
type taskTerminalRenderer struct {
	typ events.EventType
}

func (r taskTerminalRenderer) Type() events.EventType { return r.typ }

func (r taskTerminalRenderer) Handle(ctx *Context, evt events.Event) {
	if ctx == nil {
		return
	}
	if ctx.ActiveSub != "" && evt.SubmissionID == ctx.ActiveSub {
		ctx.ActiveSub = ""
	}
}
