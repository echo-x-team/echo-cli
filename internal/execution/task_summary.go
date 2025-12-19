package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	echocontext "echo-cli/internal/context"
	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

type turnSummaryInput struct {
	Submission   events.Submission
	TurnCtx      echocontext.TurnContext
	Start        time.Time
	FinalContent string
	ToolResults  []tools.ToolResult
	ExitReason   string
	ExitStage    string
	Err          error
}

func buildTurnSummary(args turnSummaryInput) events.TaskSummary {
	duration := time.Duration(0)
	if !args.Start.IsZero() {
		duration = time.Since(args.Start)
	}
	status := taskSummaryStatus(args.Err)
	inputTokens := countApproxTokensFromInput(args.Submission)
	outputTokens := countApproxTokens(args.FinalContent)

	summaryText := formatTurnSummaryText(turnSummaryTextArgs{
		Status:       status,
		FinalContent: args.FinalContent,
		ToolResults:  args.ToolResults,
		ExitReason:   args.ExitReason,
		ExitStage:    args.ExitStage,
		Err:          args.Err,
	})

	return events.TaskSummary{
		Status:     status,
		Text:       summaryText,
		Error:      errString(args.Err),
		ExitReason: args.ExitReason,
		ExitStage:  args.ExitStage,
		DurationMs: duration.Milliseconds(),
		Model:      args.TurnCtx.Model,

		InputTokens:       inputTokens,
		CachedInputTokens: 0,
		OutputTokens:      outputTokens,
	}
}

