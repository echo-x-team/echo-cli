package prompts

import "strings"

const (
	// OutputSchemaPrefix 用于提示模型遵循给定的输出模式。
	OutputSchemaPrefix = "输出模式：\n"

	reasoningEffortPrefix = "推理强度："
	legacyReasoningPrefix = "Reasoning effort:"
)

// ReviewModeSystemPrompt 是代码审查模式下的系统提示词，由内置中文审查提示词提供。
var ReviewModeSystemPrompt = builtinPrompts[PromptReview]

// BuildReasoningEffort 构造推理强度提示词，空值返回空串。
func BuildReasoningEffort(effort string) string {
	trimmed := strings.TrimSpace(effort)
	if trimmed == "" {
		return ""
	}
	return reasoningEffortPrefix + trimmed
}

// ExtractReasoningEffort 从指令文本中提取推理强度配置，兼容旧的英文前缀。
func ExtractReasoningEffort(text string) string {
	if text == "" {
		return ""
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, reasoningEffortPrefix):
			return strings.TrimSpace(strings.TrimPrefix(line, reasoningEffortPrefix))
		case strings.HasPrefix(strings.ToLower(line), strings.ToLower(legacyReasoningPrefix)):
			return strings.TrimSpace(strings.TrimPrefix(line, legacyReasoningPrefix))
		}
	}
	return ""
}

const internalPrefix = "@internal/prompts/"

// ResolveReference 将 @internal/prompts/<name> 形式的引用展开为对应的内置提示词文本。
func ResolveReference(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, internalPrefix) {
		return "", false
	}
	name := strings.TrimPrefix(ref, internalPrefix)
	if name == "" {
		return "", false
	}
	return Builtin(Name(name))
}
