package repl

import (
	"strings"

	tuirender "echo-cli/internal/tui/render"
)

// ActiveCell 对齐 codex 的“active_cell”：承载仍在变化的内容（例如流式输出），
// 在合适的时机 flush 到 Scrollback。
//
// 目前 exec/human 输出不会做 in-place redraw，但 ActiveCell 仍提供明确的边界，
// 让 “delta -> 最终 cell -> 追加到 scrollback” 的生命周期更清晰。
type ActiveCell struct {
	submissionID string
	buf          strings.Builder
}

func (c *ActiveCell) Begin(submissionID string) {
	if c == nil {
		return
	}
	c.submissionID = submissionID
	c.buf.Reset()
}

func (c *ActiveCell) SubmissionID() string {
	if c == nil {
		return ""
	}
	return c.submissionID
}

func (c *ActiveCell) AppendDelta(submissionID, delta string) {
	if c == nil || delta == "" {
		return
	}
	// 保守：只接受当前 active submission 的 delta；若尚未 Begin，则接管为 active。
	if c.submissionID == "" {
		c.submissionID = submissionID
	}
	if c.submissionID != submissionID {
		return
	}
	c.buf.WriteString(delta)
}

func (c *ActiveCell) Text() string {
	if c == nil {
		return ""
	}
	return c.buf.String()
}

// Finalize 将 active 内容转为不可变的 HistoryCell，并清空 active。
// 若 finalText 为空，会回退到累积的 delta 缓冲。
func (c *ActiveCell) Finalize(submissionID, finalText string) HistoryCell {
	if c == nil {
		return nil
	}
	if c.submissionID != "" && submissionID != "" && c.submissionID != submissionID {
		return nil
	}
	text := strings.TrimSpace(finalText)
	if text == "" {
		text = strings.TrimSpace(c.buf.String())
	}
	c.Clear()
	if text == "" {
		return nil
	}
	return newAssistantCell(text)
}

func (c *ActiveCell) Clear() {
	if c == nil {
		return
	}
	c.submissionID = ""
	c.buf.Reset()
}

// RenderLines 用于 inline viewport 的渲染（未来可做贴底/裁剪）。
func (c *ActiveCell) RenderLines(width int) []tuirender.Line {
	if c == nil {
		return nil
	}
	text := strings.TrimSpace(c.buf.String())
	if text == "" {
		return nil
	}
	return newAssistantCell(text).Render(width)
}
