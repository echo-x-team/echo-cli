package repl

import (
	"fmt"
	"io"
	"os"
	"sync"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/render"
	tuirender "echo-cli/internal/tui/render"
)

// EQRenderer 监听 EQ 事件并使用事件级渲染器增量输出。
// 每个 EQ EventType 对应一个独立渲染器（见 internal/render）。
type EQRenderer struct {
	mu        sync.Mutex
	ctx       render.Context
	renderers map[events.EventType]render.EventRenderer
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
	emit := func(lines []string) {
		for _, line := range lines {
			fmt.Fprintln(w, line)
		}
	}
	return &EQRenderer{
		ctx: render.Context{
			SessionID:  opts.SessionID,
			Transcript: tuirender.NewTranscript(width),
			EmitLines:  emit,
		},
		renderers: render.DefaultRenderers(),
	}
}

// RegisterRenderer 允许上层注入或覆盖某个事件渲染器。
func (r *EQRenderer) RegisterRenderer(renderer render.EventRenderer) {
	if renderer == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.renderers == nil {
		r.renderers = map[events.EventType]render.EventRenderer{}
	}
	r.renderers[renderer.Type()] = renderer
}

// Handle 处理单条 EQ 事件并输出增量行。
func (r *EQRenderer) Handle(evt events.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ctx.SessionID != "" && evt.SessionID != r.ctx.SessionID {
		return
	}
	if renderer := r.renderers[evt.Type]; renderer != nil {
		renderer.Handle(&r.ctx, evt)
	}
}

// AppendAssistant 用于直接追加完整助手消息（非 EQ 场景）。
func (r *EQRenderer) AppendAssistant(content string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx.Emit(r.ctx.Transcript.FinalizeAssistant(content))
}

// AppendMessages 允许批量追加消息并输出增量。
func (r *EQRenderer) AppendMessages(msgs []agent.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range msgs {
		switch m.Role {
		case agent.RoleUser:
			r.ctx.Emit(r.ctx.Transcript.AppendUser(m.Content))
		case agent.RoleAssistant:
			r.ctx.Emit(r.ctx.Transcript.FinalizeAssistant(m.Content))
		}
	}
}
