package execution

import (
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/i18n"
	"echo-cli/internal/prompts"
)

// Prompt 描述最终发往模型的消息及模型名称。
// 这是 LLM API 调用的完整请求结构，包含模型信息和对话消息列表。
type Prompt struct {
	Model    string          // 要使用的模型名称，如 "gpt-4"、"claude-3" 等
	Messages []agent.Message // 要发送给模型的消息列表，按顺序排列
}

// BuildPrompt 根据 TurnContext 生成模型可消费的提示词消息。
// 这是一个便捷方法，将 TurnContext 结构转换为可以直接发送给 LLM API 的 Prompt 结构。
func (ctx TurnContext) BuildPrompt() Prompt {
	return Prompt{
		Model:    ctx.Model,           // 从上下文中获取指定的模型
		Messages: ctx.BuildMessages(), // 构建完整的消息列表
	}
}

// BuildMessages 按 system → instructions → attachments → history 生成消息，并支持 @internal/prompts 引用。
//
// 消息构建顺序（遵循 Echo Team/Anthropic 标准）：
// 1. System messages（系统消息）：定义模型的角色和行为
//   - Reasoning effort（推理强度设置）
//   - Review mode prompt（审查模式提示词）
//   - System prompt（系统提示词）
//   - Output schema（输出格式要求）
//   - Instructions（指令列表）
//
// 2. Attachments（附件内容）：文件、图片等上下文信息
// 3. History messages（历史消息）：之前的对话记录
//
// 特性：
// - 统一管理所有提示词注入，避免分散处理
// - 支持通过 @internal/prompts 引用预设提示词
// - 自动去重，避免重复添加相同内容
// - 智能合并多条系统消息为一条
// - 正确处理附件内容的注入位置
//
// 参数说明：
//   - ctx: TurnContext 包含构建消息所需的所有上下文信息
//
// 返回值：
//   - []agent.Message: 按正确顺序排列的消息列表
func (ctx TurnContext) BuildMessages() []agent.Message {
	// 计算初始容量：系统消息(最多5条) + 附件 + 历史消息
	capacity := len(ctx.History) + len(ctx.Attachments) + 5
	messages := make([]agent.Message, 0, capacity)

	// 解析指令列表，支持 @internal/prompts 引用
	// 将指令中的引用（如 "@default/tool-use"）解析为实际内容
	instructions := resolveInstructions(ctx.Instructions)

	// 解析系统提示词，同样支持引用
	system := resolveSystemPrompt(ctx.System)

	// 注入默认语言要求，确保所有提示词遵循配置的语言。
	language := i18n.Normalize(ctx.Language)
	if directive := prompts.BuildLanguageInstruction(language); directive != "" && !hasLanguageInstruction(ctx.History, instructions, system) {
		if system == "" {
			system = directive
		} else {
			system = strings.TrimSpace(system + "\n\n" + directive)
		}
	}

	// === 系统消息构建部分 ===

	// 1. 推理强度设置（如 Claude 的 thinking token 配置）
	if reason := prompts.BuildReasoningEffort(ctx.ReasoningEffort); reason != "" && !hasReasoningEffort(ctx.History, instructions, system) {
		messages = append(messages, agent.Message{Role: agent.RoleSystem, Content: reason})
	}

	// 2. 审查模式提示词
	if ctx.ReviewMode {
		messages = append(messages, agent.Message{Role: agent.RoleSystem, Content: prompts.ReviewModeSystemPrompt})
	}

	// 3. 主要系统提示词
	if system != "" {
		messages = append(messages, agent.Message{Role: agent.RoleSystem, Content: system})
	}

	// 4. 输出格式要求（JSON Schema）
	if schema := strings.TrimSpace(ctx.OutputSchema); schema != "" && !hasOutputSchema(ctx.History, instructions) {
		// 将 schema 添加到指令列表末尾，使用特定前缀标识
		instructions = append(instructions, prompts.OutputSchemaPrefix+schema)
	}

	// 5. 所有指令（包括原始指令和可能添加的 schema）
	if len(instructions) > 0 {
		// 使用换行符连接所有指令，并去除首尾空白
		joined := strings.TrimSpace(strings.Join(instructions, "\n"))
		if joined != "" {
			messages = append(messages, agent.Message{
				Role:    agent.RoleSystem, // 所有指令都作为系统消息发送
				Content: joined,           // 合并后的指令内容
			})
		}
	}

	// === 附件内容部分 ===
	// 添加文件、图片等附件内容，为 LLM 提供额外的上下文信息
	if len(ctx.Attachments) > 0 {
		messages = append(messages, ctx.Attachments...)
	}

	// === 历史对话部分 ===
	// 添加纯对话历史，包含用户和助手之间的历史交互
	messages = append(messages, ctx.History...)

	return messages
}

