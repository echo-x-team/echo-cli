package slash

import "strings"

// Command 表示内置斜杠命令的标识符。
type Command string

// 内置命令集合，对齐 codex SlashCommand 顺序。
const (
	CommandModel    Command = "model"
	CommandSkills   Command = "skills"
	CommandReview   Command = "review"
	CommandNew      Command = "new"
	CommandResume   Command = "resume"
	CommandInit     Command = "init"
	CommandCompact  Command = "compact"
	CommandUndo     Command = "undo"
	CommandDiff     Command = "diff"
	CommandMention  Command = "mention"
	CommandStatus   Command = "status"
	CommandMCP      Command = "mcp"
	CommandLogout   Command = "logout"
	CommandQuit     Command = "quit"
	CommandExit     Command = "exit"
	CommandFeedback Command = "feedback"
	CommandRollout  Command = "rollout"

	// 兼容历史命令（未在 codex 枚举中，但现有 UI 已支持）。
	CommandClear    Command = "clear"
	CommandRun      Command = "run"
	CommandApply    Command = "apply"
	CommandAttach   Command = "attach"
	CommandSessions Command = "sessions"
)

// ItemKind 区分内置命令与自定义 Prompt。
type ItemKind int

const (
	ItemBuiltin ItemKind = iota + 1
	ItemPrompt
)

// Item 代表弹窗中的一行条目。
type Item struct {
	Kind        ItemKind
	Command     Command
	Prompt      *CustomPrompt
	Name        string
	Description string
	DebugOnly   bool
}

// Token 返回无前导斜杠的匹配键。
func (i Item) Token() string {
	switch i.Kind {
	case ItemPrompt:
		if i.Prompt != nil {
			return i.Prompt.Token()
		}
	default:
		if i.Name != "" {
			return i.Name
		}
		if i.Command != "" {
			return string(i.Command)
		}
	}
	return ""
}

// DisplayName 返回带前缀斜杠的展示名称。
func (i Item) DisplayName() string {
	token := i.Token()
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "/") {
		return token
	}
	return "/" + token
}

// CustomPrompt 描述用户保存的自定义 Prompt。
type CustomPrompt struct {
	Name         string
	Description  string
	Prefix       string
	Text         string
	Placeholders PromptPlaceholders
}

// Token 生成 `prefix:name` 形式的匹配键（默认前缀 prompts）。
func (p CustomPrompt) Token() string {
	prefix := p.Prefix
	if strings.TrimSpace(prefix) == "" {
		prefix = "prompts"
	}
	return prefix + ":" + p.Name
}

// PromptPlaceholders 表示自定义 Prompt 的占位符形态。
type PromptPlaceholders struct {
	Named      []string
	Positional int
}

// PlaceholderKind 描述占位符分类。
type PlaceholderKind int

const (
	PlaceholderNone PlaceholderKind = iota
	PlaceholderNamed
	PlaceholderPositional
)

// Kind 返回占位符类型。
func (p PromptPlaceholders) Kind() PlaceholderKind {
	switch {
	case len(p.Named) > 0:
		return PlaceholderNamed
	case p.Positional > 0:
		return PlaceholderPositional
	default:
		return PlaceholderNone
	}
}
