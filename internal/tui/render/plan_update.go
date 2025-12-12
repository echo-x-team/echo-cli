package render

import (
	"strings"

	"echo-cli/internal/tools"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	planBulletStyle     = lipgloss.NewStyle().Faint(true)
	planHeaderStyle     = lipgloss.NewStyle().Bold(true)
	planBranchStyle     = lipgloss.NewStyle().Faint(true)
	planNoteStyle       = lipgloss.NewStyle().Faint(true).Italic(true)
	planCompletedStyle  = lipgloss.NewStyle().Faint(true).Strikethrough(true)
	planInProgressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#2DD4BF")).Bold(true) // cyan-ish
	planPendingStyle    = lipgloss.NewStyle().Faint(true)
	planEmptyStepsStyle = lipgloss.NewStyle().Faint(true).Italic(true)
)

// RenderPlanUpdate renders a plan snapshot like Codex's PlanUpdateCell.
// width is the available terminal width for the section.
func RenderPlanUpdate(args tools.UpdatePlanArgs, width int) []Line {
	if width <= 0 {
		width = 80
	}

	lines := []Line{
		{Spans: []Span{
			{Text: "• ", Style: planBulletStyle},
			{Text: "Updated Plan", Style: planHeaderStyle},
		}},
	}

	indented := []Line{}

	// Optional explanation/note.
	note := strings.TrimSpace(args.Explanation)
	if note != "" {
		wrapWidth := width - 4
		if wrapWidth < 1 {
			wrapWidth = 1
		}
		for _, s := range wrapText(note, wrapWidth) {
			indented = append(indented, Line{Spans: []Span{{Text: s, Style: planNoteStyle}}})
		}
	}

	// Steps.
	if len(args.Plan) == 0 {
		indented = append(indented, Line{Spans: []Span{{Text: "(no steps provided)", Style: planEmptyStepsStyle}}})
	} else {
		for _, item := range args.Plan {
			indented = append(indented, renderPlanStep(item, width)...)
		}
	}

	// Outer indent: "  └ " for the first line, "    " for subsequent ones.
	indented = PrefixLines(
		indented,
		Span{Text: "  └ ", Style: planBranchStyle},
		Span{Text: "    ", Style: lipgloss.NewStyle()},
	)
	lines = append(lines, indented...)

	return lines
}

func renderPlanStep(item tools.PlanItem, width int) []Line {
	status := strings.TrimSpace(item.Status)
	text := strings.TrimSpace(item.Step)

	boxStr := "□ "
	stepStyle := planPendingStyle
	switch status {
	case "completed":
		boxStr = "✔ "
		stepStyle = planCompletedStyle
	case "in_progress":
		boxStr = "□ "
		stepStyle = planInProgressStyle
	case "pending":
		boxStr = "□ "
		stepStyle = planPendingStyle
	}

	// Reserve 4 columns for outer indent, then the checkbox prefix width.
	wrapWidth := width - 4 - runewidth.StringWidth(boxStr)
	if wrapWidth < 1 {
		wrapWidth = 1
	}
	parts := wrapText(text, wrapWidth)
	stepLines := make([]Line, 0, len(parts))
	for _, p := range parts {
		stepLines = append(stepLines, Line{Spans: []Span{{Text: p, Style: stepStyle}}})
	}

	// Prefix checkbox on first line, then two spaces for continuation lines.
	return PrefixLines(
		stepLines,
		Span{Text: boxStr, Style: lipgloss.Style{}},
		Span{Text: "  ", Style: lipgloss.Style{}},
	)
}
