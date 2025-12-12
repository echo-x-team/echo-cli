package render

import (
	"fmt"
	"strings"

	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

type planUpdatedRenderer struct{}

func (planUpdatedRenderer) Type() events.EventType { return events.EventPlanUpdated }

func (planUpdatedRenderer) Handle(ctx *Context, evt events.Event) {
	args, ok := evt.Payload.(tools.UpdatePlanArgs)
	if !ok || ctx.Transcript == nil {
		return
	}
	text := formatPlanUpdateText(args.Plan, args.Explanation)
	if strings.TrimSpace(text) == "" {
		return
	}
	// Render plan updates as a user-style block in the transcript.
	ctx.Emit(ctx.Transcript.AppendUser(text))
}

func formatPlanUpdateText(plan []tools.PlanItem, explanation string) string {
	var sb strings.Builder
	sb.WriteString("Plan update")
	if strings.TrimSpace(explanation) != "" {
		sb.WriteString("\nexplanation: " + strings.TrimSpace(explanation))
	}
	if len(plan) == 0 {
		sb.WriteString("\nplan: (empty)")
		return sb.String()
	}
	for _, item := range plan {
		icon := "•"
		switch item.Status {
		case "completed":
			icon = "✓"
		case "in_progress":
			icon = "→"
		}
		sb.WriteString(fmt.Sprintf("\n- [%s] %s", icon, item.Step))
	}
	return sb.String()
}