func taskSummaryStatus(err error) string {
	if err == nil {
		return "completed"
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// Best-effort classification; the summary can be more specific in text.
		if errors.Is(err, context.Canceled) {
			return "interrupted"
		}
		return "timeout"
	default:
		return "failed"
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func countApproxTokensFromInput(submission events.Submission) int64 {
	if submission.Operation.UserInput == nil {
		return 0
	}
	var total int64
	for _, item := range submission.Operation.UserInput.Items {
		if item.Role != "user" {
			continue
		}
		total += countApproxTokens(item.Content)
	}
	return total
}

func countApproxTokens(text string) int64 {
	return int64(echocontext.ApproxTokenCount(text))
}

type turnSummaryTextArgs struct {
	Status       string
	FinalContent string
	ToolResults  []tools.ToolResult
	ExitReason   string
	ExitStage    string
	Err          error
}

const summaryListLimit = 4

func formatTurnSummaryText(args turnSummaryTextArgs) string {
	completed, issues := buildTurnSummaryItems(args)

	var b strings.Builder
	b.WriteString("【本轮总结】\n")
	if args.Status != "" && args.Status != "completed" {
		b.WriteString(fmt.Sprintf("状态：%s\n", formatStatusForHuman(args.Status)))
	}
	b.WriteString("完成：\n")
	appendSummaryList(&b, completed, summaryListLimit)
	b.WriteString("问题：\n")
	appendSummaryList(&b, issues, summaryListLimit)

	return strings.TrimRight(b.String(), "\n")
}

func buildTurnSummaryItems(args turnSummaryTextArgs) ([]string, []string) {
	var completed []string
	var issues []string

	finalText := strings.TrimSpace(args.FinalContent)
	if finalText != "" {
		completed = append(completed, fmt.Sprintf("输出回复（%d 字）", utf8.RuneCountInString(finalText)))
	}

	for _, res := range args.ToolResults {
		if isToolFailure(res) {
			issues = append(issues, formatToolFailure(res))
			continue
		}
		if item := formatToolSuccess(res); item != "" {
			completed = append(completed, item)
		}
	}

	if args.Err != nil {
		issues = append(issues, analyzeFailure(args.ExitStage, args.ExitReason, args.Err))
	}

	return completed, issues
}

func appendSummaryList(b *strings.Builder, items []string, limit int) {
	if b == nil {
		return
	}
	if len(items) == 0 {
		b.WriteString("- 无\n")
		return
	}
	if limit <= 0 {
		limit = len(items)
	}
	for i, item := range items {
		if i >= limit {
			b.WriteString(fmt.Sprintf("- ... 以及另外 %d 项\n", len(items)-limit))
			break
		}
		line := strings.TrimSpace(item)
		if line == "" {
			continue
		}
		b.WriteString("- " + line + "\n")
	}
}

func formatStatusForHuman(status string) string {
	switch status {
	case "completed":
		return "完成"
	case "failed":
		return "失败"
	case "interrupted":
		return "中断"
	case "timeout":
		return "超时"
	default:
		return status
	}
}

func isToolFailure(res tools.ToolResult) bool {
	if res.Status == "error" {
		return true
	}
	if strings.TrimSpace(res.Error) != "" {
		return true
	}
	return res.ExitCode != 0
}

func formatToolSuccess(res tools.ToolResult) string {
	switch res.Kind {
	case tools.ToolCommand:
		cmd := strings.TrimSpace(res.Command)
		if cmd == "" {
			cmd = "<unknown>"
		}
		cmd = truncateOneLine(cmd, 160)
		if sess := strings.TrimSpace(res.SessionID); sess != "" {
			return fmt.Sprintf("执行命令：`%s`（session=%s）", cmd, sess)
		}
		if res.ExitCode != 0 {
			return fmt.Sprintf("执行命令：`%s`（exit=%d）", cmd, res.ExitCode)
		}
		return fmt.Sprintf("执行命令：`%s`", cmd)
	case tools.ToolApplyPatch:
		path := strings.TrimSpace(res.Path)
		if path == "" {
			path = "<unknown>"
		}
		return fmt.Sprintf("修改文件：`%s`", path)
	case tools.ToolFileRead:
		path := strings.TrimSpace(res.Path)
		if path == "" {
			path = "<unknown>"
		}
		return fmt.Sprintf("读取文件：`%s`", path)
	case tools.ToolSearch:
		count := countOutputLines(res.Output)
		if count > 0 {
			return fmt.Sprintf("扫描文件（%d 条）", count)
		}
		return "扫描文件"
	case tools.ToolPlanUpdate:
		if count := len(res.Plan); count > 0 {
			return fmt.Sprintf("更新计划（%d 项）", count)
		}
		return "更新计划"
	default:
		if res.Kind == "" {
			return "执行工具"
		}
		return fmt.Sprintf("执行工具：%s", res.Kind)
	}
}

func countOutputLines(output string) int {
	output = strings.TrimSpace(output)
	if output == "" {
		return 0
	}
	return strings.Count(output, "\n") + 1
}

func truncateOneLine(text string, max int) string {
	text = strings.Join(strings.Fields(text), " ")
	if max <= 0 || len(text) <= max {
		return text
	}
	if max < 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func formatCommandSummary(res tools.ToolResult) string {
	cmd := strings.TrimSpace(res.Command)
	if cmd == "" {
		cmd = "<unknown>"
	}
	if strings.TrimSpace(res.SessionID) != "" {
		return fmt.Sprintf("`%s` 运行中（session=%s）", cmd, strings.TrimSpace(res.SessionID))
	}
	if res.Status == "error" || strings.TrimSpace(res.Error) != "" || res.ExitCode != 0 {
		return fmt.Sprintf("`%s`（exit=%d）失败：%s", cmd, res.ExitCode, truncateOneLine(res.Error, 160))
	}
	return fmt.Sprintf("`%s`（exit=%d）成功", cmd, res.ExitCode)
}

func formatFileChangeSummary(res tools.ToolResult) string {
	path := strings.TrimSpace(res.Path)
	if path == "" {
		path = "<unknown>"
	}
	if res.Status == "error" || strings.TrimSpace(res.Error) != "" {
		return fmt.Sprintf("`%s` 失败：%s", path, truncateOneLine(res.Error, 160))
	}
	return fmt.Sprintf("`%s` 成功", path)
}

func formatToolFailure(res tools.ToolResult) string {
	switch res.Kind {
	case tools.ToolCommand:
		return formatCommandSummary(res)
	case tools.ToolApplyPatch:
		return formatFileChangeSummary(res)
	default:
		msg := strings.TrimSpace(res.Error)
		if msg == "" {
			msg = "unknown error"
		}
		return fmt.Sprintf("%s 失败：%s", res.Kind, truncateOneLine(msg, 160))
	}
}

func analyzeFailure(exitStage, exitReason string, err error) string {
	if err == nil {
		return "未知错误（缺少 error 对象）"
	}

	// Prefer stage+reason (aligned with engine log fields).
	stage := strings.TrimSpace(exitStage)
	reason := strings.TrimSpace(exitReason)

	// Provide concise heuristics.
	if errors.Is(err, context.Canceled) {
		return "收到取消信号导致本轮提前结束（可能是用户中断/上层取消）"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "达到超时限制导致本轮提前结束（可能是模型请求或工具执行耗时过长）"
	}

	if stage != "" && reason != "" {
		return fmt.Sprintf("%s 阶段失败（reason=%s）：%s", stage, reason, truncateOneLine(err.Error(), 220))
	}
	if stage != "" {
		return fmt.Sprintf("%s 阶段失败：%s", stage, truncateOneLine(err.Error(), 220))
	}
	return truncateOneLine(err.Error(), 240)
}
