package repl

import (
	"fmt"
	"strings"

	"echo-cli/internal/tools"

	tuirender "echo-cli/internal/tui/render"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const maxToolOutputLines = 40

type toolEventCell struct {
	ev tools.ToolEvent
}

func newToolEventCell(ev tools.ToolEvent) HistoryCell {
	return toolEventCell{ev: ev}
}

func (c toolEventCell) ID() string { return c.ev.Result.ID }

func (c toolEventCell) Render(width int) []tuirender.Line {
	if width <= 0 {
		width = 80
	}

	dim := lipgloss.NewStyle().Faint(true)
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00AA00")).Bold(true)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CC0000")).Bold(true)
	kindStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)

	var out []tuirender.Line
	kind := string(c.ev.Result.Kind)

	switch c.ev.Type {
	case "item.updated":
		// Keep REPL output low-noise; item.updated is mostly internal status today.
		return nil
	}

	// item.started / item.completed
	switch c.ev.Type {
	case "item.started":
		header, detail := toolStartSummary(c.ev.Result)
		if detail == "" {
			detail = kind
		}
		out = append(out, tuirender.Line{Spans: []tuirender.Span{
			{Text: header, Style: kindStyle},
			{Text: " ", Style: kindStyle},
			{Text: detail, Style: dim},
		}})
		return out
	case "item.completed":
		success := c.ev.Result.Error == "" && c.ev.Result.Status != "error"
		icon := "✓"
		iconStyle := okStyle
		statusText := "completed"
		if !success {
			icon = "✗"
			iconStyle = errStyle
			statusText = "failed"
		}
		out = append(out, tuirender.Line{Spans: []tuirender.Span{
			{Text: icon + " ", Style: iconStyle},
			{Text: kind, Style: kindStyle},
			{Text: " ", Style: kindStyle},
			{Text: statusText, Style: dim},
		}})

		// Details, indented.
		details := toolDetailsLines(c.ev.Result, width-4)
		if len(details) > 0 {
			for _, line := range details {
				out = append(out, tuirender.Line{Spans: []tuirender.Span{
					{Text: "  └ ", Style: dim},
					{Text: line, Style: dim},
				}})
			}
		}
		return out
	default:
		out = append(out, tuirender.Line{Spans: []tuirender.Span{{Text: fmt.Sprintf("%s %s", c.ev.Type, kind), Style: dim}}})
		return out
	}
}

func toolStartSummary(res tools.ToolResult) (prefix string, detail string) {
	switch res.Kind {
	case tools.ToolCommand:
		prefix = "> running"
		detail = strings.TrimSpace(res.Command)
	case tools.ToolApplyPatch:
		prefix = "Δ applying"
		detail = strings.TrimSpace(res.Path)
	case tools.ToolFileRead:
		prefix = "↳ reading"
		detail = strings.TrimSpace(res.Path)
	case tools.ToolSearch:
		prefix = "🔍 searching"
		detail = strings.TrimSpace(res.Output)
	default:
		prefix = "• running"
		detail = strings.TrimSpace(res.Status)
	}
	return prefix, detail
}

func toolDetailsLines(res tools.ToolResult, width int) []string {
	var out []string

	if strings.TrimSpace(res.Command) != "" {
		out = append(out, "command: "+strings.TrimSpace(res.Command))
	}
	if strings.TrimSpace(res.Path) != "" {
		out = append(out, "path: "+strings.TrimSpace(res.Path))
	}
	if res.ExitCode != 0 {
		out = append(out, fmt.Sprintf("exit_code: %d", res.ExitCode))
	}
	if strings.TrimSpace(res.Error) != "" {
		out = append(out, "error: "+strings.TrimSpace(res.Error))
	}
	if strings.TrimSpace(res.Output) != "" {
		out = append(out, "output:")
		out = append(out, wrapAndTruncate(res.Output, width, maxToolOutputLines)...)
	}
	return out
}

func wrapAndTruncate(text string, width int, maxLines int) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	if width <= 0 {
		width = 80
	}
	var lines []string
	for _, raw := range strings.Split(text, "\n") {
		if raw == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapLineWords(raw, width)...)
		if maxLines > 0 && len(lines) >= maxLines {
			return lines[:maxLines]
		}
	}
	if maxLines > 0 && len(lines) > maxLines {
		return lines[:maxLines]
	}
	return lines
}

func wrapLineWords(line string, width int) []string {
	if width <= 0 || runewidth.StringWidth(line) <= width {
		return []string{line}
	}
	var out []string
	cur := ""
	for _, word := range strings.Fields(line) {
		if cur == "" {
			cur = word
			continue
		}
		if runewidth.StringWidth(cur)+1+runewidth.StringWidth(word) <= width {
			cur += " " + word
			continue
		}
		out = append(out, cur)
		cur = word
	}
	if cur != "" {
		out = append(out, cur)
	}
	if len(out) == 0 {
		return []string{line}
	}
	return out
}
