package render

import (
	"fmt"
	"strings"

	"echo-cli/internal/tools"
)

const maxToolBlockLines = 60

// FormatToolEventBlock formats a tools.ToolEvent into a human-readable block suitable
// for embedding into the transcript view (role="tool"). The output is plain text
// (no ANSI) and is intended to be wrapped by the transcript renderer.
func FormatToolEventBlock(ev tools.ToolEvent) string {
	switch ev.Type {
	case "item.started":
		head, detail := toolStartLine(ev.Result)
		if detail == "" {
			detail = string(ev.Result.Kind)
		}
		line := fmt.Sprintf("%s %s", head, detail)
		if ev.Result.Kind == tools.ToolApplyPatch && strings.TrimSpace(ev.Result.Diff) != "" {
			return toolBlockWithDiff(line, ev.Result.Diff)
		}
		return line
	case "item.completed":
		return toolCompletedBlock(ev.Result)
	case "item.updated":
		return toolUpdatedBlock(ev.Result)
	default:
		return ""
	}
}

func toolUpdatedBlock(res tools.ToolResult) string {
	switch strings.ToLower(strings.TrimSpace(res.Status)) {
	case "requires_approval":
		var sb strings.Builder
		sb.WriteString("⚠ approval required")
		if strings.TrimSpace(res.Command) != "" {
			sb.WriteString("\n  └ command: " + strings.TrimSpace(res.Command))
		}
		if strings.TrimSpace(res.ApprovalID) != "" {
			sb.WriteString("\n  └ approval_id: " + strings.TrimSpace(res.ApprovalID))
		}
		if strings.TrimSpace(res.ApprovalReason) != "" {
			sb.WriteString("\n  └ reason: " + strings.TrimSpace(res.ApprovalReason))
		}
		if strings.TrimSpace(res.ApprovalID) != "" {
			sb.WriteString("\n  └ action: /approve " + strings.TrimSpace(res.ApprovalID) + "  (or /deny " + strings.TrimSpace(res.ApprovalID) + ")")
		}
		return sb.String()
	case "approved":
		if strings.TrimSpace(res.ApprovalID) == "" {
			return "✓ approved"
		}
		return "✓ approved " + strings.TrimSpace(res.ApprovalID)
	default:
		return ""
	}
}

func toolStartLine(res tools.ToolResult) (prefix string, detail string) {
	switch res.Kind {
	case tools.ToolCommand:
		if strings.TrimSpace(res.Command) == "write_stdin" && strings.TrimSpace(res.SessionID) != "" {
			return "> running", fmt.Sprintf("write_stdin (session=%s)", strings.TrimSpace(res.SessionID))
		}
		return "> running", strings.TrimSpace(res.Command)
	case tools.ToolApplyPatch:
		return "Δ applying", strings.TrimSpace(res.Path)
	case tools.ToolFileRead:
		return "↳ reading", strings.TrimSpace(res.Path)
	case tools.ToolSearch:
		return "🔍 searching", strings.TrimSpace(res.Output)
	default:
		return "• running", strings.TrimSpace(res.Status)
	}
}

func toolCompletedBlock(res tools.ToolResult) string {
	success := res.Error == "" && res.Status != "error"
	icon := "✓"
	state := "completed"
	if !success {
		icon = "✗"
		state = "failed"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %s %s", icon, res.Kind, state))

	// Keep details stable and easy to scan.
	if strings.TrimSpace(res.Command) != "" {
		sb.WriteString("\n  └ command: " + strings.TrimSpace(res.Command))
	}
	if strings.TrimSpace(res.Path) != "" {
		sb.WriteString("\n  └ path: " + strings.TrimSpace(res.Path))
	}
	if strings.TrimSpace(res.SessionID) != "" {
		sb.WriteString("\n  └ session_id: " + strings.TrimSpace(res.SessionID))
	}
	if res.ExitCode != 0 {
		sb.WriteString(fmt.Sprintf("\n  └ exit_code: %d", res.ExitCode))
	}
	if strings.TrimSpace(res.Error) != "" {
		sb.WriteString("\n  └ error: " + strings.TrimSpace(res.Error))
	}

	// file_change wants a diff label, not "output".
	if res.Kind == tools.ToolApplyPatch && strings.TrimSpace(res.Diff) != "" {
		sb.WriteString("\n  └ diff:")
		sb.WriteString(renderIndentedTruncatedLines(res.Diff, maxToolBlockLines))
		return sb.String()
	}

	if strings.TrimSpace(res.Output) != "" {
		sb.WriteString("\n  └ output:")
		sb.WriteString(renderIndentedTruncatedLines(res.Output, maxToolBlockLines))
	}
	return sb.String()
}

func toolBlockWithDiff(head string, diff string) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimRight(head, "\n"))
	sb.WriteString("\n  └ diff:")
	sb.WriteString(renderIndentedTruncatedLines(diff, maxToolBlockLines))
	return sb.String()
}

func renderIndentedTruncatedLines(text string, limit int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	truncated := false
	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
		truncated = true
	}
	var sb strings.Builder
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		sb.WriteString("\n    " + line)
	}
	if truncated {
		sb.WriteString("\n    … (truncated)")
	}
	return sb.String()
}
