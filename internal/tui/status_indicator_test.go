package tui

import (
	"testing"
	"time"

	"echo-cli/internal/tui/render"

	"github.com/mattn/go-runewidth"
)

func TestFmtElapsedCompact(t *testing.T) {
	cases := []struct {
		seconds  uint64
		expected string
	}{
		{seconds: 0, expected: "0s"},
		{seconds: 1, expected: "1s"},
		{seconds: 59, expected: "59s"},
		{seconds: 60, expected: "1m 00s"},
		{seconds: 61, expected: "1m 01s"},
		{seconds: 3*60 + 5, expected: "3m 05s"},
		{seconds: 59*60 + 59, expected: "59m 59s"},
		{seconds: 3600, expected: "1h 00m 00s"},
		{seconds: 3600 + 60 + 1, expected: "1h 01m 01s"},
		{seconds: 25*3600 + 2*60 + 3, expected: "25h 02m 03s"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()
			if got := fmtElapsedCompact(tc.seconds); got != tc.expected {
				t.Fatalf("fmtElapsedCompact(%d) = %q, want %q", tc.seconds, got, tc.expected)
			}
		})
	}
}

func TestStatusIndicatorTimerPausesAndResumes(t *testing.T) {
	base := time.Unix(0, 0)
	now := base
	widget := NewStatusIndicatorWidget(StatusIndicatorOptions{
		Clock: func() time.Time { return now },
	})
	widget.lastResumeAt = base

	now = base.Add(5 * time.Second)
	beforePause := widget.elapsedSecondsAt(now)
	if beforePause != 5 {
		t.Fatalf("expected 5s before pause, got %d", beforePause)
	}

	widget.pauseTimerAt(now)

	now = base.Add(10 * time.Second)
	paused := widget.elapsedSecondsAt(now)
	if paused != beforePause {
		t.Fatalf("expected paused elapsed %d, got %d", beforePause, paused)
	}

	now = base.Add(10 * time.Second)
	widget.resumeTimerAt(now)

	now = base.Add(13 * time.Second)
	afterResume := widget.elapsedSecondsAt(now)
	if afterResume != beforePause+3 {
		t.Fatalf("expected resumed elapsed %d, got %d", beforePause+3, afterResume)
	}
}

func TestStatusIndicatorRender(t *testing.T) {
	now := time.Unix(0, 0)
	widget := NewStatusIndicatorWidget(StatusIndicatorOptions{
		Clock: func() time.Time { return now },
	})

	buf := render.Buffer{}
	widget.Render(render.Rect{Width: 80, Height: 1}, &buf)
	if len(buf.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(buf.Lines))
	}
	var combined string
	for _, sp := range buf.Lines[0].Spans {
		combined += sp.Text
	}
	expected := "• Working (0s • esc to interrupt)"
	if combined != expected {
		t.Fatalf("unexpected render output %q, want %q", combined, expected)
	}
}

func TestStatusIndicatorRenderClampsToWidth(t *testing.T) {
	widget := NewStatusIndicatorWidget(StatusIndicatorOptions{})

	buf := render.Buffer{}
	area := render.Rect{Width: 10, Height: 1}
	widget.Render(area, &buf)
	if len(buf.Lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(buf.Lines))
	}
	var combined string
	for _, sp := range buf.Lines[0].Spans {
		combined += sp.Text
	}
	if width := runewidth.StringWidth(combined); width > area.Width {
		t.Fatalf("rendered width %d exceeds area width %d", width, area.Width)
	}
}

func TestStatusIndicatorDesiredHeight(t *testing.T) {
	widget := NewStatusIndicatorWidget(StatusIndicatorOptions{State: StatusIdle})
	if h := widget.DesiredHeight(10); h != 0 {
		t.Fatalf("idle status should report height 0, got %d", h)
	}
	widget.SetState(StatusWorking)
	if h := widget.DesiredHeight(10); h != 1 {
		t.Fatalf("working status should report height 1, got %d", h)
	}
}
