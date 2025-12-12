package render

import "echo-cli/internal/events"

type taskStartedRenderer struct{}

func (taskStartedRenderer) Type() events.EventType { return events.EventTaskStarted }

func (taskStartedRenderer) Handle(_ *Context, _ events.Event) {
	// No-op for transcript rendering.
	// Active submission is set on submission.accepted to mirror codex behaviour.
}
