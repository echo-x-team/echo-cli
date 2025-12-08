package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Span 表示一段文本及其样式。
type Span struct {
	Text  string
	Style lipgloss.Style
}

// Line 由多个 Span 组成，可选整体样式。
type Line struct {
	Spans []Span
	Style lipgloss.Style
}

// Buffer 收集渲染结果，按行存储。
type Buffer struct {
	Lines []Line
}

// WriteLine 追加单行。
func (b *Buffer) WriteLine(line Line) {
	if b == nil {
		return
	}
	b.Lines = append(b.Lines, line)
}

// WriteLines 追加多行。
func (b *Buffer) WriteLines(lines ...Line) {
	if b == nil {
		return
	}
	b.Lines = append(b.Lines, lines...)
}

// LinesToStrings 将样式化的行转换为字符串列表。
func LinesToStrings(lines []Line) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		segments := make([]string, 0, len(line.Spans))
		for _, sp := range line.Spans {
			segments = append(segments, sp.Style.Render(sp.Text))
		}
		text := strings.Join(segments, "")
		text = line.Style.Render(text)
		out = append(out, text)
	}
	return out
}
