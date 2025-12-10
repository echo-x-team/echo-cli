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
	lastLines         []string
	lastContentHeight int
	followBottom      bool
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
	v.clampOffset(v.lastContentHeight)
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
	// Track whether we are currently at the bottom to decide follow-bottom behavior.
	if v.AtBottom() {
		v.followBottom = true
	} else if v.YOffset > 0 {
		v.followBottom = false
	}
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

	contentHeight := len(lines)
	// Stick to bottom if我们本就在底部，或内容尚不足一屏。
	stickToBottom := v.followBottom || v.AtBottom() || contentHeight <= v.Height
	v.lastLines = append([]string(nil), lines...)
	v.lastContentHeight = contentHeight

	v.SetContent(strings.Join(lines, "\n"))
	v.clampOffset(contentHeight)
	if stickToBottom {
		v.followBottom = true
		v.GotoBottom()
	} else {
		v.followBottom = false
	}
	return nil
}

// ScrollPageDown 下翻一页。
func (v *HighPerformanceViewport) ScrollPageDown() tea.Cmd {
	if v == nil {
		return nil
	}
	v.followBottom = false
	v.PageDown()
	if v.AtBottom() {
		v.followBottom = true
	}
	return nil
}

// ScrollPageUp 上翻一页。
func (v *HighPerformanceViewport) ScrollPageUp() tea.Cmd {
	if v == nil {
		return nil
	}
	v.followBottom = false
	v.PageUp()
	return nil
}

// ScrollLineDown 下滚 n 行。
func (v *HighPerformanceViewport) ScrollLineDown(n int) tea.Cmd {
	if v == nil {
		return nil
	}
	v.followBottom = false
	v.ScrollDown(n)
	if v.AtBottom() {
		v.followBottom = true
	}
	return nil
}

// ScrollLineUp 上滚 n 行。
func (v *HighPerformanceViewport) ScrollLineUp(n int) tea.Cmd {
	if v == nil {
		return nil
	}
	v.followBottom = false
	v.ScrollUp(n)
	return nil
}

// GotoTopCmd 跳转顶部并返回同步命令。
func (v *HighPerformanceViewport) GotoTopCmd() tea.Cmd {
	if v == nil {
		return nil
	}
	v.followBottom = false
	v.GotoTop()
	return nil
}

// GotoBottomCmd 跳转底部并返回同步命令。
func (v *HighPerformanceViewport) GotoBottomCmd() tea.Cmd {
	if v == nil {
		return nil
	}
	v.followBottom = true
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

// PercentScrolled 返回可视窗口在内容中的滚动百分比。
func (v *HighPerformanceViewport) PercentScrolled() int {
	if v == nil {
		return 100
	}
	contentHeight := v.lastContentHeight
	if contentHeight == 0 {
		contentHeight = len(v.lastLines)
	}
	if contentHeight <= 0 || v.Height == 0 || contentHeight <= v.Height {
		return 100
	}
	maxOffset := contentHeight - v.Height
	if maxOffset <= 0 {
		return 100
	}
	if v.YOffset >= maxOffset {
		return 100
	}
	return int(float64(v.YOffset) / float64(maxOffset) * 100.0)
}

// ContentOverflow 表示内容高度是否超过视口。
func (v *HighPerformanceViewport) ContentOverflow() bool {
	if v == nil {
		return false
	}
	contentHeight := v.lastContentHeight
	if contentHeight == 0 {
		contentHeight = len(v.lastLines)
	}
	if v.Height <= 0 {
		return false
	}
	return contentHeight > v.Height
}

// FollowingBottom 表示当前是否维持“贴底”滚动。
func (v *HighPerformanceViewport) FollowingBottom() bool {
	if v == nil {
		return true
	}
	if v.lastContentHeight <= v.Height {
		return true
	}
	return v.followBottom
}

func (v *HighPerformanceViewport) clampOffset(contentHeight int) {
	if v == nil {
		return
	}
	if contentHeight <= 0 || v.Height <= 0 {
		v.YOffset = 0
		return
	}
	maxYOffset := contentHeight - v.Height
	if maxYOffset < 0 {
		maxYOffset = 0
	}
	if v.YOffset > maxYOffset {
		v.YOffset = maxYOffset
	}
}
