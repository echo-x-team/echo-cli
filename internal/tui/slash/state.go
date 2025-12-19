package slash

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/sahilm/fuzzy"
)

// Options 控制 Slash 弹窗数据来源与过滤。
type Options struct {
	CustomPrompts   []CustomPrompt
	SkillsAvailable bool
	Debug           bool
	MaxLines        int
}

// Input 表示当前文本与光标状态。
type Input struct {
	Value        string
	CursorLine   int
	CursorColumn int
	Blocked      bool // 其他弹窗（@/$）已占用时屏蔽 slash
}

// ActionKind 描述按键触发后的处理类型。
type ActionKind int

const (
	ActionNone ActionKind = iota
	ActionClose
	ActionInsert
	ActionSubmitCommand
	ActionSubmitPrompt
	ActionError
)

// Action 汇总 Slash 处理结果。
type Action struct {
	Kind         ActionKind
	Command      Command
	Prompt       *CustomPrompt
	NewValue     string
	CursorColumn int
	SubmitText   string
	Args         string
	Message      string
}

// State 维护 slash 弹窗的匹配与选择状态。
type State struct {
	options  Options
	items    []Item
	matches  []match
	selected int
	open     bool
	input    parsedInput
	maxLines int
}

type match struct {
	item       Item
	highlights []int
	score      int
}

type parsedInput struct {
	firstLine string
	rest      string
	token     tokenInfo
	cursor    int
}

type tokenInfo struct {
	found   bool
	active  bool
	blocked bool
	value   string
	start   int
	end     int
	args    string
}

// NewState 构造 slash 状态机。
func NewState(opts Options) *State {
	maxLines := opts.MaxLines
	if maxLines <= 0 {
		maxLines = 8
	}
	items := builtinItems(opts)
	if len(opts.CustomPrompts) > 0 {
		items = append(items, promptItems(opts.CustomPrompts)...)
	}
	return &State{
		options:  opts,
		items:    items,
		open:     false,
		maxLines: maxLines,
	}
}

// Open 返回弹窗是否展示。
func (s *State) Open() bool {
	return s != nil && s.open
}

// SyncInput 根据最新文本同步过滤列表与选中项。
func (s *State) SyncInput(in Input) {
	if s == nil {
		return
	}
	s.input = parseInput(in)
	if !s.input.token.found || s.input.token.blocked {
		s.open = false
		s.matches = nil
		return
	}
	if s.input.token.active && !in.Blocked && in.CursorLine == 0 {
		s.open = true
	} else {
		s.open = false
	}
	if !s.open {
		s.matches = nil
		return
	}
	s.matches = filterMatches(s.items, s.input.token.value)
	if len(s.matches) == 0 {
		s.selected = 0
		return
	}
	if s.selected >= len(s.matches) {
		s.selected = 0
	}
}

// ResolveSubmit 按 Enter 行为解析当前输入，不依赖弹窗是否打开。
func (s *State) ResolveSubmit(value string) Action {
	p := parseInput(Input{
		Value:        value,
		CursorLine:   0,
		CursorColumn: runeLen(firstLine(value)),
	})
	if !p.token.found || p.token.value == "" {
		return Action{Kind: ActionNone}
	}
	item, ok := s.findExactItem(p.token.value)
	if !ok {
		return Action{Kind: ActionError, Message: "不认识的命令，请输入 / 查看列表"}
	}
	return s.actionForItem(item, TriggerEnter, p)
}

// HandleKey 处理键盘事件，返回对应动作。
func (s *State) HandleKey(msg string) (Action, bool) {
	if s == nil || !s.open {
		return Action{}, false
	}
	switch msg {
	case "up", "ctrl+p":
		if len(s.matches) == 0 {
			return Action{Kind: ActionClose}, true
		}
		s.selected--
		if s.selected < 0 {
			s.selected = len(s.matches) - 1
		}
		return Action{Kind: ActionNone}, true
	case "down", "ctrl+n":
		if len(s.matches) == 0 {
			return Action{Kind: ActionClose}, true
		}
		s.selected++
		if s.selected >= len(s.matches) {
			s.selected = 0
		}
		return Action{Kind: ActionNone}, true
	case "esc":
		s.open = false
		return Action{Kind: ActionClose}, true
	case "tab", "enter":
		if len(s.matches) == 0 {
			return Action{Kind: ActionError, Message: "不认识的命令，请输入 / 查看列表"}, true
		}
		trigger := TriggerTab
		if msg == "enter" {
			trigger = TriggerEnter
		}
		item := s.matches[s.selected].item
		act := s.actionForItem(item, trigger, s.input)
		if act.Kind == ActionSubmitCommand || act.Kind == ActionSubmitPrompt {
			s.open = false
		}
		return act, true
	default:
		return Action{}, false
	}
}

