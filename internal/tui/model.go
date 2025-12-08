package tui

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/execution"
	"echo-cli/internal/i18n"
	"echo-cli/internal/policy"
	"echo-cli/internal/sandbox"
	"echo-cli/internal/search"
	"echo-cli/internal/session"
	"echo-cli/internal/tools"
	toolengine "echo-cli/internal/tools/engine"
	"echo-cli/internal/tui/render"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

type Options struct {
	Engine          *execution.Engine
	Gateway         SubmissionGateway
	Model           string
	Reasoning       string
	Sandbox         string
	Workdir         string
	InitialPrompt   string
	Language        string
	InitialMessages []agent.Message
	Roots           []string
	Policy          policy.Policy
	Events          *events.Bus
	Runner          sandbox.Runner
	Approver        *toolengine.UIApprover
	ResumePicker    bool
	ResumeShowAll   bool
	ResumeSessions  []string
	ResumeSessionID string
}

// SubmissionGateway 抽象 REPL 层提交/订阅能力，避免 TUI 与实现耦合。
type SubmissionGateway interface {
	SubmitUserInput(ctx context.Context, items []events.InputMessage, inputCtx events.InputContext) (string, error)
	Events() <-chan events.Event
}

type assistantReplyMsg struct {
	Text string
}

type agentErrorMsg struct {
	Err error
}

type startPromptMsg struct {
	Text string
}

type assistantChunkMsg struct {
	Text string
}

type engineEventMsg struct {
	Event events.Event
}

type busEventMsg struct {
	Event any
}

type searchResultsMsg struct {
	Paths []string
	Err   error
}

type systemMsg struct {
	Text string
}

type Model struct {
	textarea        textarea.Model
	viewport        render.HighPerformanceViewport
	eventsPane      viewport.Model
	search          list.Model
	sessions        list.Model
	messages        []agent.Message
	streamIdx       int
	streamCh        chan streamEvent
	modelName       string
	reasoning       string
	sandbox         string
	language        string
	workdir         string
	policy          policy.Policy
	runner          sandbox.Runner
	engine          *toolengine.Engine
	roots           []string
	eventsSub       <-chan any
	gateway         SubmissionGateway
	eqSub           <-chan events.Event
	activeSub       string
	pending         bool
	err             error
	initSend        string
	searching       bool
	mentionAt       int
	approveText     string
	approver        *toolengine.UIApprover
	pendingApprove  string
	approveQueue    []approvalPrompt
	pickingSession  bool
	events          []uiEvent
	resumeSessionID string
	updateAction    string
	width           int
	height          int
	eventsWidth     int
	spin            spinner.Model
	showHelp        bool
	transcriptDirty bool
	// chatStatusHeight 为滚动状态栏预留高度。
	chatStatusHeight int
}

type streamEvent struct {
	chunk string
	done  bool
	err   error
}

type uiEvent struct {
	Kind string
	Text string
}

type approvalPrompt struct {
	id   string
	text string
}

