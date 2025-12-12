package repl

import (
	"strings"

	tuirender "echo-cli/internal/tui/render"
	"github.com/charmbracelet/lipgloss"
)

// BottomPane 对齐 codex 的“bottom_pane”：输入/状态/弹窗等固定在屏幕底部的区域。
//
// internal/repl 目前主要用于 exec/human 的输出编排，因此这里先保留最小状态，
// 给后续把“pending、审批提示、输入提示”等沉到一个可重绘区域留下落点。
type BottomPane struct {
	StatusLine string
}

func (p *BottomPane) Clear() {
	if p == nil {
		return
	}
	p.StatusLine = ""
}

func (p *BottomPane) RenderLines() []tuirender.Line {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(p.StatusLine)
	if s == "" {
		return nil
	}
	dim := lipgloss.NewStyle().Faint(true)
	return []tuirender.Line{{Spans: []tuirender.Span{{Text: s, Style: dim}}}}
}
