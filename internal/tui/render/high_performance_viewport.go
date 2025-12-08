package render

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// HighPerformanceViewport 包装 bubbles viewport，提供 diff 感知与滚动命令生成。
type HighPerformanceViewport struct {
	viewport.Model
	lastLines []string
}

// NewHighPerformanceViewport 创建开启高性能渲染的视口。
func NewHighPerformanceViewport(width, height int) HighPerformanceViewport {
	vp := viewport.New(width, height)
	vp.HighPerformanceRendering = true
	return HighPerformanceViewport{Model: vp}
}

// Resize 更新宽高并在变更时返回同步命令。
func (v *HighPerformanceViewport) Resize(width, height int) tea.Cmd {
	if v == nil {
		return nil
	}
	widthChanged := v.Width != width
	heightChanged := v.Height != height
	if !widthChanged && !heightChanged {
		return nil
	}
	v.Width = width
	v.Height = height
	if widthChanged {
		v.Invalidate()
	}
	return v.sync()
}

// SetYPosition 更新视口在终端中的起始行。
func (v *HighPerformanceViewport) SetYPosition(y int) {
	if v == nil {
		return
	}
	v.YPosition = y
}

// HandleUpdate 代理 bubbles 的 Update，保持内部状态。
func (v *HighPerformanceViewport) HandleUpdate(msg tea.Msg) tea.Cmd {
	if v == nil {
		return nil
	}
	var cmd tea.Cmd
	v.Model, cmd = v.Model.Update(msg)
	return cmd
}

// SetLines 更新内容并根据差分选择增量或全量同步命令。
func (v *HighPerformanceViewport) SetLines(lines []string) tea.Cmd {
	if v == nil {
		return nil
	}
	if slices.Equal(lines, v.lastLines) {
		return nil
	}

	prev := v.lastLines
	stickToBottom := v.AtBottom()
	v.lastLines = append([]string(nil), lines...)

	v.SetContent(strings.Join(lines, "\n"))
	if stickToBottom {
		v.GotoBottom()
	}

	if appended := appendedLines(prev, lines); stickToBottom && len(appended) > 0 {
		return v.scrollDown(appended)
	}
	return v.sync()
}

// ScrollPageDown 下翻一页。
func (v *HighPerformanceViewport) ScrollPageDown() tea.Cmd {
	if v == nil {
		return nil
	}
	lines := v.ViewDown()
	if v.HighPerformanceRendering {
		return viewport.ViewDown(v.Model, lines)
	}
	return nil
}

// ScrollPageUp 上翻一页。
func (v *HighPerformanceViewport) ScrollPageUp() tea.Cmd {
	if v == nil {
		return nil
	}
	lines := v.ViewUp()
	if v.HighPerformanceRendering {
		return viewport.ViewUp(v.Model, lines)
	}
	return nil
}

// ScrollLineDown 下滚 n 行。
func (v *HighPerformanceViewport) ScrollLineDown(n int) tea.Cmd {
	if v == nil {
		return nil
	}
	lines := v.LineDown(n)
	if v.HighPerformanceRendering {
		return viewport.ViewDown(v.Model, lines)
	}
	return nil
}

// ScrollLineUp 上滚 n 行。
func (v *HighPerformanceViewport) ScrollLineUp(n int) tea.Cmd {
	if v == nil {
		return nil
	}
	lines := v.LineUp(n)
	if v.HighPerformanceRendering {
		return viewport.ViewUp(v.Model, lines)
	}
	return nil
}

// GotoTopCmd 跳转顶部并返回同步命令。
func (v *HighPerformanceViewport) GotoTopCmd() tea.Cmd {
	if v == nil {
		return nil
	}
	v.GotoTop()
	return v.sync()
}

// GotoBottomCmd 跳转底部并返回同步命令。
func (v *HighPerformanceViewport) GotoBottomCmd() tea.Cmd {
	if v == nil {
		return nil
	}
	v.GotoBottom()
	return v.sync()
}

// Invalidate 清空已缓存的行，强制下次更新走全量同步。
func (v *HighPerformanceViewport) Invalidate() {
	if v == nil {
		return
	}
	v.lastLines = nil
}

func (v *HighPerformanceViewport) sync() tea.Cmd {
	if v == nil || !v.HighPerformanceRendering {
		return nil
	}
	return viewport.Sync(v.Model)
}

func (v *HighPerformanceViewport) scrollDown(lines []string) tea.Cmd {
	if v == nil || !v.HighPerformanceRendering {
		return nil
	}
	if len(lines) == 0 {
		return viewport.Sync(v.Model)
	}
	return viewport.ViewDown(v.Model, lines)
}

func appendedLines(prev, next []string) []string {
	if len(next) == 0 || len(next) < len(prev) || len(prev) == 0 {
		return nil
	}
	if !slices.Equal(prev, next[:len(prev)]) {
		return nil
	}
	return next[len(prev):]
}
