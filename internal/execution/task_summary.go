package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"echo-cli/internal/events"
	"echo-cli/internal/tools"
)

type taskSummaryAccumulator struct {
	start time.Time

	seenToolCalls map[string]struct{}
	toolFailures  []tools.ToolResult
}

func newTaskSummaryAccumulator(start time.Time) *taskSummaryAccumulator {
	return &taskSummaryAccumulator{
		start:         start,
		seenToolCalls: map[string]struct{}{},
	}
}

func (a *taskSummaryAccumulator) ObserveToolResults(results []tools.ToolResult) {
	if a == nil || len(results) == 0 {
		return
	}
	for _, res := range results {
		if strings.TrimSpace(res.ID) == "" {
			continue
		}
		if _, ok := a.seenToolCalls[res.ID]; ok {
			continue
		}
		a.seenToolCalls[res.ID] = struct{}{}

		if res.Status == "error" || strings.TrimSpace(res.Error) != "" || res.ExitCode != 0 {
			a.toolFailures = append(a.toolFailures, res)
		}
	}
}

func (a *taskSummaryAccumulator) Build(
	submission events.Submission,
	turnCtx TurnContext,
	exitReason string,
	exitStage string,
	finalContent string,
	exitErr error,
) events.TaskSummary {
	duration := time.Since(a.start)
	status := taskSummaryStatus(exitErr)
	inputTokens := countApproxTokensFromInput(submission)
	outputTokens := countApproxTokens(finalContent)

	summaryText := formatTaskSummaryText(taskSummaryTextArgs{
		Status:       status,
		Model:        turnCtx.Model,
		Duration:     duration,
		ToolFailures: a.toolFailures,
		ExitReason:   exitReason,
		ExitStage:    exitStage,
		Err:          exitErr,
	})

	return events.TaskSummary{
		Status:     status,
		Text:       summaryText,
		Error:      errString(exitErr),
		ExitReason: exitReason,
		ExitStage:  exitStage,
		DurationMs: duration.Milliseconds(),
		Model:      turnCtx.Model,

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
	return int64(len(strings.Fields(text)))
}

type taskSummaryTextArgs struct {
	Status   string
	Model    string
	Duration time.Duration

	ToolFailures []tools.ToolResult

	ExitReason string
	ExitStage  string
	Err        error
}

func formatTaskSummaryText(args taskSummaryTextArgs) string {
	if args.Status == "completed" {
		return ""
	}

	var b strings.Builder
	b.WriteString("【任务总结】\n")
	b.WriteString(fmt.Sprintf("状态：%s\n", formatStatusForHuman(args.Status)))
	if strings.TrimSpace(args.Model) != "" {
		b.WriteString(fmt.Sprintf("模型：%s\n", args.Model))
	}
	if args.Duration > 0 {
		b.WriteString(fmt.Sprintf("耗时：%s\n", formatDuration(args.Duration)))
	}

	if stage := strings.TrimSpace(args.ExitStage); stage != "" {
		b.WriteString(fmt.Sprintf("退出阶段：%s\n", stage))
	}
	if reason := strings.TrimSpace(args.ExitReason); reason != "" {
		b.WriteString(fmt.Sprintf("退出原因：%s\n", reason))
	}
	if errText := strings.TrimSpace(errString(args.Err)); errText != "" {
		b.WriteString(fmt.Sprintf("错误：%s\n", truncateOneLine(errText, 240)))
	}

	if len(args.ToolFailures) > 0 {
		b.WriteString("工具失败：\n")
		for i, fail := range args.ToolFailures {
			if i >= 3 {
				b.WriteString(fmt.Sprintf("  - ... 以及另外 %d 个\n", len(args.ToolFailures)-3))
				break
			}
			b.WriteString("  - " + formatToolFailure(fail) + "\n")
		}
	}

	b.WriteString("错误分析：\n")
	b.WriteString("  - " + analyzeFailure(args) + "\n")

	return strings.TrimRight(b.String(), "\n")
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

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	sec := d.Round(100 * time.Millisecond).Seconds()
	return fmt.Sprintf("%.1fs", sec)
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

func analyzeFailure(args taskSummaryTextArgs) string {
	if args.Err == nil {
		return "未知错误（缺少 error 对象）"
	}

	// Prefer stage+reason (aligned with engine log fields).
	stage := strings.TrimSpace(args.ExitStage)
	reason := strings.TrimSpace(args.ExitReason)

	// Provide concise heuristics.
	if errors.Is(args.Err, context.Canceled) {
		return "收到取消信号导致任务提前结束（可能是用户中断/上层取消）"
	}
	if errors.Is(args.Err, context.DeadlineExceeded) {
		return "达到超时限制导致任务提前结束（可能是模型请求或工具执行耗时过长）"
	}

	if stage != "" && reason != "" {
		return fmt.Sprintf("%s 阶段失败（reason=%s）：%s", stage, reason, truncateOneLine(args.Err.Error(), 220))
	}
	if stage != "" {
		return fmt.Sprintf("%s 阶段失败：%s", stage, truncateOneLine(args.Err.Error(), 220))
	}
	return truncateOneLine(args.Err.Error(), 240)
}
