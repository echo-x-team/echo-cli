package prompts

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed text/*
var builtinFS embed.FS

// Name 表示内置提示词的唯一标识。
type Name string

const (
	PromptCore                     Name = "core"
	PromptLanguage                 Name = "language"
	PromptGPT5Echo                 Name = "gpt-5-echo"
	PromptGPT51                    Name = "gpt-5.1"
	PromptGPT51EchoMax             Name = "gpt-5.1-echo-max"
	PromptReview                   Name = "review"
	PromptCompact                  Name = "compact"
	PromptCompactSummaryPrefix     Name = "compact-summary-prefix"
	PromptParallelInstructions     Name = "parallel-instructions"
	PromptReviewHistoryCompleted   Name = "review-history-completed"
	PromptReviewHistoryInterrupted Name = "review-history-interrupted"
	PromptReviewExitSuccess        Name = "review-exit-success"
	PromptReviewExitInterrupted    Name = "review-exit-interrupted"
	PromptInitCommand              Name = "init-command"
	PromptIssueDeduplicator        Name = "issue-deduplicator"
	PromptIssueLabeler             Name = "issue-labeler"
)

var builtinFiles = map[Name]string{
	PromptCore:                     "text/core_prompt.md",
	PromptLanguage:                 "text/language_prompt.md",
	PromptGPT5Echo:                 "text/gpt5_echo_prompt.md",
	PromptGPT51:                    "text/gpt5_1_prompt.md",
	PromptGPT51EchoMax:             "text/gpt5_1_echo_max_prompt.md",
	PromptReview:                   "text/review_prompt.md",
	PromptCompact:                  "text/compact_prompt.md",
	PromptCompactSummaryPrefix:     "text/compact_summary_prefix.md",
	PromptParallelInstructions:     "text/parallel_instructions.md",
	PromptReviewHistoryCompleted:   "text/review_history_completed.md",
	PromptReviewHistoryInterrupted: "text/review_history_interrupted.md",
	PromptReviewExitSuccess:        "text/review_exit_success.xml",
	PromptReviewExitInterrupted:    "text/review_exit_interrupted.xml",
	PromptInitCommand:              "text/init_command_prompt.md",
	PromptIssueDeduplicator:        "text/issue_deduplicator.txt",
	PromptIssueLabeler:             "text/issue_labeler.txt",
}

var builtinPrompts = func() map[Name]string {
	out := make(map[Name]string, len(builtinFiles))
	for name, path := range builtinFiles {
		data, err := builtinFS.ReadFile(path)
		if err != nil {
			panic(fmt.Sprintf("load builtin prompt %q from %s: %v", name, path, err))
		}
		out[name] = strings.TrimSpace(string(data))
	}
	return out
}()

// Builtin 返回指定名称的内置提示词文本。
func Builtin(name Name) (string, bool) {
	text, ok := builtinPrompts[name]
	return text, ok
}

// Builtins 返回内置提示词的拷贝，便于统一管理与调试。
func Builtins() map[Name]string {
	out := make(map[Name]string, len(builtinPrompts))
	for k, v := range builtinPrompts {
		out[k] = v
	}
	return out
}