func New(opts Options) *Model {
	ti := textarea.New()
	ti.Placeholder = "Ask Echo anything…"
	ti.Prompt = "› "
	ti.CharLimit = 0
	ti.SetWidth(90)
	ti.SetHeight(1) // 默认单行，按需扩展
	ti.ShowLineNumbers = false
	ti.Focus()

	vp := render.NewHighPerformanceViewport(90, 12)
	vp.SetContent("Welcome to Echo (Go). Type a message to start.\n")
	evp := viewport.New(30, 12)
	evp.SetContent("Events")

	items := []list.Item{}
	search := list.New(items, list.NewDefaultDelegate(), 40, 10)
	search.Title = "Select file (@ search)"
	search.SetShowStatusBar(false)
	search.DisableQuitKeybindings()
	sessionItems := []list.Item{}
	for _, id := range opts.ResumeSessions {
		sessionItems = append(sessionItems, listItem(id))
	}
	sessions := list.New(sessionItems, list.NewDefaultDelegate(), 40, 10)
	sessions.Title = "Resume session"
	sessions.SetShowStatusBar(false)
	sessions.DisableQuitKeybindings()

	runner := opts.Runner
	if runner == nil {
		roots := opts.Roots
		if len(roots) == 0 && opts.Workdir != "" {
			roots = []string{opts.Workdir}
		}
		runner = sandbox.NewRunner(opts.Sandbox, roots...)
	}
	approver := opts.Approver
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))

	m := Model{
		textarea:         ti,
		viewport:         vp,
		eventsPane:       evp,
		search:           search,
		sessions:         sessions,
		modelName:        opts.Model,
		reasoning:        opts.Reasoning,
		sandbox:          opts.Sandbox,
		language:         i18n.Normalize(opts.Language).Code(),
		workdir:          opts.Workdir,
		policy:           opts.Policy,
		runner:           runner,
		engine:           toolengine.New(opts.Policy, runner, approver, opts.Workdir),
		roots:            opts.Roots,
		approver:         approver,
		initSend:         opts.InitialPrompt,
		streamIdx:        -1,
		mentionAt:        -1,
		pickingSession:   opts.ResumePicker,
		resumeSessionID:  opts.ResumeSessionID,
		width:            90,
		height:           24,
		eventsWidth:      32,
		spin:             spin,
		transcriptDirty:  true,
		chatStatusHeight: 1,
	}
	if len(m.roots) == 0 && m.workdir != "" {
		m.roots = []string{m.workdir}
	}
	if len(opts.InitialMessages) > 0 {
		m.messages = append(m.messages, opts.InitialMessages...)
		m.refreshTranscript()
	}
	if opts.Events != nil {
		m.eventsSub = opts.Events.Subscribe()
	}
	if opts.Gateway != nil {
		m.gateway = opts.Gateway
		m.eqSub = opts.Gateway.Events()
	}
	return &m
}

func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.listenQueues()...)
	cmds = append(cmds, m.spin.Tick)
	if cmd := m.flushTranscript(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if strings.TrimSpace(m.initSend) == "" {
		return tea.Batch(cmds...)
	}
	prompt := strings.TrimSpace(m.initSend)
	cmds = append(cmds, func() tea.Msg {
		return startPromptMsg{Text: prompt}
	})
	return tea.Batch(cmds...)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		cmds = append(cmds, m.resize(msg.Width, msg.Height)...)
		return m.finish(cmds...)
	case searchResultsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			m.searching = false
			return m.finish(cmds...)
		}
		items := make([]list.Item, 0, len(msg.Paths))
		for _, p := range msg.Paths {
			items = append(items, listItem(p))
		}
		m.search.SetItems(items)
		m.searching = true
		return m.finish(cmds...)
	case startPromptMsg:
		m.messages = append(m.messages, agent.Message{Role: agent.RoleUser, Content: msg.Text})
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: ""})
		m.streamIdx = len(m.messages) - 1
		m.refreshTranscript()
		m.pending = true
		cmds = append(cmds, m.startStream(msg.Text))
		return m.finish(cmds...)
	case assistantReplyMsg:
		m.finishStream(msg.Text)
		return m.finish(cmds...)
	case assistantChunkMsg:
		m.appendChunk(msg.Text)
		cmds = append(cmds, m.listenStream())
		return m.finish(cmds...)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		cmds = append(cmds, cmd)
		return m.finish(cmds...)
	case busEventMsg:
		cmd := m.handleBusEvent(msg.Event)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, m.listenQueues()...)
		return m.finish(cmds...)
	case engineEventMsg:
		cmd := m.handleEngineEvent(msg.Event)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, m.listenQueues()...)
		return m.finish(cmds...)
	case systemMsg:
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: msg.Text})
		m.refreshTranscript()
		return m.finish(cmds...)
	case agentErrorMsg:
		m.pending = false
		m.err = msg.Err
		m.streamCh = nil
		m.streamIdx = -1
		return m.finish(cmds...)
	case tea.MouseMsg:
		if vCmd := m.viewport.HandleUpdate(msg); vCmd != nil {
			cmds = append(cmds, vCmd)
		}
		return m.finish(cmds...)
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter && msg.Alt {
			break
		}
		if m.pickingSession {
			var cmd tea.Cmd
			m.sessions, cmd = m.sessions.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			switch msg.String() {
			case "enter":
				if sel, ok := m.sessions.SelectedItem().(listItem); ok {
					rec, err := session.Load(string(sel))
					if err != nil {
						m.pickingSession = false
						cmds = append(cmds, func() tea.Msg { return systemMsg{Text: fmt.Sprintf("session load error: %v", err)} })
						return m.finish(cmds...)
					}
					m.messages = append([]agent.Message{}, rec.Messages...)
					m.resumeSessionID = rec.ID
					m.pickingSession = false
					m.refreshTranscript()
					return m.finish(cmds...)
				}
			case "esc", "ctrl+c":
				m.pickingSession = false
				return m.finish(cmds...)
			}
			return m.finish(cmds...)
		}
		if m.pendingApprove != "" {
			switch msg.String() {
			case "y", "Y", "enter":
				m.resolveApproval(true)
				return m.finish(cmds...)
			case "n", "N", "esc", "ctrl+c":
				m.resolveApproval(false)
				return m.finish(cmds...)
			}
			return m.finish(cmds...)
		}
		if m.searching {
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			switch msg.String() {
			case "enter":
				if sel, ok := m.search.SelectedItem().(listItem); ok {
					m.insertPath(string(sel))
				}
				m.searching = false
				m.mentionAt = -1
				return m.finish(cmds...)
			case "esc", "ctrl+c":
				m.searching = false
				m.mentionAt = -1
				return m.finish(cmds...)
			}
			return m.finish(cmds...)
		}
		if cmd, handled := m.handleScrollKeys(msg); handled {
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m.finish(cmds...)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			cmds = append(cmds, tea.Quit)
			return m.finish(cmds...)
		case "?":
			m.showHelp = !m.showHelp
			return m.finish(cmds...)
		case "@":
			m.mentionAt = len(m.textarea.Value())
			cmds = append(cmds, m.loadSearch())
			return m.finish(cmds...)
		case "enter":
			if m.pending {
				return m.finish(cmds...)
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m.finish(cmds...)
			}
			if strings.HasPrefix(input, "/") {
				cmd := m.handleSlash(input)
				m.textarea.Reset()
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m.finish(cmds...)
			}
			m.messages = append(m.messages, agent.Message{Role: agent.RoleUser, Content: input})
			m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: ""})
			m.streamIdx = len(m.messages) - 1
			m.refreshTranscript()
			m.textarea.Reset()
			m.setComposerHeight()
			m.pending = true
			cmds = append(cmds, m.startStream(input))
			return m.finish(cmds...)
		}
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.setComposerHeight()
	cmds = append(cmds, cmd)
	return m.finish(cmds...)
}

