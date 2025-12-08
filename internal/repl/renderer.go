package repl

import (
	"fmt"
	"io"
	"os"
	"sync"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/tui/render"
)

// EQRenderer 监听 EQ 事件并使用 TUI 渲染模块增量输出。
type EQRenderer struct {
	mu         sync.Mutex
	sessionID  string
	activeSub  string
	transcript *render.Transcript
	writer     io.Writer
}

// EQRendererOptions 配置增量渲染行为。
type EQRendererOptions struct {
	SessionID string
	Width     int
	Writer    io.Writer
}

// NewEQRenderer 创建 EQRenderer。
func NewEQRenderer(opts EQRendererOptions) *EQRenderer {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	width := opts.Width
	if width <= 0 {
		width = 80
	}
	return &EQRenderer{
		sessionID:  opts.SessionID,
		transcript: render.NewTranscript(width),
		writer:     w,
	}
}

// Handle 处理单条 EQ 事件并输出增量行。
func (r *EQRenderer) Handle(evt events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessionID != "" && evt.SessionID != r.sessionID {
		return
	}
	switch evt.Type {
	case events.EventSubmissionAccepted:
		r.handleSubmissionAccepted(evt)
	case events.EventAgentOutput:
		r.handleAgentOutput(evt)
	case events.EventTaskCompleted, events.EventError:
		if r.activeSub != "" && evt.SubmissionID == r.activeSub {
			r.activeSub = ""
		}
	}
}

func (r *EQRenderer) handleSubmissionAccepted(evt events.Event) {
	op, ok := evt.Payload.(events.Operation)
	if !ok {
		return
	}
	if op.UserInput == nil {
		return
	}
	r.activeSub = evt.SubmissionID
	for _, msg := range op.UserInput.Items {
		if msg.Role != "user" {
			continue
		}
		r.writeLines(r.transcript.AppendUser(msg.Content))
	}
}

func (r *EQRenderer) handleAgentOutput(evt events.Event) {
	if r.activeSub != "" && evt.SubmissionID != r.activeSub {
		return
	}
	msg, ok := evt.Payload.(events.AgentOutput)
	if !ok {
		return
	}
	if msg.Final {
		final := msg.Content
		if final == "" {
			final = ""
		}
		r.writeLines(r.transcript.FinalizeAssistant(final))
		r.activeSub = ""
		return
	}
	if msg.Content != "" {
		r.writeLines(r.transcript.AppendAssistantChunk(msg.Content))
	}
}

func (r *EQRenderer) writeLines(lines []string) {
	for _, line := range lines {
		fmt.Fprintln(r.writer, line)
	}
}

// AppendAssistant 用于直接追加完整助手消息（非 EQ 场景）。
func (r *EQRenderer) AppendAssistant(content string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writeLines(r.transcript.FinalizeAssistant(content))
}

// AppendMessages 允许批量追加消息并输出增量。
func (r *EQRenderer) AppendMessages(msgs []agent.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range msgs {
		switch m.Role {
		case agent.RoleUser:
			r.writeLines(r.transcript.AppendUser(m.Content))
		case agent.RoleAssistant:
			r.writeLines(r.transcript.FinalizeAssistant(m.Content))
		}
	}
}
