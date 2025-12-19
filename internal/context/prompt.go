package context

import (
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/i18n"
	"echo-cli/internal/prompts"
)

// Prompt 描述最终发往模型的消息及模型名称。
// 这是 LLM API 调用的完整请求结构，包含模型信息和对话消息列表。
type Prompt = agent.Prompt

// BuildPrompt 根据 TurnContext 生成模型可消费的提示词消息。
// 这是一个便捷方法，将 TurnContext 结构转换为可以直接发送给 LLM API 的 Prompt 结构。
func (ctx TurnContext) BuildPrompt() Prompt {
	return Prompt{
		Model:             ctx.Model,
		Messages:          ctx.BuildMessages(),
		Tools:             agent.DefaultTools(),
		ParallelToolCalls: true,
		OutputSchema:      strings.TrimSpace(ctx.OutputSchema),
	}
}

// BuildMessages 按 system → instructions → attachments → history 生成消息，并支持 @internal/prompts 引用。
func (ctx TurnContext) BuildMessages() []agent.Message {
	capacity := len(ctx.History) + len(ctx.Attachments) + 6
	messages := make([]agent.Message, 0, capacity)

	instructions := resolveInstructions(ctx.Instructions)
	instructions = filterLanguagePrompts(instructions)

	system := resolveSystemPrompt(ctx.System)
	if prompts.IsLanguagePrompt(system) {
		system = defaultSystemPrompt()
	}
	languagePrompt := prompts.BuildLanguagePrompt(i18n.Normalize(ctx.Language))

	if reason := prompts.BuildReasoningEffort(ctx.ReasoningEffort); reason != "" && !hasReasoningEffort(ctx.History, instructions, system) {
		messages = append(messages, agent.Message{Role: agent.RoleSystem, Content: reason})
	}
	if ctx.ReviewMode {
		messages = append(messages, agent.Message{Role: agent.RoleSystem, Content: prompts.ReviewModeSystemPrompt})
	}
	if system != "" {
		messages = append(messages, agent.Message{Role: agent.RoleSystem, Content: system})
	}
	if schema := strings.TrimSpace(ctx.OutputSchema); schema != "" && !hasOutputSchema(ctx.History, instructions) {
		instructions = append(instructions, prompts.OutputSchemaPrefix+schema)
	}
	if len(instructions) > 0 {
		joined := strings.TrimSpace(strings.Join(instructions, "\n"))
		if joined != "" {
			messages = append(messages, agent.Message{Role: agent.RoleSystem, Content: joined})
		}
	}
	if len(ctx.Attachments) > 0 {
		messages = append(messages, ctx.Attachments...)
	}
	messages = append(messages, ctx.History...)

	if promptText := strings.TrimSpace(languagePrompt); promptText != "" {
		messages = append(messages, agent.Message{Role: agent.RoleSystem, Content: promptText})
	}
	return messages
}

func resolveInstructions(instr []string) []string {
	out := make([]string, 0, len(instr))
	for _, item := range instr {
		if strings.TrimSpace(item) == "" {
			continue
		}
		if resolved := resolvePromptText(item); resolved != "" {
			out = append(out, resolved)
		}
	}
	return out
}

func resolvePromptText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if resolved, ok := prompts.ResolveReference(text); ok {
		return strings.TrimSpace(resolved)
	}
	if strings.HasPrefix(text, "@internal/prompts/") {
		return ""
	}
	return text
}

func resolveSystemPrompt(text string) string {
	if resolved := resolvePromptText(text); resolved != "" {
		return resolved
	}
	return defaultSystemPrompt()
}

func defaultSystemPrompt() string {
	if text, ok := prompts.Builtin(prompts.PromptCore); ok {
		return text
	}
	return ""
}

func hasOutputSchema(history []agent.Message, instructions []string) bool {
	for _, item := range instructions {
		if strings.HasPrefix(item, prompts.OutputSchemaPrefix) {
			return true
		}
	}
	for _, msg := range history {
		if msg.Role != agent.RoleSystem {
			continue
		}
		if strings.Contains(msg.Content, prompts.OutputSchemaPrefix) {
			return true
		}
	}
	return false
}

func hasReasoningEffort(history []agent.Message, instructions []string, system string) bool {
	if prompts.ExtractReasoningEffort(system) != "" {
		return true
	}
	for _, item := range instructions {
		if prompts.ExtractReasoningEffort(item) != "" {
			return true
		}
	}
	for _, msg := range history {
		if prompts.ExtractReasoningEffort(msg.Content) != "" {
			return true
		}
	}
	return false
}

func filterLanguagePrompts(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if prompts.IsLanguagePrompt(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}
