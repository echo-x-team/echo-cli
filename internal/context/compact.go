package context

import (
	stdcontext "context"
	"errors"
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/prompts"
)

const compactUserMessageMaxTokens = 20_000

var ErrContextWindowTooSmallForCompaction = errors.New("context window too small for compaction")

// CompactConversationHistory 对齐 codex 的 inline compaction：
// 1) 用 compact prompt 让模型生成“交接摘要”
// 2) 将历史重建为：最近若干 user 消息 + summary（作为 user 消息注入）
// 3) 当 compact prompt 超出窗口时，从最旧处裁剪以尽量保留 prefix cache 与最近消息
func CompactConversationHistory(
	ctx stdcontext.Context,
	client agent.ModelClient,
	turn TurnContext,
	historyItems []ResponseItem,
) (newHistory []ResponseItem, trimmedOlderItems int, summaryText string, err error) {
	compactPrompt, ok := prompts.Builtin(prompts.PromptCompact)
	if !ok {
		return nil, 0, "", errors.New("missing builtin compact prompt")
	}
	summaryPrefix, ok := prompts.Builtin(prompts.PromptCompactSummaryPrefix)
	if !ok {
		return nil, 0, "", errors.New("missing builtin compact summary prefix")
	}

	window, hasWindow := ContextWindowForModel(turn.Model)

	itemsForPrompt := make([]ResponseItem, len(historyItems))
	copy(itemsForPrompt, historyItems)
	normalizeHistory(&itemsForPrompt)

	for {
		historyMsgs := ResponseItemsToAgentMessages(itemsForPrompt)
		compactTurn := turn
		compactTurn.History = historyMsgs

		baseMessages := compactTurn.BuildMessages()
		messages := append(baseMessages, agent.Message{
			Role:    agent.RoleUser,
			Content: compactPrompt,
		})

		prompt := agent.Prompt{
			Model:             turn.Model,
			Messages:          messages,
			Tools:             nil,
			ParallelToolCalls: false,
			OutputSchema:      "",
		}

		if hasWindow {
			estimated := EstimatePromptTokens(prompt)
			if int64(window) > 0 && estimated > window {
				if len(itemsForPrompt) <= 1 {
					return nil, trimmedOlderItems, "", ErrContextWindowTooSmallForCompaction
				}
				itemsForPrompt = RemoveFirstItem(itemsForPrompt)
				trimmedOlderItems++
				continue
			}
		}

		summarySuffix, callErr := client.Complete(ctx, prompt)
		if callErr != nil {
			return nil, trimmedOlderItems, "", callErr
		}
		summarySuffix = strings.TrimSpace(summarySuffix)
		summaryText = strings.TrimSpace(summaryPrefix) + "\n" + summarySuffix

		ghost := findLastGhostSnapshot(itemsForPrompt)
		initialContext := collectSessionPrefixItems(itemsForPrompt)
		userMessages := collectUserMessages(itemsForPrompt, summaryPrefix)
		newHistory = buildCompactedHistory(initialContext, userMessages, summaryText, compactUserMessageMaxTokens)
		if ghost != nil {
			newHistory = append(newHistory, *ghost)
		}
		return newHistory, trimmedOlderItems, summaryText, nil
	}
}

func findLastGhostSnapshot(items []ResponseItem) *ResponseItem {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Type != ResponseItemTypeGhostSnapshot || items[i].GhostSnapshot == nil {
			continue
		}
		cp := items[i]
		return &cp
	}
	return nil
}

func isSummaryMessage(message string, summaryPrefix string) bool {
	prefix := strings.TrimSpace(summaryPrefix)
	if prefix == "" {
		return false
	}
	return strings.HasPrefix(message, prefix+"\n")
}

const userInstructionsPrefix = "# AGENTS.md instructions for "

func isSessionPrefixMessage(text string) bool {
	trimmed := strings.TrimLeft(text, " \t\r\n")
	lowered := strings.ToLower(trimmed)
	if strings.HasPrefix(lowered, "<environment_context>") {
		return true
	}
	if strings.HasPrefix(trimmed, userInstructionsPrefix) {
		return true
	}
	if strings.HasPrefix(lowered, "<user_shell_command>") {
		return true
	}
	return false
}

func collectUserMessages(items []ResponseItem, summaryPrefix string) []string {
	out := make([]string, 0, 16)
	for _, item := range items {
		if item.Type != ResponseItemTypeMessage || item.Message == nil {
			continue
		}
		if strings.TrimSpace(item.Message.Role) != "user" {
			continue
		}
		text := FlattenContentItems(item.Message.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if isSessionPrefixMessage(text) {
			continue
		}
		if isSummaryMessage(text, summaryPrefix) {
			continue
		}
		out = append(out, text)
	}
	return out
}

func collectSessionPrefixItems(items []ResponseItem) []ResponseItem {
	out := make([]ResponseItem, 0, 8)
	for _, item := range items {
		if item.Type != ResponseItemTypeMessage || item.Message == nil {
			continue
		}
		if strings.TrimSpace(item.Message.Role) != "user" {
			continue
		}
		text := FlattenContentItems(item.Message.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if !isSessionPrefixMessage(text) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func buildCompactedHistory(initialContext []ResponseItem, userMessages []string, summaryText string, maxTokens int) []ResponseItem {
	selected := make([]string, 0, len(userMessages))
	if maxTokens > 0 {
		remaining := maxTokens
		for i := len(userMessages) - 1; i >= 0; i-- {
			if remaining == 0 {
				break
			}
			msg := userMessages[i]
			tokens := ApproxTokenCount(msg)
			if tokens <= remaining {
				selected = append(selected, msg)
				remaining -= tokens
				continue
			}
			truncated := TruncateText(msg, TokensPolicy(remaining))
			selected = append(selected, truncated)
			break
		}
		for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
			selected[i], selected[j] = selected[j], selected[i]
		}
	}

	out := make([]ResponseItem, 0, len(initialContext)+len(selected)+1)
	out = append(out, initialContext...)
	for _, msg := range selected {
		out = append(out, ResponseItem{
			Type: ResponseItemTypeMessage,
			Message: &MessageResponseItem{
				Role: "user",
				Content: []ContentItem{
					{Type: ContentItemInputText, Text: msg},
				},
			},
		})
	}

	if strings.TrimSpace(summaryText) == "" {
		summaryText = "(no summary available)"
	}
	out = append(out, ResponseItem{
		Type: ResponseItemTypeMessage,
		Message: &MessageResponseItem{
			Role: "user",
			Content: []ContentItem{
				{Type: ContentItemInputText, Text: summaryText},
			},
		},
	})
	return out
}
