package render

import (
	"slices"
	"testing"
)

func TestWrapTextWithWideRunes(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{
			name:  "pure wide runes",
			text:  "你好世界",
			width: 4,
			want:  []string{"你好", "世界"},
		},
		{
			name:  "mix wide and ascii",
			text:  "你好 hello",
			width: 4,
			want:  []string{"你好", "hell", "o"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapText(tt.text, tt.width)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("wrapText(%q,%d)=%v want %v", tt.text, tt.width, got, tt.want)
			}
		})
	}
}
