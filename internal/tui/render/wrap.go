package render

import (
	"strings"
	"unicode/utf8"
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
	if width <= 0 || utf8.RuneCountInString(line) <= width {
		return []string{line}
	}
	out := []string{}
	current := ""
	for _, word := range strings.Fields(line) {
		if current == "" {
			if utf8.RuneCountInString(word) > width {
				out = append(out, breakLongWord(word, width)...)
				continue
			}
			current = word
			continue
		}
		if utf8.RuneCountInString(current)+1+utf8.RuneCountInString(word) <= width {
			current += " " + word
			continue
		}
		out = append(out, current)
		if utf8.RuneCountInString(word) > width {
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
	runes := []rune(word)
	for len(runes) > 0 {
		if len(runes) <= width {
			out = append(out, string(runes))
			break
		}
		out = append(out, string(runes[:width]))
		runes = runes[width:]
	}
	return out
}
