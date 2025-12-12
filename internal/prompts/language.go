package prompts

import (
	"fmt"
	"strings"

	"echo-cli/internal/i18n"
)

const (
	languagePromptPlaceholder = "{{PREFERRED_LANGUAGE}}"
	languagePromptFallback    = "输出语言指令：保持回答使用中文；如用户明确要求其他语言，按用户要求切换；未提供偏好时默认为中文。"
)

// BuildLanguagePrompt 构造独立的输出语言提示词（默认中文）。
func BuildLanguagePrompt(lang i18n.Language) string {
	templateText, ok := builtinPrompts[PromptLanguage]
	preferred := i18n.Normalize(lang.Code())
	display := strings.TrimSpace(preferred.DisplayName())
	if display == "" {
		display = i18n.LanguageChinese.DisplayName()
	}
	if ok {
		rendered := strings.TrimSpace(strings.ReplaceAll(templateText, languagePromptPlaceholder, display))
		if rendered != "" {
			return rendered
		}
	}
	return fallbackLanguagePrompt(display)
}

// IsLanguagePrompt 判断文本是否为输出语言提示词。
func IsLanguagePrompt(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	return strings.Contains(strings.ToLower(text), "输出语言指令")
}

func fallbackLanguagePrompt(display string) string {
	if strings.TrimSpace(display) == "" {
		display = i18n.LanguageChinese.DisplayName()
	}
	return fmt.Sprintf("输出语言指令：保持回答使用%s；如用户明确要求其他语言，按用户要求切换；未提供偏好时默认为中文。", display)
}
