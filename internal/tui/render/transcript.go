package render

import (
	"strings"

	"echo-cli/internal/agent"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	userPrefixStyle      = lipgloss.NewStyle().Faint(true).Bold(true)
	userIndentStyle      = lipgloss.NewStyle().Faint(true)
	assistantPrefixStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))
	assistantIndentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))
	toolStyle            = lipgloss.NewStyle().Faint(true)
	diffAddStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#16a34a"))
	diffDelStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#dc2626"))
	diffHunkStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Faint(true)
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
	case "tool":
		buf.WriteLines(renderToolLines(content, area.Width)...)
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
	case "tool":
		return len(renderToolLines(m.msg.Content, width))
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

func renderToolLines(content string, width int) []Line {
	// Tool/event blocks are already "cell shaped" (contain their own bullets/indent).
	// Keep them visually distinct from user/assistant messages, but preserve whitespace
	// (tool outputs/diffs are often preformatted).
	wrapWidth := width - 2
	if wrapWidth < 1 {
		wrapWidth = width
	}
	return wrapToolBlock(content, wrapWidth)
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

func wrapToolBlock(content string, width int) []Line {
	if width <= 0 {
		width = len(content)
	}
	rawLines := strings.Split(content, "\n")

	inDiff := false
	out := []Line{}
	for _, raw := range rawLines {
		isDiffHeader := strings.Contains(raw, "└ diff:")
		lineInDiff := inDiff && !isDiffHeader

		for _, l := range wrapLinePreserveSpaces(raw, width) {
			out = append(out, Line{Spans: toolLineSpans(l, lineInDiff)})
		}
		if isDiffHeader {
			inDiff = true
		}
	}
	if len(out) == 0 {
		return []Line{{}}
	}
	return out
}

func toolLineSpans(line string, inDiff bool) []Span {
	if strings.TrimSpace(line) == "" {
		return []Span{{Text: line, Style: toolStyle}}
	}

	if !inDiff {
		return []Span{{Text: line, Style: toolStyle}}
	}

	// Keep cell indentation dim (our tool blocks indent payload lines by 4 spaces),
	// but preserve the actual diff marker (e.g. leading ' ' context lines).
	indent := ""
	rest := line
	if strings.HasPrefix(line, "    ") {
		indent = "    "
		rest = line[4:]
	}

	style := toolStyle
	switch {
	case strings.HasPrefix(rest, "+") && !strings.HasPrefix(rest, "+++"):
		style = diffAddStyle
	case strings.HasPrefix(rest, "-") && !strings.HasPrefix(rest, "---"):
		style = diffDelStyle
	case strings.HasPrefix(rest, "@@"):
		style = diffHunkStyle
	case strings.HasPrefix(rest, "diff ") || strings.HasPrefix(rest, "*** ") || strings.HasPrefix(rest, "+++ ") || strings.HasPrefix(rest, "--- "):
		style = toolStyle
	}

	if indent == "" {
		return []Span{{Text: rest, Style: style}}
	}
	return []Span{
		{Text: indent, Style: toolStyle},
		{Text: rest, Style: style},
	}
}

func wrapLinePreserveSpaces(line string, width int) []string {
	if width <= 0 || runewidth.StringWidth(line) <= width {
		return []string{line}
	}
	out := []string{}
	current := []rune{}
	w := 0
	for _, r := range line {
		rw := runewidth.RuneWidth(r)
		if w+rw > width && len(current) > 0 {
			out = append(out, string(current))
			current = current[:0]
			w = 0
		}
		current = append(current, r)
		w += rw
	}
	if len(current) > 0 {
		out = append(out, string(current))
	}
	if len(out) == 0 {
		return []string{line}
	}
	return out
}

// Transcript 维护消息列表并输出增量渲染结果。
type Transcript struct {
	width int
	// history holds persisted conversation messages (user/assistant).
	history []agent.Message
	// view holds render entries for the transcript, including non-persisted blocks
	// such as tool.event cells.
	view       []agent.Message
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
	return append([]agent.Message{}, t.history...)
}

// LoadMessages replaces transcript content with provided messages.
// Used by UIs to hydrate from persisted session state.
func (t *Transcript) LoadMessages(msgs []agent.Message) {
	if t == nil {
		return
	}
	t.history = append([]agent.Message{}, msgs...)
	t.view = append([]agent.Message{}, msgs...)
	t.lastRender = nil
}

// Reset clears all messages and cached render state.
func (t *Transcript) Reset() {
	if t == nil {
		return
	}
	t.history = nil
	t.view = nil
	t.lastRender = nil
}

// AppendUser 追加用户消息并返回增量行。
func (t *Transcript) AppendUser(content string) []string {
	msg := agent.Message{Role: agent.RoleUser, Content: content}
	t.history = append(t.history, msg)
	t.view = append(t.view, msg)
	return t.renderDelta()
}

// AppendAssistantChunk 追加助手流式片段。
func (t *Transcript) AppendAssistantChunk(chunk string) []string {
	if len(t.history) == 0 || t.history[len(t.history)-1].Role != agent.RoleAssistant {
		t.history = append(t.history, agent.Message{Role: agent.RoleAssistant})
	}
	t.history[len(t.history)-1].Content += chunk

	// Mirror into view; ensure last view entry is assistant.
	if len(t.view) == 0 || t.view[len(t.view)-1].Role != agent.RoleAssistant {
		t.view = append(t.view, agent.Message{Role: agent.RoleAssistant})
	}
	t.view[len(t.view)-1].Content += chunk
	return t.renderDelta()
}

// FinalizeAssistant 完成助手输出。
func (t *Transcript) FinalizeAssistant(final string) []string {
	if len(t.history) == 0 || t.history[len(t.history)-1].Role != agent.RoleAssistant {
		if final == "" {
			return nil
		}
		msg := agent.Message{Role: agent.RoleAssistant, Content: final}
		t.history = append(t.history, msg)
		t.view = append(t.view, msg)
		return t.renderDelta()
	}
	if final != "" {
		t.history[len(t.history)-1].Content = final
		// Mirror to view if last view entry is assistant, else append a new one.
		if len(t.view) == 0 || t.view[len(t.view)-1].Role != agent.RoleAssistant {
			t.view = append(t.view, agent.Message{Role: agent.RoleAssistant, Content: final})
		} else {
			t.view[len(t.view)-1].Content = final
		}
	}
	return t.renderDelta()
}

// AppendToolBlock appends a non-persisted tool/event block to the transcript view.
// The block is rendered with role="tool" and will not be returned by Messages().
func (t *Transcript) AppendToolBlock(content string) []string {
	if t == nil {
		return nil
	}
	content = strings.TrimRight(content, "\n")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	t.view = append(t.view, agent.Message{Role: "tool", Content: content})
	return t.renderDelta()
}

// RenderViewLines renders the full transcript view (including tool blocks).
func (t *Transcript) RenderViewLines(width int) []Line {
	if t == nil {
		return nil
	}
	if width <= 0 {
		width = t.width
	}
	return RenderMessages(t.view, width)
}

func (t *Transcript) renderDelta() []string {
	lines := LinesToStrings(RenderMessages(t.view, t.width))
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
