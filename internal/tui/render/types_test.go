package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestLinesToPlainStrings(t *testing.T) {
	lines := []Line{
		{
			Spans: []Span{
				{Text: "• ", Style: lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))},
				{Text: "hello", Style: lipgloss.NewStyle().Bold(true)},
			},
		},
		{Spans: []Span{}},
	}

	got := LinesToPlainStrings(lines)
	want := []string{"• hello", ""}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d mismatch: got %q want %q", i, got[i], want[i])
		}
		if strings.Contains(got[i], "\x1b") {
			t.Errorf("line %d contains ANSI sequences: %q", i, got[i])
		}
	}
}
