package render

import "echo-cli/internal/events"

// DefaultRenderers returns the built-in per-event renderers.
func DefaultRenderers() map[events.EventType]EventRenderer {
	renderers := []EventRenderer{
		submissionAcceptedRenderer{},
		taskStartedRenderer{},
		agentOutputRenderer{},
		toolEventRenderer{},
		taskSummaryRenderer{},
		taskTerminalRenderer{typ: events.EventTaskCompleted},
		taskTerminalRenderer{typ: events.EventError},
		planUpdatedRenderer{},
	}
	out := make(map[events.EventType]EventRenderer, len(renderers))
	for _, r := range renderers {
		out[r.Type()] = r
	}
	return out
}
