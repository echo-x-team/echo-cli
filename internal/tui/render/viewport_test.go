package render

import "testing"

func TestHighPerformanceViewportSetLinesKeepsBottom(t *testing.T) {
	vp := NewHighPerformanceViewport(10, 2)
	_ = vp.SetLines([]string{"a", "b"})
	vp.GotoBottom()

	cmd := vp.SetLines([]string{"a", "b", "c"})
	if cmd != nil {
		t.Fatalf("expected no command, got %T", cmd)
	}
	if !vp.AtBottom() {
		t.Fatalf("viewport should stay anchored at bottom after append")
	}
}

func TestHighPerformanceViewportScrollLineDown(t *testing.T) {
	t.Run("standard rendering adjusts offset", func(t *testing.T) {
		vp := NewHighPerformanceViewport(8, 2)
		_ = vp.SetLines([]string{"a", "b", "c"})
		vp.SetYOffset(0)

		if cmd := vp.ScrollLineDown(1); cmd != nil {
			t.Fatalf("expected no command for standard rendering")
		}
		if vp.YOffset != 1 {
			t.Fatalf("unexpected YOffset after scroll: %d", vp.YOffset)
		}
	})

	t.Run("ignore extra scroll when at bottom", func(t *testing.T) {
		vp := NewHighPerformanceViewport(8, 2)
		_ = vp.SetLines([]string{"a", "b"})
		vp.GotoBottom()

		if cmd := vp.ScrollLineDown(1); cmd != nil {
			t.Fatalf("expected no command when scrolling at bottom, got %T", cmd)
		}
		if !vp.AtBottom() {
			t.Fatalf("viewport should stay at bottom")
		}
	})
}
