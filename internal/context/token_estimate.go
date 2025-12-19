package context

import (
	"encoding/json"

	"echo-cli/internal/agent"
)

// EstimatePromptTokens 对齐 codex 的“粗估”思路：不依赖 tokenizer，使用 bytes/4 启发式。
// codex-rs 在 recompute/estimate 中会走 JSON 序列化后的 bytes 近似计数，因此这里优先 marshal 整体结构。
func EstimatePromptTokens(prompt agent.Prompt) int64 {
	if raw, err := json.Marshal(prompt); err == nil && len(raw) > 0 {
		return int64(ApproxTokenCount(string(raw)))
	}

	// 兜底：按字段粗估，避免序列化失败导致 compaction 永不触发。
	var total int64
	total += estimateMessagesTokens(prompt.Messages)
	total += estimateToolsTokens(prompt.Tools)
	total += int64(ApproxTokenCount(prompt.OutputSchema))
	return total
}

func estimateMessagesTokens(msgs []agent.Message) int64 {
	var total int64
	for _, msg := range msgs {
		total += int64(ApproxTokenCount(msg.Content))
		if msg.ToolUse != nil {
			total += int64(ApproxTokenCount(msg.ToolUse.ID))
			total += int64(ApproxTokenCount(msg.ToolUse.Name))
			total += int64(ApproxTokenCount(string(msg.ToolUse.Input)))
		}
		if msg.ToolResult != nil {
			total += int64(ApproxTokenCount(msg.ToolResult.ToolUseID))
			total += int64(ApproxTokenCount(msg.ToolResult.Content))
		}
	}
	return total
}

func estimateToolsTokens(tools []agent.ToolSpec) int64 {
	if len(tools) == 0 {
		return 0
	}
	raw, err := json.Marshal(tools)
	if err != nil {
		return 0
	}
	return int64(ApproxTokenCount(string(raw)))
}
