package render

import (
	"fmt"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHighPerformanceViewportSetLinesDiff(t *testing.T) {
	tests := []struct {
		name       string
		initial    []string
		update     []string
		expectType string
	}{
		{
			name:       "append at bottom uses scroll down",
			initial:    []string{"a", "b"},
			update:     []string{"a", "b", "c"},
			expectType: "github.com/charmbracelet/bubbletea/scrollDownMsg",
		},
		{
			name:       "content replaced triggers sync",
			initial:    []string{"left", "right"},
			update:     []string{"x", "y"},
			expectType: "github.com/charmbracelet/bubbletea/syncScrollAreaMsg",
		},
		{
			name:    "no change returns no command",
			initial: []string{"same"},
			update:  []string{"same"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vp := NewHighPerformanceViewport(10, 2)
			_ = vp.SetLines(tt.initial)
			vp.GotoBottom()
			gotType := cmdType(vp.SetLines(tt.update))
			if gotType != tt.expectType {
				t.Fatalf("cmd type mismatch: got %q want %q", gotType, tt.expectType)
			}
		})
	}
}

func TestHighPerformanceViewportScrollLineDown(t *testing.T) {
	vp := NewHighPerformanceViewport(8, 2)
	_ = vp.SetLines([]string{"a", "b", "c"})
	vp.SetYOffset(0)
	msgType := cmdType(vp.ScrollLineDown(1))
	if msgType != "github.com/charmbracelet/bubbletea/scrollDownMsg" {
		t.Fatalf("unexpected cmd type: %s", msgType)
	}
}

func cmdType(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	msg := cmd()
	if msg == nil {
		return ""
	}
	typ := reflect.TypeOf(msg)
	return fmt.Sprintf("%s/%s", typ.PkgPath(), typ.Name())
}
