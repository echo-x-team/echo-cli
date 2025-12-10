package slash

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	nameStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#C4A1FF"))
	descStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	highlightStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EBCB8B"))
	selectedStyle  = lipgloss.NewStyle().Background(lipgloss.Color("#2F2A3D"))
)

// View 渲染弹窗内容（不含外围边框）。
func (s *State) View(width int) string {
	if s == nil || !s.open {
		return ""
	}
	contentWidth := width
	if contentWidth <= 20 {
		contentWidth = 20
	}
	visible := s.visibleEntries(contentWidth)
	if len(visible) == 0 {
		return lipgloss.NewStyle().Width(contentWidth).Render("no matches")
	}

	lines := []string{}
	for _, entry := range visible {
		for i, line := range entry.lines {
			rendered := line
			if entry.selected {
				rendered = selectedStyle.Render(line)
			}
			if i == 0 {
				lines = append(lines, rendered)
				continue
			}
			lines = append(lines, rendered)
		}
	}

	return lipgloss.NewStyle().
		Width(contentWidth).
		Render(strings.Join(lines, "\n"))
}

type renderedEntry struct {
	lines    []string
	selected bool
	height   int
}

func (s *State) visibleEntries(contentWidth int) []renderedEntry {
	matches := s.matches
	if matches == nil {
		matches = []match{}
	}
	if len(matches) == 0 {
		return []renderedEntry{{
			lines:  []string{"no matches"},
			height: 1,
		}}
	}

	nameWidth, descWidth := s.computeColumnWidths(contentWidth, matches)
	entries := make([]renderedEntry, 0, len(matches))
	for idx, m := range matches {
		name := applyHighlights(m.item.DisplayName(), m.highlights, nameWidth)
		desc := m.item.Description
		if desc == "" {
			desc = "—"
		}
		descLines := wrap(desc, descWidth)
		lines := make([]string, 0, len(descLines))
		nameCell := lipgloss.NewStyle().Width(nameWidth).Render(nameStyle.Render(name))
		for i, raw := range descLines {
			dl := descStyle.Render(raw)
			if i == 0 {
				lines = append(lines, fmt.Sprintf("%s  %s", nameCell, dl))
			} else {
				lines = append(lines, fmt.Sprintf("%s  %s", strings.Repeat(" ", lipgloss.Width(nameCell)), dl))
			}
		}
		entries = append(entries, renderedEntry{
			lines:    lines,
			height:   len(lines),
			selected: idx == s.selected,
		})
	}
	return clampByHeight(entries, s.maxLines, s.selected)
}

func (s *State) computeColumnWidths(contentWidth int, matches []match) (int, int) {
	maxName := 0
	for _, m := range matches {
		name := m.item.DisplayName()
		if name == "" {
			name = "/"
		}
		if w := lipgloss.Width(name); w > maxName {
			maxName = w
		}
	}
	minName := 10
	if maxName < minName {
		maxName = minName
	}
	if maxName > contentWidth-12 {
		maxName = contentWidth - 12
	}
	descWidth := contentWidth - maxName - 2
	if descWidth < 8 {
		descWidth = 8
	}
	return maxName, descWidth
}

func clampByHeight(entries []renderedEntry, maxLines int, selected int) []renderedEntry {
	if maxLines <= 0 {
		return entries
	}
	start := 0
	for start < len(entries) {
		height := 0
		end := start
		for end < len(entries) && height+entries[end].height <= maxLines {
			height += entries[end].height
			end++
		}
		if selected < end {
			return entries[start:end]
		}
		start++
	}
	// fallback:至少包含选中项
	return []renderedEntry{entries[selected]}
}

func wrap(text string, width int) []string {
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
		return []string{""}
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
				out = append(out, breakWord(word, width)...)
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
			out = append(out, breakWord(word, width)...)
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

func breakWord(word string, width int) []string {
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

func applyHighlights(name string, indexes []int, width int) string {
	if len(indexes) == 0 {
		return name
	}
	runes := []rune(name)
	marked := map[int]bool{}
	for _, idx := range indexes {
		marked[idx] = true
	}
	parts := make([]string, 0, len(runes))
	for i, r := range runes {
		ch := string(r)
		if marked[i] {
			parts = append(parts, highlightStyle.Render(ch))
			continue
		}
		parts = append(parts, ch)
	}
	joined := strings.Join(parts, "")
	return lipgloss.NewStyle().Width(width).Render(joined)
}
