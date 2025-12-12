package repl

import (
	"fmt"
	"io"
	"os"

	tuirender "echo-cli/internal/tui/render"
)

// Scrollback 对齐 codex 的“历史区”概念：一旦内容完成，就作为不可变的 block
// 追加写入终端的自然滚动缓冲（或任意 io.Writer）。
//
// 注意：这里的 Scrollback 只负责“输出策略”，不负责持久化历史。
type Scrollback struct {
	w     io.Writer
	width int
}

type ScrollbackOptions struct {
	Writer io.Writer
	Width  int
}

func NewScrollback(opts ScrollbackOptions) *Scrollback {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	width := opts.Width
	if width <= 0 {
		width = 80
	}
	return &Scrollback{w: w, width: width}
}

func (s *Scrollback) SetWidth(width int) {
	if s == nil {
		return
	}
	if width > 0 {
		s.width = width
	}
}

func (s *Scrollback) Width() int {
	if s == nil {
		return 0
	}
	return s.width
}

// AppendCell 将一个已完成的 HistoryCell 写入 scrollback。
func (s *Scrollback) AppendCell(cell HistoryCell) {
	if s == nil || cell == nil || s.w == nil {
		return
	}
	lines := cell.Render(s.width)
	for _, line := range tuirender.LinesToStrings(lines) {
		fmt.Fprintln(s.w, line)
	}
}