func (m *Model) finish(cmds ...tea.Cmd) (tea.Model, tea.Cmd) {
	if m.transcriptDirty {
		if cmd := m.flushTranscript(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	banner := renderBanner(m.modelName, m.reasoning, m.workdir, m.width)
	tip := renderTip(m.width)
	chatBody := m.viewport.View()
	if status := m.renderScrollStatus(); status != "" {
		chatBody = lipgloss.JoinVertical(lipgloss.Left, chatBody, status)
	}
	chatPane := renderPane("", chatBody, m.width, m.viewport.Height+m.chatStatusHeight)
	composer := renderPane("Prompt", m.textarea.View(), m.width, m.textarea.Height())
	status := statusLine(m.modelName, m.sandbox, m.workdir, m.pending, m.err, m.width, m.spin)
	hints := renderHints(m.width)
	content := lipgloss.JoinVertical(lipgloss.Left, banner, tip, chatPane, composer, status, hints)

	if m.searching {
		overlay := modalStyle.Render(m.search.View())
		return lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}
	if m.pickingSession {
		overlay := modalStyle.Render(m.sessions.View())
		return lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}
	if m.pendingApprove != "" {
		overlay := modalStyle.Render(
			fmt.Sprintf("Approval required: %s\n[y] approve • [n] cancel", m.approveText),
		)
		return lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}
	if m.showHelp {
		help := strings.Join([]string{
			"快捷键",
			"Enter 发送 • Ctrl+C 退出 • @ 搜索文件 • /sessions 恢复会话 • /run 执行命令 • /apply 应用补丁",
			"? 切换帮助 • /add-dir 添加工作区 • /status 查看状态",
		}, "\n")
		overlay := modalStyle.Render(help)
		return lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}
	return content
}

// History returns a copy of the chat history.
func (m *Model) History() []agent.Message {
	return append([]agent.Message{}, m.messages...)
}

// SessionID returns the active session id if one is set.
func (m *Model) SessionID() string {
	return m.resumeSessionID
}

// UpdateAction returns any pending update action (unused placeholder for parity).
func (m *Model) UpdateAction() string {
	return m.updateAction
}

func (m *Model) startStream(input string) tea.Cmd {
	if m.gateway == nil {
		m.err = fmt.Errorf("submission gateway not configured")
		m.pending = false
		return nil
	}
	if m.resumeSessionID == "" {
		m.resumeSessionID = uuid.NewString()
	}
	subCtx := events.InputContext{
		SessionID:       m.resumeSessionID,
		Model:           m.modelName,
		Language:        m.language,
		ReasoningEffort: m.reasoning,
	}
	id, err := m.gateway.SubmitUserInput(context.Background(), []events.InputMessage{
		{Role: "user", Content: input},
	}, subCtx)
	if err != nil {
		m.err = err
		m.pending = false
		return nil
	}
	m.activeSub = id
	return tea.Batch(m.listenQueues()...)
}

func (m *Model) listenStream() tea.Cmd {
	if m.streamCh == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-m.streamCh
		if !ok {
			return assistantReplyMsg{Text: ""}
		}
		if ev.err != nil {
			return agentErrorMsg{Err: ev.err}
		}
		if ev.done {
			return assistantReplyMsg{Text: ""}
		}
		return assistantChunkMsg{Text: ev.chunk}
	}
}

func (m *Model) listenEvents() tea.Cmd {
	if m.eventsSub == nil {
		return nil
	}
	return func() tea.Msg {
		evt, ok := <-m.eventsSub
		if !ok {
			return nil
		}
		return busEventMsg{Event: evt}
	}
}

func (m *Model) listenEngineEvents() tea.Cmd {
	if m.eqSub == nil {
		return nil
	}
	return func() tea.Msg {
		evt, ok := <-m.eqSub
		if !ok {
			return nil
		}
		return engineEventMsg{Event: evt}
	}
}

func (m *Model) listenQueues() []tea.Cmd {
	cmds := []tea.Cmd{}
	if cmd := m.listenEvents(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.listenEngineEvents(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return cmds
}

func (m *Model) handleBusEvent(evt any) tea.Cmd {
	switch ev := evt.(type) {
	case tools.ToolEvent:
		switch ev.Type {
		case "approval.requested":
			text := approvalDescription(ev)
			m.enqueueApproval(ev.Result.ID, text)
			m.logEvent(ev.Type, text)
		case "approval.completed":
			m.logEvent(ev.Type, ev.Reason)
		case "item.updated":
			text := ev.Result.Output
			if text == "" {
				text = ev.Result.Status
			}
			m.logEvent(string(ev.Result.Kind), text)
		case "item.started", "item.completed":
			status := ev.Result.Status
			if status == "" {
				status = ev.Type
			}
			if ev.Result.Error != "" {
				status = fmt.Sprintf("error: %s", ev.Result.Error)
			}
			m.logEvent(string(ev.Result.Kind), status)
		}
	}
	return nil
}

func (m *Model) handleEngineEvent(evt events.Event) tea.Cmd {
	switch evt.Type {
	case events.EventAgentOutput:
		msg, ok := evt.Payload.(events.AgentOutput)
		if !ok || evt.SubmissionID != m.activeSub {
			return nil
		}
		if msg.Final {
			finalText := msg.Content
			if finalText == "" && m.streamIdx >= 0 && m.streamIdx < len(m.messages) {
				finalText = m.messages[m.streamIdx].Content
			}
			m.finishStream(finalText)
			m.activeSub = ""
			return nil
		}
		if msg.Content != "" {
			m.appendChunk(msg.Content)
		}
	case events.EventError:
		if evt.SubmissionID != m.activeSub {
			return nil
		}
		m.pending = false
		m.activeSub = ""
		m.err = fmt.Errorf("%v", evt.Payload)
	case events.EventTaskCompleted:
		if evt.SubmissionID != m.activeSub {
			return nil
		}
		m.pending = false
		m.activeSub = ""
	}
	return nil
}

func approvalDescription(ev tools.ToolEvent) string {
	switch ev.Result.Kind {
	case tools.ToolCommand:
		if ev.Result.Command != "" {
			return fmt.Sprintf("command: %s", ev.Result.Command)
		}
		return "command execution"
	case tools.ToolApplyPatch:
		if ev.Result.Path != "" {
			return fmt.Sprintf("apply patch: %s", ev.Result.Path)
		}
		return "apply patch"
	case tools.ToolFileRead:
		if ev.Result.Path != "" {
			return fmt.Sprintf("read file: %s", ev.Result.Path)
		}
		return "read file"
	case tools.ToolSearch:
		return "search workspace"
	default:
		if ev.Reason != "" {
			return ev.Reason
		}
		return "approval required"
	}
}

func (m *Model) enqueueApproval(id, text string) {
	if id == "" || m.approver == nil {
		return
	}
	if id == m.pendingApprove {
		return
	}
	for _, item := range m.approveQueue {
		if item.id == id {
			return
		}
	}
	m.approveQueue = append(m.approveQueue, approvalPrompt{id: id, text: text})
	if m.pendingApprove == "" {
		m.advanceApproval()
	}
}

func (m *Model) resolveApproval(allow bool) {
	if m.pendingApprove != "" && m.approver != nil {
		m.approver.Resolve(m.pendingApprove, allow)
	}
	m.pendingApprove = ""
	m.approveText = ""
	m.advanceApproval()
}

func (m *Model) advanceApproval() {
	if len(m.approveQueue) == 0 {
		m.pendingApprove = ""
		m.approveText = ""
		return
	}
	next := m.approveQueue[0]
	m.approveQueue = m.approveQueue[1:]
	m.pendingApprove = next.id
	m.approveText = next.text
}

func (m *Model) finishStream(finalText string) {
	m.pending = false
	m.err = nil
	m.streamCh = nil
	m.streamIdx = -1
	if finalText != "" && len(m.messages) > 0 {
		m.messages = append(m.messages[:len(m.messages)-1], agent.Message{Role: agent.RoleAssistant, Content: finalText})
	}
	m.refreshTranscript()
	m.logEvent("agent_message", "assistant reply completed")
}

func (m *Model) appendChunk(chunk string) {
	if m.streamIdx < 0 || m.streamIdx >= len(m.messages) {
		return
	}
	m.messages[m.streamIdx].Content += chunk
	m.refreshTranscript()
}

func (m *Model) resize(width, height int) []tea.Cmd {
	cmds := []tea.Cmd{}
	m.width = width
	m.height = height
	composerHeight := m.textarea.Height() + 3 // title + border
	headerHeight := 6                         // approximate banner height with border
	tipHeight := 1
	statusHeight := 1
	hintsHeight := 1
	mainHeight := height - composerHeight - headerHeight - statusHeight - hintsHeight - tipHeight
	if mainHeight < 6 {
		mainHeight = 6
	}
	eventsWidth := 0
	chatWidth := width
	m.eventsWidth = eventsWidth

	contentHeight := mainHeight - 2 // border + title
	if contentHeight < 3 {
		contentHeight = 3
	}
	if chatWidth > 0 {
		m.viewport.Width = chatWidth
	}
	if eventsWidth > 0 {
		m.eventsPane.Width = eventsWidth
		m.eventsPane.Height = contentHeight
	}
	statusReserve := m.chatStatusHeight
	if contentHeight <= statusReserve {
		statusReserve = 0
	}
	viewHeight := contentHeight - statusReserve
	if viewHeight < 1 {
		viewHeight = 1
	}
	m.chatStatusHeight = statusReserve
	if cmd := m.viewport.Resize(chatWidth, viewHeight); cmd != nil {
		cmds = append(cmds, cmd)
	}
	m.textarea.SetWidth(width)
	m.eventsPane.SetContent(renderEvents(m.events, m.eventsPane.Width, m.eventsPane.Height))
	m.viewport.SetYPosition(m.viewportTop())
	m.refreshTranscript()
	return cmds
}

func (m *Model) setComposerHeight() {
	lines := strings.Count(m.textarea.Value(), "\n") + 1
	if lines < 1 {
		lines = 1
	}
	if lines > 6 {
		lines = 6
	}
	if m.textarea.Height() != lines {
		m.textarea.SetHeight(lines)
		if m.width > 0 && m.height > 0 {
			m.resize(m.width, m.height)
		}
	}
}

func (m *Model) refreshTranscript() {
	m.transcriptDirty = true
}

func (m *Model) flushTranscript() tea.Cmd {
	if !m.transcriptDirty {
		return nil
	}
	lines := m.renderTranscriptLines()
	m.transcriptDirty = false
	m.viewport.SetYPosition(m.viewportTop())
	return m.viewport.SetLines(lines)
}

func (m *Model) renderTranscriptLines() []string {
	width := m.viewport.Width
	if width <= 0 {
		width = 80
	}
	lines := render.RenderMessages(m.messages, width)
	if len(lines) == 0 {
		return []string{"Welcome to Echo (Go). Type a message to start."}
	}
	return render.LinesToStrings(lines)
}

func statusLine(model, sandbox, workdir string, pending bool, err error, width int, spin spinner.Model) string {
	parts := []string{
		fmt.Sprintf("Model: %s", model),
		fmt.Sprintf("Sandbox: %s", sandbox),
	}
	if workdir != "" {
		parts = append(parts, fmt.Sprintf("Dir: %s", workdir))
	}
	if pending {
		parts = append(parts, "Working… "+spin.View())
	}
	if err != nil {
		parts = append(parts, fmt.Sprintf("Error: %v", err))
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D7A85")).
		Padding(0, 1).
		Width(maxInt(20, width)).
		Render(strings.Join(parts, " • "))
}

func renderHeader(model, sandbox, workdir string, width int) string {
	left := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render("Echo")
	info := []string{fmt.Sprintf("Model %s", model), fmt.Sprintf("Sandbox %s", sandbox)}
	if workdir != "" {
		info = append(info, workdir)
	}
	right := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D7A85")).Render(strings.Join(info, " • "))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Width(maxInt(20, width)).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, left, lipgloss.NewStyle().PaddingLeft(2).Render(right)))
}

const tuiVersion = "v0.65.0"

func renderBanner(model, reasoning, workdir string, width int) string {
	line1 := fmt.Sprintf(">_ Echo Team Echo (%s)", tuiVersion)
	modelText := model
	if reasoning != "" {
		modelText = fmt.Sprintf("%s %s", model, reasoning)
	}
	line2 := fmt.Sprintf("model:     %s   /model to change", modelText)
	dirLine := fmt.Sprintf("directory: %s", workdir)

	body := []string{
		line1,
		"",
		line2,
		dirLine,
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Width(maxInt(40, width)).
		Render(strings.Join(body, "\n"))
}

func renderPane(title string, body string, width int, height int) string {
	titleText := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render(title)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5E6472")).
		Padding(0, 1)
	if width > 0 {
		style = style.Width(width)
	}
	if height > 0 {
		totalHeight := height + 2 // body + padding
		if strings.TrimSpace(title) != "" {
			totalHeight++
		}
		style = style.Height(totalHeight)
	}
	content := body
	if strings.TrimSpace(title) != "" {
		content = lipgloss.JoinVertical(lipgloss.Left, titleText, body)
	}
	return style.Render(content)
}

func renderTip(width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D7A85")).
		Padding(0, 2).
		Width(maxInt(20, width)).
		Render("Tip: Use /feedback to send logs to the maintainers when something looks off.")
}

func renderHints(width int) string {
	hint := "↑/↓ 滚动 • Enter 发送 • Alt+Enter 换行 • Ctrl+C 退出 • @ 搜索文件 • ? 帮助 • /sessions 恢复会话"
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D7A85")).
		Padding(0, 1).
		Width(maxInt(20, width)).
		Render(hint)
}

func (m *Model) viewportTop() int {
	banner := renderBanner(m.modelName, m.reasoning, m.workdir, m.width)
	tip := renderTip(m.width)
	top := lipgloss.Height(banner) + lipgloss.Height(tip) + 1 // border 顶部占一行
	if top < 0 {
		return 0
	}
	return top
}

func (m *Model) handleScrollKeys(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyPgUp:
		return m.viewport.ScrollPageUp(), true
	case tea.KeyPgDown:
		return m.viewport.ScrollPageDown(), true
	case tea.KeyHome:
		return m.viewport.GotoTopCmd(), true
	case tea.KeyEnd:
		return m.viewport.GotoBottomCmd(), true
	case tea.KeyUp:
		if m.shouldScrollViewport(tea.KeyUp, msg) {
			return m.viewport.ScrollLineUp(1), true
		}
	case tea.KeyDown:
		if m.shouldScrollViewport(tea.KeyDown, msg) {
			return m.viewport.ScrollLineDown(1), true
		}
	}
	return nil, false
}

func (m *Model) shouldScrollViewport(direction tea.KeyType, msg tea.KeyMsg) bool {
	if msg.Alt {
		return true
	}

	lineInfo := m.textarea.LineInfo()
	if lineInfo.Height < 1 {
		lineInfo.Height = 1
	}

	atTop := m.textarea.Line() == 0 && lineInfo.RowOffset == 0
	lastLine := m.textarea.LineCount() - 1
	if lastLine < 0 {
		lastLine = 0
	}
	atBottomLine := m.textarea.Line() >= lastLine
	atBottomRow := lineInfo.RowOffset >= lineInfo.Height-1

	switch direction {
	case tea.KeyUp:
		return atTop
	case tea.KeyDown:
		return atBottomLine && atBottomRow
	default:
		return false
	}
}

func (m *Model) renderScrollStatus() string {
	if m.chatStatusHeight == 0 {
		return ""
	}
	percent := int(math.Round(m.viewport.ScrollPercent() * 100))
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	width := m.viewport.Width - 12
	if width < 10 {
		width = 10
	}
	filled := int(math.Round(float64(width) * float64(percent) / 100.0))
	if filled > width {
		filled = width
	}
	bar := "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]"
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D7A85")).
		Render(fmt.Sprintf("%s %3d%%", bar, percent))
}

var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(1).
	BorderForeground(lipgloss.Color("#FFB454")).
	Background(lipgloss.Color("#1F1D2B"))

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *Model) insertPath(text string) {
	val := m.textarea.Value()
	if m.mentionAt >= 0 && m.mentionAt <= len(val) {
		before := val[:m.mentionAt]
		after := ""
		if m.mentionAt+1 <= len(val) {
			after = val[m.mentionAt+1:]
		}
		m.textarea.SetValue(before + text + " " + after)
		m.mentionAt = -1
		return
	}
	m.textarea.SetValue(val + text)
}

func (m *Model) loadSearch() tea.Cmd {
	m.search.SetItems(nil)
	m.searching = true
	root := m.workdir
	if root == "" {
		root = "."
	}
	return func() tea.Msg {
		paths, err := search.FindFiles(root, 200)
		return searchResultsMsg{Paths: paths, Err: err}
	}
}

type listItem string

func (i listItem) FilterValue() string { return string(i) }
func (i listItem) Title() string       { return string(i) }
func (i listItem) Description() string { return "" }

func (m *Model) handleSlash(input string) tea.Cmd {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}
	cmd := parts[0]
	switch cmd {
	case "/quit", "/exit":
		return tea.Quit
	case "/clear":
		m.messages = nil
		m.refreshTranscript()
		return nil
	case "/status":
		info := fmt.Sprintf("model=%s sandbox=%s dir=%s", m.modelName, m.sandbox, m.workdir)
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: info})
		m.refreshTranscript()
		return nil
	case "/sessions":
		ids, err := session.ListIDs()
		if err != nil {
			return func() tea.Msg { return systemMsg{Text: fmt.Sprintf("sessions error: %v", err)} }
		}
		items := make([]list.Item, 0, len(ids))
		for _, id := range ids {
			items = append(items, listItem(id))
		}
		m.sessions.SetItems(items)
		m.pickingSession = true
		return nil
	case "/model":
		if len(parts) > 1 {
			m.modelName = parts[1]
		}
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: fmt.Sprintf("using model %s", m.modelName)})
		m.refreshTranscript()
		return nil
	case "/approvals":
		if len(parts) > 1 {
			m.policy.ApprovalPolicy = parts[1]
			m.engine = toolengine.New(m.policy, m.runner, m.approver, m.workdir)
		}
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: fmt.Sprintf("approval policy=%s", m.policy.ApprovalPolicy)})
		m.refreshTranscript()
		return nil
	case "/init":
		m.messages = nil
		m.refreshTranscript()
		return nil
	case "/resume":
		rec, err := session.Last()
		if err != nil {
			return func() tea.Msg { return systemMsg{Text: fmt.Sprintf("resume error: %v", err)} }
		}
		m.messages = append([]agent.Message{}, rec.Messages...)
		m.resumeSessionID = rec.ID
		m.refreshTranscript()
		return nil
	case "/diff":
		return m.runTool(tools.ToolRequest{
			ID:      "local-diff",
			Kind:    tools.ToolCommand,
			Command: "git diff --stat",
		})
	case "/add-dir":
		if len(parts) < 2 {
			m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: "usage: /add-dir <path>"})
			m.refreshTranscript()
			return nil
		}
		path := parts[1]
		if !filepath.IsAbs(path) && m.workdir != "" {
			path = filepath.Join(m.workdir, path)
		}
		m.roots = append(m.roots, path)
		m.runner = sandbox.NewRunner(m.sandbox, m.roots...)
		m.engine = toolengine.New(m.policy, m.runner, m.approver, m.workdir)
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: fmt.Sprintf("added workspace root: %s", path)})
		m.refreshTranscript()
		return nil
	case "/run":
		if len(parts) < 2 {
			m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: "usage: /run <command>"})
			m.refreshTranscript()
			return nil
		}
		command := strings.TrimPrefix(input, "/run ")
		return m.runTool(tools.ToolRequest{
			ID:      "local-run",
			Kind:    tools.ToolCommand,
			Command: command,
		})
	case "/apply":
		if len(parts) < 2 {
			m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: "usage: /apply <patch-file>"})
			m.refreshTranscript()
			return nil
		}
		patchPath := parts[1]
		abs := patchPath
		if !filepath.IsAbs(patchPath) && m.workdir != "" {
			abs = filepath.Join(m.workdir, patchPath)
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return func() tea.Msg { return systemMsg{Text: fmt.Sprintf("read patch failed: %v", err)} }
		}
		return m.runTool(tools.ToolRequest{
			ID:    "local-apply",
			Kind:  tools.ToolApplyPatch,
			Path:  patchPath,
			Patch: string(data),
		})
	case "/attach":
		if len(parts) < 2 {
			m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: "usage: /attach <file>"})
			m.refreshTranscript()
			return nil
		}
		target := parts[1]
		if !filepath.IsAbs(target) && m.workdir != "" {
			target = filepath.Join(m.workdir, target)
		}
		data, err := os.ReadFile(target)
		if err != nil {
			return func() tea.Msg { return systemMsg{Text: fmt.Sprintf("attach failed: %v", err)} }
		}
		msg := agent.Message{Role: agent.RoleUser, Content: fmt.Sprintf("Attachment %s:\n%s", parts[1], string(data))}
		m.messages = append(m.messages, msg)
		m.refreshTranscript()
		return nil
	case "/mention":
		return m.loadSearch()
	case "/feedback":
		msg := "feedback captured (local only); thank you!"
		if len(parts) > 1 {
			msg = strings.TrimPrefix(input, "/feedback ")
		}
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: msg})
		m.refreshTranscript()
		return nil
	}
	return nil
}

