package prompts

import (
	"strings"

	"echo-cli/internal/i18n"
)

const (
	// OutputSchemaPrefix 用于提示模型遵循给定的输出模式。
	OutputSchemaPrefix = "输出模式：\n"

	reasoningEffortPrefix = "推理强度："
	legacyReasoningPrefix = "Reasoning effort:"

	languagePrefix       = "默认语言："
	legacyLanguagePrefix = "Default language:"
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

// BuildLanguageInstruction 构造默认语言指令，未指定语言时回退到中文。
func BuildLanguageInstruction(lang i18n.Language) string {
	resolved := i18n.Normalize(lang.Code())
	switch resolved {
	case i18n.LanguageChinese:
		return languagePrefix + "中文。使用该语言回复，若用户指定其他语言则优先按照用户的选择。"
	default:
		name := resolved.DisplayName()
		return legacyLanguagePrefix + " " + name + ". Respond in this language unless the user explicitly requests another."
	}
}

// HasLanguageInstruction 检查文本是否包含默认语言指令前缀。
func HasLanguageInstruction(text string) bool {
	if text == "" {
		return false
	}
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	return strings.Contains(trimmed, languagePrefix) || strings.Contains(lower, strings.ToLower(legacyLanguagePrefix))
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