type Trigger int

const (
	TriggerTab Trigger = iota + 1
	TriggerEnter
)

func (s *State) actionForItem(item Item, trigger Trigger, input parsedInput) Action {
	switch item.Kind {
	case ItemPrompt:
		if item.Prompt == nil {
			return Action{Kind: ActionNone}
		}
		return s.handlePromptAction(*item.Prompt, trigger, input)
	default:
		return s.handleCommandAction(item.Command, trigger, input)
	}
}

func (s *State) handleCommandAction(cmd Command, trigger Trigger, input parsedInput) Action {
	switch trigger {
	case TriggerTab:
		// /skills 直接派发命令
		if cmd == CommandSkills {
			return Action{Kind: ActionSubmitCommand, Command: cmd, Args: input.token.args}
		}
		return Action{
			Kind:         ActionInsert,
			NewValue:     buildCommandValue(cmd, input),
			CursorColumn: runeLen("/"+string(cmd)) + 1,
		}
	case TriggerEnter:
		return Action{Kind: ActionSubmitCommand, Command: cmd, Args: input.token.args}
	default:
		return Action{Kind: ActionNone}
	}
}

func (s *State) handlePromptAction(prompt CustomPrompt, trigger Trigger, input parsedInput) Action {
	args := input.token.args
	switch trigger {
	case TriggerTab:
		val, cursor := buildPromptValue(prompt, input)
		return Action{Kind: ActionInsert, NewValue: val, CursorColumn: cursor, Args: args}
	case TriggerEnter:
		if placeholdersSatisfied(prompt.Placeholders, args) {
			text := expandPrompt(prompt, args)
			return Action{Kind: ActionSubmitPrompt, Prompt: &prompt, SubmitText: text, Args: args}
		}
		val, cursor := buildPromptValue(prompt, input)
		return Action{Kind: ActionInsert, NewValue: val, CursorColumn: cursor, Args: args}
	default:
		return Action{Kind: ActionNone}
	}
}

func (s *State) findExactItem(token string) (Item, bool) {
	for _, item := range s.items {
		if strings.EqualFold(item.Token(), token) {
			return item, true
		}
	}
	return Item{}, false
}

func filterMatches(items []Item, query string) []match {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		matches := make([]match, 0, len(items))
		for _, item := range items {
			matches = append(matches, match{item: item})
		}
		return matches
	}

	candidates, keys := buildCandidates(items)
	results := fuzzy.Find(strings.ToLower(trimmed), keys)
	seen := map[int]bool{}
	matches := make([]match, 0, len(results))
	for _, res := range results {
		cand := candidates[res.Index]
		if seen[cand.itemIdx] {
			continue
		}
		seen[cand.itemIdx] = true
		item := items[cand.itemIdx]
		matches = append(matches, match{
			item:       item,
			highlights: adjustHighlights(res.MatchedIndexes, highlightOffset(item.Token(), cand.key)),
			score:      res.Score,
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].item.Token() < matches[j].item.Token()
		}
		return matches[i].score > matches[j].score
	})
	return matches
}

type candidate struct {
	itemIdx int
	key     string
}

func buildCandidates(items []Item) ([]candidate, []string) {
	candidates := make([]candidate, 0, len(items)*2)
	keys := make([]string, 0, len(items)*2)
	for idx, item := range items {
		token := strings.ToLower(item.Token())
		if token == "" {
			continue
		}
		candidates = append(candidates, candidate{itemIdx: idx, key: token})
		keys = append(keys, token)
		// 自定义 Prompt 允许按去掉前缀的 name 匹配
		if item.Kind == ItemPrompt && strings.Contains(token, ":") {
			parts := strings.SplitN(token, ":", 2)
			if len(parts) == 2 && parts[1] != "" {
				candidates = append(candidates, candidate{itemIdx: idx, key: parts[1]})
				keys = append(keys, parts[1])
			}
		}
	}
	return candidates, keys
}

