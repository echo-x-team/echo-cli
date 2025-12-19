package execution

import (
	"encoding/json"
	"fmt"
	"strings"

	echocontext "echo-cli/internal/context"
	"echo-cli/internal/tools"
)

// ProcessedResponseItem pairs a ResponseItem with an optional ResponseInputItem.
type ProcessedResponseItem struct {
	Item     echocontext.ResponseItem
	Response *echocontext.ResponseInputItem
}

// ResponseInputFromToolResult converts a ToolResult into a ResponseInputItem.
func ResponseInputFromToolResult(result tools.ToolResult) echocontext.ResponseInputItem {
	success := result.Error == "" && result.ExitCode == 0 && strings.ToLower(result.Status) != "error"
	content := result.Output
	if content == "" && result.Error != "" {
		content = result.Error
	}
	if content == "" && len(result.Plan) > 0 {
		content = formatPlanText(result.Plan, result.Explanation)
	}
	if result.Kind == tools.ToolCommand {
		out := map[string]any{
			"output": strings.TrimRight(content, "\n"),
		}
		if strings.TrimSpace(result.SessionID) != "" {
			out["session_id"] = strings.TrimSpace(result.SessionID)
		} else {
			out["exit_code"] = result.ExitCode
		}
		if strings.TrimSpace(result.Error) != "" {
			out["error"] = strings.TrimSpace(result.Error)
		}
		if data, err := json.Marshal(out); err == nil {
			content = string(data)
		}
	}
	payload := echocontext.FunctionCallOutputPayload{Content: content, Success: &success}
	return echocontext.ResponseInputItem{
		Type: echocontext.ResponseInputTypeFunctionCallOutput,
		FunctionCallOutput: &echocontext.FunctionCallOutputInput{
			CallID: result.ID,
			Output: payload,
		},
	}
}

// processedFromToolResults pairs tool outputs with their ResponseInputItem.
func processedFromToolResults(results []tools.ToolResult) []ProcessedResponseItem {
	items := make([]ProcessedResponseItem, 0, len(results))
	for _, result := range results {
		resp := ResponseInputFromToolResult(result)
		items = append(items, ProcessedResponseItem{
			Item:     resp.ToResponseItem(),
			Response: &resp,
		})
	}
	return items
}

// processedFromResponseItems wraps raw ResponseItem entries into ProcessedResponseItem for persistence.
func processedFromResponseItems(items []echocontext.ResponseItem) []ProcessedResponseItem {
	out := make([]ProcessedResponseItem, 0, len(items))
	for _, item := range items {
		out = append(out, ProcessedResponseItem{Item: item})
	}
	return out
}

// formatPlanText mirrors the textual plan rendering used for tool results.
func formatPlanText(plan []tools.PlanItem, explanation string) string {
	var sb strings.Builder
	if strings.TrimSpace(explanation) != "" {
		sb.WriteString(strings.TrimSpace(explanation))
	}
	for _, item := range plan {
		icon := "•"
		switch item.Status {
		case "completed":
			icon = "✓"
		case "in_progress":
			icon = "→"
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s", icon, item.Step))
	}
	return sb.String()
}
