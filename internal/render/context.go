package render

import (
	"echo-cli/internal/events"
	tuirender "echo-cli/internal/tui/render"
)

// Context holds shared state for all EQ event renderers.
// It is mutated by event-specific renderers to implement incremental rendering.
type Context struct {
	// SessionID, if set, filters events by session.
	SessionID string
	// ActiveSub is the submission currently being rendered (used to gate streaming).
	ActiveSub string
	// Transcript is the incremental transcript store/formatter.
	Transcript *tuirender.Transcript
	// EmitLines receives incremental delta lines produced by Transcript.
	// It can be nil if caller doesn't need per-line output.
	EmitLines func([]string)
}

// Emit forwards delta lines to EmitLines if provided.
func (c *Context) Emit(lines []string) {
	if c == nil || len(lines) == 0 {
		return
	}
	if c.EmitLines != nil {
		c.EmitLines(lines)
	}
}

// EventRenderer renders a single EQ EventType.
// Aligns with codex-rs: each protocol event has its own handler/renderer.
type EventRenderer interface {
	Type() events.EventType
	Handle(ctx *Context, evt events.Event)
}
