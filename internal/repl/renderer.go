package repl

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/tools"
	tuirender "echo-cli/internal/tui/render"
)

// EQRenderer listens to EQ events and renders them to a terminal writer as history cells.
// Compared to the older Transcript-based renderer, this keeps tool calls and other events
// as first-class, self-contained blocks.
type EQRenderer struct {
	mu sync.Mutex

	sessionID string
	activeSub string
	width     int
	w         io.Writer

	renderers map[events.EventType]EventCellRenderer

	// Accumulate agent.output deltas so we can render a coherent final assistant cell
	// without trying to do in-place terminal updates.
	pendingAssistant map[string]*strings.Builder // submission_id -> text builder
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
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	width := opts.Width
	if width <= 0 {
		width = 80
	}
	r := &EQRenderer{
		sessionID:        opts.SessionID,
		width:            width,
		w:                w,
		renderers:        map[events.EventType]EventCellRenderer{},
		pendingAssistant: map[string]*strings.Builder{},
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
	r.emitCell(newAssistantCell(content))
}

// AppendMessages appends historical messages as cells (non-EQ usage).
func (r *EQRenderer) AppendMessages(msgs []agent.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range msgs {
		switch m.Role {
		case agent.RoleUser:
			r.emitCell(newUserCell(m.Content))
		case agent.RoleAssistant:
			r.emitCell(newAssistantCell(m.Content))
		}
	}
}

func (r *EQRenderer) emitCell(cell HistoryCell) {
	if cell == nil {
		return
	}
	lines := cell.Render(r.width)
	for _, s := range tuirender.LinesToStrings(lines) {
		fmt.Fprintln(r.w, s)
	}
}

func (r *EQRenderer) assistantBuf(subID string) *strings.Builder {
	if subID == "" {
		subID = "_"
	}
	b := r.pendingAssistant[subID]
	if b == nil {
		b = &strings.Builder{}
		r.pendingAssistant[subID] = b
	}
	return b
}

func (r *EQRenderer) clearAssistantBuf(subID string) {
	delete(r.pendingAssistant, subID)
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
	for _, item := range op.UserInput.Items {
		if item.Role != "user" {
			continue
		}
		r.emitCell(newUserCell(item.Content))
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

	// Buffer deltas; only render a single assistant cell when Final=true.
	if !msg.Final {
		if msg.Content != "" {
			r.assistantBuf(evt.SubmissionID).WriteString(msg.Content)
		}
		return
	}

	finalText := msg.Content
	if strings.TrimSpace(finalText) == "" {
		finalText = r.assistantBuf(evt.SubmissionID).String()
	}
	r.clearAssistantBuf(evt.SubmissionID)
	if strings.TrimSpace(finalText) != "" {
		r.emitCell(newAssistantCell(finalText))
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
	r.emitCell(newPlanCell(args))
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
		r.emitCell(newToolEventCell(tev))
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
	r.emitCell(newAssistantCell("error: " + msg))
}
