package repl

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

// EQRenderer listens to EQ events and renders them to a terminal writer as history cells.
// Compared to the older Transcript-based renderer, this keeps tool calls and other events
// as first-class, self-contained blocks.
type EQRenderer struct {
	mu sync.Mutex

	sessionID string
	activeSub string
	width     int

	renderers map[events.EventType]EventCellRenderer

	// 对齐 codex：屏幕由 Scrollback + InlineViewport 组成。
	scrollback *Scrollback
	viewport   *InlineViewport
}

type EQRendererOptions struct {
	SessionID string
	Width     int
	Writer    io.Writer
}

// EventCellRenderer handles one EQ EventType and may emit one or more cells.
type EventCellRenderer interface {
	Type() events.EventType
	Handle(r *EQRenderer, evt events.Event)
}

func NewEQRenderer(opts EQRendererOptions) *EQRenderer {
	width := opts.Width
	if width <= 0 {
		width = 80
	}
	r := &EQRenderer{
		sessionID:  opts.SessionID,
		width:      width,
		renderers:  map[events.EventType]EventCellRenderer{},
		scrollback: NewScrollback(ScrollbackOptions{Writer: opts.Writer, Width: width}),
		viewport:   NewInlineViewport(),
	}
	for _, rr := range defaultCellRenderers() {
		r.renderers[rr.Type()] = rr
	}
	return r
}

func (r *EQRenderer) RegisterRenderer(renderer EventCellRenderer) {
	if renderer == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.renderers[renderer.Type()] = renderer
}

func (r *EQRenderer) Handle(evt events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.sessionID != "" && evt.SessionID != r.sessionID {
		return
	}
	if rr := r.renderers[evt.Type]; rr != nil {
		rr.Handle(r, evt)
	}
}

// AppendAssistant appends a full assistant message as a cell (non-EQ usage).
func (r *EQRenderer) AppendAssistant(content string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ScrollbackAppend(newAssistantCell(content))
}

// AppendMessages appends historical messages as cells (non-EQ usage).
func (r *EQRenderer) AppendMessages(msgs []agent.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range msgs {
		switch m.Role {
		case agent.RoleUser:
			r.ScrollbackAppend(newUserCell(m.Content))
		case agent.RoleAssistant:
			r.ScrollbackAppend(newAssistantCell(m.Content))
		}
	}
}

func (r *EQRenderer) ScrollbackAppend(cell HistoryCell) {
	if r == nil || cell == nil || r.scrollback == nil {
		return
	}
	r.scrollback.SetWidth(r.width)
	r.scrollback.AppendCell(cell)
}

// --- Default cell renderers ---

func defaultCellRenderers() []EventCellRenderer {
	return []EventCellRenderer{
		submissionAcceptedRenderer{},
		agentOutputRenderer{},
		planUpdatedRenderer{},
		toolEventRenderer{},
		taskErrorRenderer{},
		// task.started / task.completed are currently no-op in human output.
	}
}

type submissionAcceptedRenderer struct{}

func (submissionAcceptedRenderer) Type() events.EventType { return events.EventSubmissionAccepted }

func (submissionAcceptedRenderer) Handle(r *EQRenderer, evt events.Event) {
	op, ok := evt.Payload.(events.Operation)
	if !ok || op.Kind != events.OperationUserInput || op.UserInput == nil {
		return
	}
	r.activeSub = evt.SubmissionID
	if r.viewport != nil && r.viewport.Active != nil {
		r.viewport.Active.Begin(evt.SubmissionID)
	}
	for _, item := range op.UserInput.Items {
		if item.Role != "user" {
			continue
		}
		r.ScrollbackAppend(newUserCell(item.Content))
	}
}

type agentOutputRenderer struct{}

func (agentOutputRenderer) Type() events.EventType { return events.EventAgentOutput }

func (agentOutputRenderer) Handle(r *EQRenderer, evt events.Event) {
	msg, ok := evt.Payload.(events.AgentOutput)
	if !ok {
		return
	}
	if r.activeSub != "" && evt.SubmissionID != r.activeSub {
		return
	}

	// 对齐 codex：delta 更新 active_cell；最终结果 flush 到 scrollback。
	if !msg.Final {
		if r.viewport != nil && r.viewport.Active != nil && msg.Content != "" {
			r.viewport.Active.AppendDelta(evt.SubmissionID, msg.Content)
		}
		return
	}

	var cell HistoryCell
	if r.viewport != nil && r.viewport.Active != nil {
		cell = r.viewport.Active.Finalize(evt.SubmissionID, msg.Content)
	} else {
		// Defensive fallback; should not happen.
		finalText := strings.TrimSpace(msg.Content)
		if finalText != "" {
			cell = newAssistantCell(finalText)
		}
	}
	if cell != nil {
		r.ScrollbackAppend(cell)
	}
	r.activeSub = ""
}

type planUpdatedRenderer struct{}

func (planUpdatedRenderer) Type() events.EventType { return events.EventPlanUpdated }

func (planUpdatedRenderer) Handle(r *EQRenderer, evt events.Event) {
	args, ok := evt.Payload.(tools.UpdatePlanArgs)
	if !ok {
		return
	}
	r.ScrollbackAppend(newPlanCell(args))
}

type toolEventRenderer struct{}

func (toolEventRenderer) Type() events.EventType { return events.EventToolEvent }

func (toolEventRenderer) Handle(r *EQRenderer, evt events.Event) {
	tev, ok := evt.Payload.(tools.ToolEvent)
	if !ok {
		return
	}
	// In REPL human output, render key tool lifecycle events as their own blocks.
	switch tev.Type {
	case "approval.requested", "approval.completed", "item.started", "item.completed":
		r.ScrollbackAppend(newToolEventCell(tev))
	}
}

type taskErrorRenderer struct{}

func (taskErrorRenderer) Type() events.EventType { return events.EventError }

func (taskErrorRenderer) Handle(r *EQRenderer, evt events.Event) {
	msg := strings.TrimSpace(fmt.Sprint(evt.Payload))
	if msg == "" {
		return
	}
	// Reuse assistant styling for errors to keep output compact.
	r.ScrollbackAppend(newAssistantCell("error: " + msg))
}