func highlightOffset(token string, key string) int {
	if key == "" {
		return 0
	}
	token = strings.ToLower(token)
	key = strings.ToLower(key)
	if strings.HasSuffix(token, key) {
		return runeLen(token) - runeLen(key)
	}
	return 0
}

func adjustHighlights(indexes []int, offset int) []int {
	if offset == 0 || len(indexes) == 0 {
		return indexes
	}
	out := make([]int, len(indexes))
	for i, idx := range indexes {
		out[i] = idx + offset
	}
	return out
}

func buildCommandValue(cmd Command, input parsedInput) string {
	token := "/" + string(cmd)
	args := strings.TrimSpace(input.token.args)
	if args != "" {
		return token + " " + args + input.rest
	}
	return token + " " + input.rest
}

func buildPromptValue(prompt CustomPrompt, input parsedInput) (string, int) {
	token := "/" + prompt.Token()
	args := strings.TrimSpace(input.token.args)
	placeholder := ""
	cursor := runeLen(token)
	switch prompt.Placeholders.Kind() {
	case PlaceholderNamed:
		if len(prompt.Placeholders.Named) > 0 {
			placeholderParts := make([]string, 0, len(prompt.Placeholders.Named))
			for _, name := range prompt.Placeholders.Named {
				placeholderParts = append(placeholderParts, name+`=""`)
			}
			placeholder = strings.Join(placeholderParts, " ")
			cursor = runeLen(token) + 1 + runeLen(prompt.Placeholders.Named[0]) + 2 // cursor inside ""
		}
	case PlaceholderPositional:
		placeholder = ""
		cursor = runeLen(token) + 1
	default:
		cursor = runeLen(token)
	}
	if args != "" {
		value := token + " " + args + input.rest
		pos := strings.Index(args, `=""`)
		if pos >= 0 {
			cursor = runeLen(token) + 1 + runeLenBefore(args, pos) + 1
		} else {
			cursor = runeLen(token) + 1 + runeLen(args)
		}
		return value, cursor
	}
	if placeholder != "" {
		return token + " " + placeholder + input.rest, cursor
	}
	if prompt.Placeholders.Kind() == PlaceholderPositional {
		return token + " " + input.rest, cursor
	}
	return token + input.rest, cursor
}

func placeholdersSatisfied(placeholders PromptPlaceholders, args string) bool {
	switch placeholders.Kind() {
	case PlaceholderNamed:
		if len(placeholders.Named) == 0 {
			return strings.TrimSpace(args) != ""
		}
		assignments := parseAssignments(args)
		for _, name := range placeholders.Named {
			val, ok := assignments[name]
			if !ok || strings.TrimSpace(val) == "" {
				return false
			}
		}
		return true
	case PlaceholderPositional:
		if placeholders.Positional <= 0 {
			return strings.TrimSpace(args) != ""
		}
		return len(strings.Fields(args)) >= placeholders.Positional
	default:
		return true
	}
}

func expandPrompt(prompt CustomPrompt, args string) string {
	text := prompt.Text
	if strings.TrimSpace(text) == "" {
		trimmed := strings.TrimSpace(args)
		if trimmed != "" {
			return trimmed
		}
		return prompt.Token()
	}
	switch prompt.Placeholders.Kind() {
	case PlaceholderNamed:
		assignments := parseAssignments(args)
		for name, val := range assignments {
			placeholder := "{{" + name + "}}"
			text = strings.ReplaceAll(text, placeholder, val)
		}
	case PlaceholderPositional:
		values := strings.Fields(args)
		for i, val := range values {
			placeholder := "{{" + strconv.Itoa(i+1) + "}}"
			text = strings.ReplaceAll(text, placeholder, val)
		}
	}
	return text
}

func parseInput(in Input) parsedInput {
	first, rest := splitFirstLine(in.Value)
	runes := []rune(first)
	token := locateToken(runes, in.CursorColumn)
	return parsedInput{
		firstLine: first,
		rest:      rest,
		token:     token,
		cursor:    in.CursorColumn,
	}
}

func splitFirstLine(value string) (string, string) {
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		return value[:idx], value[idx:]
	}
	return value, ""
}

func firstLine(value string) string {
	line, _ := splitFirstLine(value)
	return line
}

