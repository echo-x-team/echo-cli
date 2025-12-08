package tui

import (
	"fmt"
	"time"

	"echo-cli/internal/tui/render"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// StatusIndicatorState 枚举了状态指示器可显示的所有状态。
type StatusIndicatorState int

const (
	// StatusWorking 表示任务正在进行，计时器持续累加。
	StatusWorking StatusIndicatorState = iota
	// StatusWaiting 表示等待外部响应（例如排队或阻塞的请求），计时器持续累加。
	StatusWaiting
	// StatusPaused 表示暂停计时（例如等待用户动作但无需提示中断）。
	StatusPaused
	// StatusError 表示任务进入错误态。
	StatusError
	// StatusIdle 表示空闲，不显示状态行。
	StatusIdle
)

func (s StatusIndicatorState) String() string {
	switch s {
	case StatusWorking:
		return "working"
	case StatusWaiting:
		return "waiting"
	case StatusPaused:
		return "paused"
	case StatusError:
		return "error"
	case StatusIdle:
		return "idle"
	default:
		return "unknown"
	}
}

func (s StatusIndicatorState) defaultHeader() string {
	switch s {
	case StatusWorking:
		return "Working"
	case StatusWaiting:
		return "Waiting"
	case StatusPaused:
		return "Paused"
	case StatusError:
		return "Error"
	default:
		return ""
	}
}

func (s StatusIndicatorState) tracksElapsed() bool {
	return s == StatusWorking || s == StatusWaiting
}

func (s StatusIndicatorState) interruptible() bool {
	return s == StatusWorking || s == StatusWaiting
}

func (s StatusIndicatorState) visible() bool {
	return s != StatusIdle
}

func (s StatusIndicatorState) valid() bool {
	switch s {
	case StatusWorking, StatusWaiting, StatusPaused, StatusError, StatusIdle:
		return true
	default:
		return false
	}
}

// StatusIndicatorOptions 控制指示器的初始化行为。
type StatusIndicatorOptions struct {
	State             StatusIndicatorState
	Header            string
	AnimationsEnabled bool
	ShowInterruptHint *bool
	OnInterrupt       func()
	Clock             func() time.Time
}

// StatusIndicatorWidget 渲染与管理状态行（spinner + 标题 + 计时/中断提示）。
type StatusIndicatorWidget struct {
	header            string
	showInterruptHint bool
	state             StatusIndicatorState
	animationsEnabled bool
	onInterrupt       func()

	elapsedRunning time.Duration
	lastResumeAt   time.Time
	paused         bool

	clock func() time.Time
}

// NewStatusIndicatorWidget 构造默认的状态指示器，默认处于 Working。
func NewStatusIndicatorWidget(opts StatusIndicatorOptions) *StatusIndicatorWidget {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	state := opts.State
	if !state.valid() {
		state = StatusWorking
	}

	header := opts.Header
	if header == "" {
		header = state.defaultHeader()
	}

	showHint := true
	if opts.ShowInterruptHint != nil {
		showHint = *opts.ShowInterruptHint
	}

	now := clock()
	w := &StatusIndicatorWidget{
		header:            header,
		showInterruptHint: showHint,
		state:             state,
		animationsEnabled: opts.AnimationsEnabled,
		onInterrupt:       opts.OnInterrupt,
		clock:             clock,
		lastResumeAt:      now,
	}
	if !state.tracksElapsed() {
		w.paused = true
	}
	return w
}

// UpdateHeader 允许动态更新标题文本。
func (w *StatusIndicatorWidget) UpdateHeader(header string) {
	if w == nil {
		return
	}
	w.header = header
}

// SetState 更新状态并根据状态是否计时自动处理计时器。
func (w *StatusIndicatorWidget) SetState(state StatusIndicatorState) {
	if w == nil || !state.valid() {
		return
	}
	now := w.now()
	w.syncTimerForState(now, state)
	w.state = state
	w.header = state.defaultHeader()
}

// SetInterruptHintVisible 控制是否显示 Esc 提示。
func (w *StatusIndicatorWidget) SetInterruptHintVisible(visible bool) {
	if w == nil {
		return
	}
	w.showInterruptHint = visible
}

// Interrupt 触发外部中断回调（若已配置）。
func (w *StatusIndicatorWidget) Interrupt() {
	if w == nil || w.onInterrupt == nil || !w.state.interruptible() {
		return
	}
	w.onInterrupt()
}

// PauseTimer 停止计时。
func (w *StatusIndicatorWidget) PauseTimer() {
	if w == nil {
		return
	}
	w.pauseTimerAt(w.now())
}

// ResumeTimer 继续计时。
func (w *StatusIndicatorWidget) ResumeTimer() {
	if w == nil {
		return
	}
	w.resumeTimerAt(w.now())
}

// ElapsedSeconds 返回累计秒数。
func (w *StatusIndicatorWidget) ElapsedSeconds() uint64 {
	if w == nil {
		return 0
	}
	return w.elapsedSecondsAt(w.now())
}

// DesiredHeight 满足 Renderable 接口。
func (w *StatusIndicatorWidget) DesiredHeight(_ int) int {
	if w == nil || !w.state.visible() {
		return 0
	}
	return 1
}

// CursorPos 满足 Renderable 接口。
func (w *StatusIndicatorWidget) CursorPos(render.Rect) *render.CursorPos {
	return nil
}

// Render 绘制状态行：spinner + 标题 + 计时/中断提示。
func (w *StatusIndicatorWidget) Render(area render.Rect, buf *render.Buffer) {
	if w == nil || buf == nil || area.Height <= 0 || area.Width <= 0 || !w.state.visible() {
		return
	}

	now := w.now()
	elapsed := w.elapsedDurationAt(now)
	prettyElapsed := fmtElapsedCompact(uint64(elapsed.Seconds()))

	spans := []render.Span{
		{Text: w.spinnerFrame(now)},
	}
	if w.header != "" {
		spans = append(spans, render.Span{Text: " "}, render.Span{Text: w.header})
	}

	hint := formatHint(prettyElapsed, w.showInterruptHint && w.state.interruptible())
	spans = append(spans, render.Span{Text: " "}, render.Span{
		Text:  hint,
		Style: lipgloss.NewStyle().Faint(true),
	})

	clamped := clampSpans(spans, area.Width)
	if len(clamped) == 0 {
		return
	}
	buf.WriteLine(render.Line{Spans: clamped})
}

func (w *StatusIndicatorWidget) now() time.Time {
	if w.clock != nil {
		return w.clock()
	}
	return time.Now()
}

func (w *StatusIndicatorWidget) syncTimerForState(now time.Time, next StatusIndicatorState) {
	if next.tracksElapsed() && w.paused {
		w.resumeTimerAt(now)
		return
	}
	if !next.tracksElapsed() && !w.paused {
		w.pauseTimerAt(now)
	}
}

func (w *StatusIndicatorWidget) pauseTimerAt(now time.Time) {
	if w.paused {
		return
	}
	w.elapsedRunning += now.Sub(w.lastResumeAt)
	w.paused = true
}

func (w *StatusIndicatorWidget) resumeTimerAt(now time.Time) {
	if !w.paused {
		return
	}
	w.lastResumeAt = now
	w.paused = false
}

func (w *StatusIndicatorWidget) elapsedDurationAt(now time.Time) time.Duration {
	if w.paused {
		return w.elapsedRunning
	}
	return w.elapsedRunning + now.Sub(w.lastResumeAt)
}

func (w *StatusIndicatorWidget) elapsedSecondsAt(now time.Time) uint64 {
	return uint64(w.elapsedDurationAt(now).Seconds())
}

func (w *StatusIndicatorWidget) spinnerFrame(now time.Time) string {
	switch w.state {
	case StatusPaused:
		return "||"
	case StatusError:
		return "!"
	case StatusIdle:
		return ""
	}
	if w.animationsEnabled {
		frames := []string{"-", "\\", "|", "/"}
		idx := int(now.UnixMilli()/120) % len(frames)
		return frames[idx]
	}
	return "•"
}

func formatHint(elapsed string, interruptible bool) string {
	if interruptible {
		return fmt.Sprintf("(%s • esc to interrupt)", elapsed)
	}
	return fmt.Sprintf("(%s)", elapsed)
}

// fmtElapsedCompact 将秒数格式化为友好字符串（对齐 Rust 参考实现）。
func fmtElapsedCompact(elapsedSecs uint64) string {
	switch {
	case elapsedSecs < 60:
		return fmt.Sprintf("%ds", elapsedSecs)
	case elapsedSecs < 3600:
		minutes := elapsedSecs / 60
		seconds := elapsedSecs % 60
		return fmt.Sprintf("%dm %02ds", minutes, seconds)
	default:
		hours := elapsedSecs / 3600
		minutes := (elapsedSecs % 3600) / 60
		seconds := elapsedSecs % 60
		return fmt.Sprintf("%dh %02dm %02ds", hours, minutes, seconds)
	}
}

func clampSpans(spans []render.Span, width int) []render.Span {
	if width <= 0 {
		return nil
	}
	remaining := width
	out := make([]render.Span, 0, len(spans))
	for _, sp := range spans {
		if remaining <= 0 {
			break
		}
		tw := runewidth.StringWidth(sp.Text)
		if tw <= remaining {
			out = append(out, sp)
			remaining -= tw
			continue
		}
		text := truncateToWidth(sp.Text, remaining)
		if text != "" {
			sp.Text = text
			out = append(out, sp)
			remaining = 0
		}
	}
	return out
}

func truncateToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	w := 0
	out := make([]rune, 0, len(text))
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if w+rw > width {
			break
		}
		out = append(out, r)
		w += rw
	}
	return string(out)
}