// resolveInstructions 解析指令列表中的每个字符串
// 支持处理 @internal/prompts 引用，并过滤掉空字符串
//
// 参数：
//   - instr: 原始指令字符串列表
//
// 返回：
//   - []string: 解析后的非空指令列表
func resolveInstructions(instr []string) []string {
	// 预分配与输入相同大小的切片，避免多次扩容
	out := make([]string, 0, len(instr))

	// 遍历每个指令项
	for _, item := range instr {
		// 跳过空白项，避免触发默认提示词的回填
		if strings.TrimSpace(item) == "" {
			continue
		}
		// 尝试解析引用（如果有），并过滤空字符串
		if resolved := resolvePromptText(item); resolved != "" {
			out = append(out, resolved)
		}
	}
	return out
}

// resolvePromptText 解析单个提示词文本
// 功能：
// 1. 去除首尾空白字符
// 2. 解析 @internal/prompts 引用
// 3. 过滤空字符串；默认兜底由上层决定（系统提示词会回退到 core_prompt）
//
// 引用格式示例：
//   - "@default/tool-use" → 解析为预定义的工具使用提示词
//   - "普通文本" → 直接返回
//
// 参数：
//   - text: 待解析的文本
//
// 返回：
//   - string: 解析后的文本，如果输入为空或解析失败则返回空字符串
func resolvePromptText(text string) string {
	// 去除首尾空白，规范化输入
	text = strings.TrimSpace(text)

	// 过滤空字符串
	if text == "" {
		return ""
	}

	// 尝试解析引用格式
	// 如果文本以 @ 开头，尝试从 prompts 包中查找对应内容
	if resolved, ok := prompts.ResolveReference(text); ok {
		return strings.TrimSpace(resolved)
	}

	// 对于内置引用格式解析失败的情况，保持空字符串让上层决定兜底逻辑
	if strings.HasPrefix(text, "@internal/prompts/") {
		return ""
	}

	// 如果不是引用格式，直接返回原文本
	return text
}

// resolveSystemPrompt 解析系统提示词，并在缺失或解析失败时回退到核心系统提示词。
func resolveSystemPrompt(text string) string {
	if resolved := resolvePromptText(text); resolved != "" {
		return resolved
	}
	return defaultSystemPrompt()
}

// defaultSystemPrompt 返回内置的核心系统提示词，用作兜底。
func defaultSystemPrompt() string {
	if text, ok := prompts.Builtin(prompts.PromptCore); ok {
		return text
	}
	return ""
}

// hasOutputSchema 检查是否已经包含输出格式定义
// 避免重复添加 JSON Schema，保持消息的简洁性
//
// 检查位置：
// 1. 指令列表中是否已包含 OutputSchemaPrefix 前缀的内容
// 2. 历史消息中是否已包含相同前缀的内容
//
// 参数：
//   - history: 历史对话消息列表
//   - instructions: 待添加的指令列表
//
// 返回：
//   - bool: 如果已包含输出格式定义返回 true，否则返回 false
func hasOutputSchema(history []agent.Message, instructions []string) bool {
	// 首先检查指令列表中是否已包含输出格式定义
	for _, item := range instructions {
		// 使用前缀匹配，检查是否包含输出格式标识
		if strings.HasPrefix(item, prompts.OutputSchemaPrefix) {
			return true
		}
	}

	// 然后检查历史消息中是否已包含输出格式定义
	// 这包括之前轮次已经添加过的格式要求
	for _, msg := range history {
		if strings.HasPrefix(msg.Content, prompts.OutputSchemaPrefix) {
			return true
		}
	}

	// 都没有找到，说明需要添加输出格式定义
	return false
}

// hasReasoningEffort 检查是否已经包含推理强度设置
// 避免重复添加 reasoning effort 配置
//
// 检查位置：
// 1. 系统提示词中
// 2. 指令列表中
// 3. 历史系统消息中（仅检查系统角色）
//
// 参数：
//   - history: 历史对话消息列表
//   - instructions: 待添加的指令列表
//   - system: 系统提示词
//
// 返回：
//   - bool: 如果已包含推理强度设置返回 true，否则返回 false
func hasReasoningEffort(history []agent.Message, instructions []string, system string) bool {
	// 首先检查当前的系统提示词
	if prompts.ExtractReasoningEffort(system) != "" {
		return true
	}

	// 然后检查指令列表
	for _, item := range instructions {
		if prompts.ExtractReasoningEffort(item) != "" {
			return true
		}
	}

	// 最后检查历史消息中的系统消息
	// 注意：只检查系统角色，因为推理强度通常只在系统消息中设置
	for _, msg := range history {
		// 只检查系统消息，忽略用户和助手的消息
		if msg.Role == agent.RoleSystem && prompts.ExtractReasoningEffort(msg.Content) != "" {
			return true
		}
	}

	// 没有找到推理强度设置
	return false
}

// hasLanguageInstruction 检查提示中是否已包含默认语言指令，避免重复注入。
func hasLanguageInstruction(history []agent.Message, instructions []string, system string) bool {
	if prompts.HasLanguageInstruction(system) {
		return true
	}
	for _, item := range instructions {
		if prompts.HasLanguageInstruction(item) {
			return true
		}
	}
	for _, msg := range history {
		if msg.Role == agent.RoleSystem && prompts.HasLanguageInstruction(msg.Content) {
			return true
		}
	}
	return false
}