func locateToken(runes []rune, cursor int) tokenInfo {
	if len(runes) == 0 || unicode.IsSpace(runes[0]) {
		return tokenInfo{}
	}
	token := tokenInfo{}
	if runes[0] != '/' {
		return token
	}
	if isBlockedByOtherToken(runes, cursor) {
		token.blocked = true
		return token
	}
	token.found = true
	token.start = 0
	token.end = len(runes)
	for i := 1; i < len(runes); i++ {
		if unicode.IsSpace(runes[i]) {
			token.end = i
			break
		}
		if runes[i] == '/' {
			return tokenInfo{}
		}
	}
	token.value = string(runes[token.start+1 : token.end])
	token.args = strings.TrimLeftFunc(string(runes[token.end:]), unicode.IsSpace)
	token.active = cursor <= token.end
	token.blocked = false
	return token
}

func isBlockedByOtherToken(runes []rune, cursor int) bool {
	if cursor > len(runes) {
		cursor = len(runes)
	}
	start := cursor
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	if start < len(runes) && (runes[start] == '@' || runes[start] == '$') {
		return true
	}
	return false
}

func parseAssignments(args string) map[string]string {
	assignments := map[string]string{}
	fields := strings.Fields(args)
	for _, field := range fields {
		if !strings.Contains(field, "=") {
			continue
		}
		parts := strings.SplitN(field, "=", 2)
		name := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
		assignments[name] = val
	}
	return assignments
}

func runeLen(text string) int {
	return len([]rune(text))
}

func runeLenBefore(text string, byteIdx int) int {
	return len([]rune(text[:byteIdx]))
}

func builtinItems(opts Options) []Item {
	commands := []Item{
		{Kind: ItemBuiltin, Command: CommandModel, Description: "切换模型"},
	}
	if opts.SkillsAvailable {
		commands = append(commands, Item{Kind: ItemBuiltin, Command: CommandSkills, Description: "查看可用技能"})
	}
	commands = append(commands,
		Item{Kind: ItemBuiltin, Command: CommandReview, Description: "进入代码审查模式"},
		Item{Kind: ItemBuiltin, Command: CommandNew, Description: "开始新会话"},
		Item{Kind: ItemBuiltin, Command: CommandResume, Description: "恢复最近会话"},
		Item{Kind: ItemBuiltin, Command: CommandInit, Description: "生成 AGENTS.md 指南"},
		Item{Kind: ItemBuiltin, Command: CommandCompact, Description: "压缩上下文"},
		Item{Kind: ItemBuiltin, Command: CommandUndo, Description: "撤销上一步"},
		Item{Kind: ItemBuiltin, Command: CommandDiff, Description: "查看工作区 diff"},
		Item{Kind: ItemBuiltin, Command: CommandMention, Description: "搜索文件/路径"},
		Item{Kind: ItemBuiltin, Command: CommandStatus, Description: "查看当前状态"},
		Item{Kind: ItemBuiltin, Command: CommandMCP, Description: "管理 MCP 连接"},
		Item{Kind: ItemBuiltin, Command: CommandLogout, Description: "注销登录"},
		Item{Kind: ItemBuiltin, Command: CommandQuit, Description: "退出 Echo"},
		Item{Kind: ItemBuiltin, Command: CommandExit, Description: "退出 Echo"},
		Item{Kind: ItemBuiltin, Command: CommandFeedback, Description: "发送反馈"},
	)
	if opts.Debug {
		commands = append(commands,
			Item{Kind: ItemBuiltin, Command: CommandRollout, Description: "调试：切换分流", DebugOnly: true},
		)
	}
	// 兼容历史命令
	commands = append(commands,
		Item{Kind: ItemBuiltin, Command: CommandClear, Description: "清空会话"},
		Item{Kind: ItemBuiltin, Command: CommandRun, Description: "执行本地命令"},
		Item{Kind: ItemBuiltin, Command: CommandApply, Description: "应用补丁文件"},
		Item{Kind: ItemBuiltin, Command: CommandAttach, Description: "附加文件内容"},
		Item{Kind: ItemBuiltin, Command: CommandSessions, Description: "列出会话"},
	)
	return commands
}

func promptItems(prompts []CustomPrompt) []Item {
	items := make([]Item, 0, len(prompts))
	for _, p := range prompts {
		items = append(items, Item{
			Kind:        ItemPrompt,
			Prompt:      &p,
			Description: promptDescription(p),
			Name:        p.Token(),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Token() < items[j].Token()
	})
	return items
}

func promptDescription(p CustomPrompt) string {
	if strings.TrimSpace(p.Description) != "" {
		return p.Description
	}
	return "send saved prompt"
}
