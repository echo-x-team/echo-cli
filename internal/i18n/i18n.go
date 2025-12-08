package i18n

import "strings"

// Language 描述用户希望使用的主要语言。
// 使用简短的语言代码（如 zh、en），便于在配置与提示词中传递。
type Language string

const (
	LanguageChinese Language = "zh"
	LanguageEnglish Language = "en"

	// DefaultLanguage 未配置时的默认语言。
	DefaultLanguage = LanguageChinese
)

// Normalize 将用户输入的语言值转换为统一的语言代码。
// 空字符串或未知值会回退到默认语言。
func Normalize(value string) Language {
	lang := strings.ToLower(strings.TrimSpace(value))
	switch lang {
	case "", "zh", "zh-cn", "zh_cn", "zh-hans", "cn", "chinese", "中文":
		return LanguageChinese
	case "en", "en-us", "en_us", "en-gb", "english":
		return LanguageEnglish
	default:
		if lang == "" {
			return DefaultLanguage
		}
		return Language(lang)
	}
}

// Code 返回规范化后的语言代码，空值回退到默认语言。
func (l Language) Code() string {
	if l == "" {
		return string(DefaultLanguage)
	}
	return string(Normalize(string(l)))
}

// DisplayName 返回适合展示的语言名称。
// 已知语言返回标准名称，未知语言则直接返回原始代码。
func (l Language) DisplayName() string {
	switch Normalize(string(l)) {
	case LanguageChinese:
		return "中文"
	case LanguageEnglish:
		return "English"
	default:
		code := strings.TrimSpace(string(l))
		if code == "" {
			return "中文"
		}
		return code
	}
}
