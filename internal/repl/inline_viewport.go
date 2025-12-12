package repl

import tuirender "echo-cli/internal/tui/render"

// InlineViewport 对齐 codex 的“可重绘区域”：它位于屏幕底部，通常包含：
// - ActiveCell：仍在变化的内容（例如流式输出）
// - BottomPane：输入/状态/弹窗等
//
// 当前实现仅作为结构边界（便于拆分职责与测试）；exec/human 输出暂不做
// 真实的终端 viewport diff/贴底重绘。
type InlineViewport struct {
	Active     *ActiveCell
	BottomPane *BottomPane
}

func NewInlineViewport() *InlineViewport {
	return &InlineViewport{
		Active:     &ActiveCell{},
		BottomPane: &BottomPane{},
	}
}

func (v *InlineViewport) Clear() {
	if v == nil {
		return
	}
	if v.Active != nil {
		v.Active.Clear()
	}
	if v.BottomPane != nil {
		v.BottomPane.Clear()
	}
}

// DesiredHeight 返回 inline viewport 的理想高度（按渲染后的行数估算）。
func (v *InlineViewport) DesiredHeight(width int) int {
	if v == nil {
		return 0
	}
	h := 0
	if v.Active != nil {
		h += len(v.Active.RenderLines(width))
	}
	if v.BottomPane != nil {
		h += len(v.BottomPane.RenderLines())
	}
	return h
}

func (v *InlineViewport) RenderLines(width int) []tuirender.Line {
	if v == nil {
		return nil
	}
	var out []tuirender.Line
	if v.Active != nil {
		out = append(out, v.Active.RenderLines(width)...)
	}
	if v.BottomPane != nil {
		out = append(out, v.BottomPane.RenderLines()...)
	}
	return out
}
