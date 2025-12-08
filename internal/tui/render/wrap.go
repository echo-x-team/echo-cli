package render

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// wrapText 使用词级别换行。
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	lines := []string{}
	for _, raw := range strings.Split(text, "\n") {
		if raw == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapLine(raw, width)...)
	}
	if len(lines) == 0 {
		lines = append(lines, "")
	}
	return lines
}

func wrapLine(line string, width int) []string {
	if width <= 0 || runewidth.StringWidth(line) <= width {
		return []string{line}
	}
	out := []string{}
	current := ""
	for _, word := range strings.Fields(line) {
		if current == "" {
			if runewidth.StringWidth(word) > width {
				out = append(out, breakLongWord(word, width)...)
				continue
			}
			current = word
			continue
		}
		if runewidth.StringWidth(current)+1+runewidth.StringWidth(word) <= width {
			current += " " + word
			continue
		}
		out = append(out, current)
		if runewidth.StringWidth(word) > width {
			out = append(out, breakLongWord(word, width)...)
			current = ""
			continue
		}
		current = word
	}
	if current != "" {
		out = append(out, current)
	}
	if len(out) == 0 {
		return []string{line}
	}
	return out
}

func breakLongWord(word string, width int) []string {
	if width <= 0 {
		return []string{word}
	}
	out := []string{}
	current := []rune{}
	w := 0
	for _, r := range word {
		rw := runewidth.RuneWidth(r)
		if w+rw > width && len(current) > 0 {
			out = append(out, string(current))
			current = current[:0]
			w = 0
		}
		current = append(current, r)
		w += rw
	}
	if len(current) > 0 {
		out = append(out, string(current))
	}
	return out
}
