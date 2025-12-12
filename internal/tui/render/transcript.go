package render

import (
	"strings"

	"echo-cli/internal/agent"
	"github.com/charmbracelet/lipgloss"
)

var (
	userPrefixStyle      = lipgloss.NewStyle().Faint(true).Bold(true)
	userIndentStyle      = lipgloss.NewStyle().Faint(true)
	assistantPrefixStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))
	assistantIndentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))
)

// RenderMessages 使用 ColumnRenderable 将消息列表渲染为行。
func RenderMessages(msgs []agent.Message, width int) []Line {
	col := NewColumn()
	for _, msg := range msgs {
		col.Push(messageRenderable{msg: msg})
	}
	buf := Buffer{}
	height := col.DesiredHeight(width)
	col.Render(Rect{Width: width, Height: height}, &buf)
	return buf.Lines
}

type messageRenderable struct {
	baseRenderable
	msg agent.Message
}

func (m messageRenderable) Render(area Rect, buf *Buffer) {
	content := strings.TrimRight(m.msg.Content, "\n")
	switch m.msg.Role {
	case agent.RoleUser:
		buf.WriteLines(renderUserLines(content, area.Width)...)
	case agent.RoleAssistant:
		buf.WriteLines(renderAssistantLines(content, area.Width)...)
	default:
		buf.WriteLines(StaticLines(wrapPlain(content, area.Width))...)
	}
}

func (m messageRenderable) DesiredHeight(width int) int {
	switch m.msg.Role {
	case agent.RoleUser:
		return len(renderUserLines(m.msg.Content, width))
	case agent.RoleAssistant:
		return len(renderAssistantLines(m.msg.Content, width))
	default:
		return len(wrapPlain(m.msg.Content, width))
	}
}

func (m messageRenderable) CursorPos(Rect) *CursorPos { return nil }

func renderUserLines(content string, width int) []Line {
	wrapWidth := width - 2
	if wrapWidth < 1 {
		wrapWidth = width
	}
	body := wrapLines(content, wrapWidth, lipgloss.Style{})
	prefixed := PrefixLines(body, Span{Text: "› ", Style: userPrefixStyle}, Span{Text: "  ", Style: userIndentStyle})
	lines := make([]Line, 0, len(prefixed)+2)
	lines = append(lines, Line{Spans: []Span{{Text: "", Style: userPrefixStyle}}})
	lines = append(lines, prefixed...)
	lines = append(lines, Line{Spans: []Span{{Text: "", Style: userPrefixStyle}}})
	return lines
}

func renderAssistantLines(content string, width int) []Line {
	wrapWidth := width - 2
	if wrapWidth < 1 {
		wrapWidth = width
	}
	body := wrapLines(content, wrapWidth, lipgloss.Style{})
	prefixed := PrefixLines(body, Span{Text: "• ", Style: assistantPrefixStyle}, Span{Text: "  ", Style: assistantIndentStyle})
	if len(prefixed) == 0 {
		prefixed = []Line{{Spans: []Span{{Text: "• ", Style: assistantPrefixStyle}}}}
	}
	return prefixed
}

func wrapPlain(content string, width int) []Line {
	return wrapLines(content, width, lipgloss.Style{})
}

func wrapLines(content string, width int, style lipgloss.Style) []Line {
	if width <= 0 {
		width = len(content)
	}
	lines := wrapText(content, width)
	out := make([]Line, 0, len(lines))
	for _, l := range lines {
		out = append(out, Line{Spans: []Span{{Text: l, Style: style}}})
	}
	return out
}

// Transcript 维护消息列表并输出增量渲染结果。
type Transcript struct {
	width      int
	messages   []agent.Message
	lastRender []string
}

// NewTranscript 创建 Transcript。
func NewTranscript(width int) *Transcript {
	if width <= 0 {
		width = 80
	}
	return &Transcript{width: width}
}

// SetWidth 更新渲染宽度。
func (t *Transcript) SetWidth(width int) {
	if width > 0 {
		t.width = width
		t.lastRender = nil
	}
}

// Messages returns a copy of current messages.
func (t *Transcript) Messages() []agent.Message {
	if t == nil {
		return nil
	}
	return append([]agent.Message{}, t.messages...)
}

// LoadMessages replaces transcript content with provided messages.
// Used by UIs to hydrate from persisted session state.
func (t *Transcript) LoadMessages(msgs []agent.Message) {
	if t == nil {
		return
	}
	t.messages = append([]agent.Message{}, msgs...)
	t.lastRender = nil
}

// Reset clears all messages and cached render state.
func (t *Transcript) Reset() {
	if t == nil {
		return
	}
	t.messages = nil
	t.lastRender = nil
}

// AppendUser 追加用户消息并返回增量行。
func (t *Transcript) AppendUser(content string) []string {
	t.messages = append(t.messages, agent.Message{Role: agent.RoleUser, Content: content})
	return t.renderDelta()
}

// AppendAssistantChunk 追加助手流式片段。
func (t *Transcript) AppendAssistantChunk(chunk string) []string {
	if len(t.messages) == 0 || t.messages[len(t.messages)-1].Role != agent.RoleAssistant {
		t.messages = append(t.messages, agent.Message{Role: agent.RoleAssistant})
	}
	t.messages[len(t.messages)-1].Content += chunk
	return t.renderDelta()
}

// FinalizeAssistant 完成助手输出。
func (t *Transcript) FinalizeAssistant(final string) []string {
	if len(t.messages) == 0 || t.messages[len(t.messages)-1].Role != agent.RoleAssistant {
		if final == "" {
			return nil
		}
		t.messages = append(t.messages, agent.Message{Role: agent.RoleAssistant, Content: final})
		return t.renderDelta()
	}
	if final != "" {
		t.messages[len(t.messages)-1].Content = final
	}
	return t.renderDelta()
}

func (t *Transcript) renderDelta() []string {
	lines := LinesToStrings(RenderMessages(t.messages, t.width))
	start := 0
	for start < len(lines) && start < len(t.lastRender) && t.lastRender[start] == lines[start] {
		start++
	}
	t.lastRender = lines
	if start >= len(lines) {
		return nil
	}
	return lines[start:]
}
