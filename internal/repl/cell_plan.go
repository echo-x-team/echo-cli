package repl

import (
	"fmt"
	"strings"

	"echo-cli/internal/tools"

	tuirender "echo-cli/internal/tui/render"
	"github.com/charmbracelet/lipgloss"
)

type planCell struct {
	args tools.UpdatePlanArgs
}

func (c planCell) ID() string { return "" }

func newPlanCell(args tools.UpdatePlanArgs) HistoryCell {
	// Defensive copy; treat plan as snapshot.
	cp := tools.UpdatePlanArgs{Explanation: strings.TrimSpace(args.Explanation)}
	cp.Plan = append([]tools.PlanItem(nil), args.Plan...)
	return planCell{args: cp}
}

func (c planCell) Render(width int) []tuirender.Line {
	var lines []tuirender.Line

	titleStyle := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)

	lines = append(lines, tuirender.Line{Spans: []tuirender.Span{{Text: "Plan update", Style: titleStyle}}})
	if c.args.Explanation != "" {
		lines = append(lines, tuirender.Line{Spans: []tuirender.Span{
			{Text: "explanation: ", Style: dim},
			{Text: c.args.Explanation, Style: dim},
		}})
	}
	if len(c.args.Plan) == 0 {
		lines = append(lines, tuirender.Line{Spans: []tuirender.Span{{Text: "(empty)", Style: dim}}})
		return lines
	}

	for _, item := range c.args.Plan {
		icon := "•"
		switch item.Status {
		case "completed":
			icon = "✓"
		case "in_progress":
			icon = "→"
		}
		lines = append(lines, tuirender.Line{Spans: []tuirender.Span{
			{Text: fmt.Sprintf("- [%s] %s", icon, item.Step)},
		}})
	}
	return lines
}
