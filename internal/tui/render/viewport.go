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

// NewHighPerformanceViewport 创建视口；Bubble Tea v1 推荐默认渲染器，因此不启用
// 兼容的高性能命令路径。
func NewHighPerformanceViewport(width, height int) HighPerformanceViewport {
	vp := viewport.New(width, height)
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
	return nil
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

	stickToBottom := v.AtBottom()
	v.lastLines = append([]string(nil), lines...)

	v.SetContent(strings.Join(lines, "\n"))
	if stickToBottom {
		v.GotoBottom()
	}
	return nil
}

// ScrollPageDown 下翻一页。
func (v *HighPerformanceViewport) ScrollPageDown() tea.Cmd {
	if v == nil {
		return nil
	}
	v.PageDown()
	return nil
}

// ScrollPageUp 上翻一页。
func (v *HighPerformanceViewport) ScrollPageUp() tea.Cmd {
	if v == nil {
		return nil
	}
	v.PageUp()
	return nil
}

// ScrollLineDown 下滚 n 行。
func (v *HighPerformanceViewport) ScrollLineDown(n int) tea.Cmd {
	if v == nil {
		return nil
	}
	v.ScrollDown(n)
	return nil
}

// ScrollLineUp 上滚 n 行。
func (v *HighPerformanceViewport) ScrollLineUp(n int) tea.Cmd {
	if v == nil {
		return nil
	}
	v.ScrollUp(n)
	return nil
}

// GotoTopCmd 跳转顶部并返回同步命令。
func (v *HighPerformanceViewport) GotoTopCmd() tea.Cmd {
	if v == nil {
		return nil
	}
	v.GotoTop()
	return nil
}

// GotoBottomCmd 跳转底部并返回同步命令。
func (v *HighPerformanceViewport) GotoBottomCmd() tea.Cmd {
	if v == nil {
		return nil
	}
	v.GotoBottom()
	return nil
}

// Invalidate 清空已缓存的行，强制下次更新走全量同步。
func (v *HighPerformanceViewport) Invalidate() {
	if v == nil {
		return
	}
	v.lastLines = nil
}