func (m *Model) runTool(req tools.ToolRequest) tea.Cmd {
	return func() tea.Msg {
		m.engine.Run(context.Background(), req, func(ev tools.ToolEvent) {
			m.handleBusEvent(ev)
		})
		return nil
	}
}

func (m *Model) logEvent(kind, text string) {
	m.events = append(m.events, uiEvent{Kind: kind, Text: text})
	if len(m.events) > 50 {
		m.events = m.events[len(m.events)-50:]
	}
	m.eventsPane.SetContent(renderEvents(m.events, m.eventsPane.Width, m.eventsPane.Height))
}

func renderEvents(events []uiEvent, width int, height int) string {
	if width <= 0 {
		width = 80
	}
	limit := 5
	if height > 1 {
		limit = height - 1
		if limit < 1 {
			limit = 1
		}
	}
	if len(events) < limit {
		limit = len(events)
	}
	if limit == 0 {
		return ""
	}
	start := len(events) - limit
	lines := make([]string, 0, limit+1)
	lines = append(lines, "Recent events")
	for _, ev := range events[start:] {
		tag := ev.Kind
		prefix := "•"
		switch ev.Kind {
		case "approval.requested":
			prefix = "?"
		case "approval.completed":
			prefix = "✓"
		case "command_execution":
			prefix = ">"
		case "file_change":
			prefix = "Δ"
		case "agent_message":
			prefix = "✎"
		}
		lines = append(lines, fmt.Sprintf("%s %s %s", prefix, tag, ev.Text))
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}
