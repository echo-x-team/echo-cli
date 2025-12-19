package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/execution"
	"echo-cli/internal/history"
	"echo-cli/internal/i18n"
	"echo-cli/internal/logger"
	"echo-cli/internal/search"
	"echo-cli/internal/session"
	"echo-cli/internal/tools"
	"echo-cli/internal/tools/handlers"
	tuirender "echo-cli/internal/tui/render"
	"echo-cli/internal/tui/slash"

	"github.com/atotto/clipboard"
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
	Workdir         string
	InitialPrompt   string
	Language        string
	InitialMessages []agent.Message
	Events          *events.Bus
	Runner          tools.Runner
	ResumePicker    bool
	ResumeShowAll   bool
	ResumeSessions  []string
	ResumeSessionID string
	CustomPrompts   []slash.CustomPrompt
	SkillsAvailable bool
	Debug           bool
	ConversationLog *logger.LogEntry
	CopyableOutput  bool
}

// SubmissionGateway 抽象 REPL 层提交/订阅能力，避免 TUI 与实现耦合。
type SubmissionGateway interface {
	SubmitUserInput(ctx context.Context, items []events.InputMessage, inputCtx events.InputContext) (string, error)
	SubmitApprovalDecision(ctx context.Context, sessionID string, approvalID string, approved bool) (string, error)
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
	textarea                 textarea.Model
	viewport                 tuirender.HighPerformanceViewport
	eventsPane               viewport.Model
	search                   list.Model
	sessions                 list.Model
	messages                 []agent.Message
	planUpdate               *tools.UpdatePlanArgs
	eqCtx                    tuirender.Context
	eqRenderers              map[events.EventType]tuirender.EventRenderer
	streamIdx                int
	streamCh                 chan streamEvent
	modelName                string
	reasoning                string
	language                 string
	workdir                  string
	runner                   tools.Runner
	toolRuntime              *tools.Runtime
	eventsSub                <-chan any
	gateway                  SubmissionGateway
	eqSub                    <-chan events.Event
	activeSub                string
	pending                  bool
	err                      error
	approvalActive           *approvalRequest
	approvalQueue            []approvalRequest
	initSend                 string
	searching                bool
	mentionAt                int
	queuedMessages           []string
	pickingSession           bool
	events                   []uiEvent
	resumeSessionID          string
	updateAction             string
	historyStore             *history.Store
	history                  promptHistory
	width                    int
	height                   int
	eventsWidth              int
	spin                     spinner.Model
	showHelp                 bool
	transcriptDirty          bool
	pendingSince             time.Time
	slash                    *slash.State
	reviewMode               bool
	chromeCollapsed          bool
	conversationLog          *logger.LogEntry
	lastConversationSnapshot string
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

func New(opts Options) *Model {
	ti := textarea.New()
	ti.Placeholder = "Ask Echo anything…"
	ti.Prompt = "› "
	ti.CharLimit = 0
	ti.SetWidth(90)
	ti.SetHeight(1) // 默认单行，按需扩展
	ti.ShowLineNumbers = false
	ti.Focus()

	vp := tuirender.NewHighPerformanceViewport(90, 12)
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
		runner = tools.DirectRunner{}
	}
	toolRuntime := tools.NewRuntime(tools.RuntimeOptions{
		Runner:   runner,
		Workdir:  opts.Workdir,
		Handlers: handlers.Default(),
	})
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))

	sl := slash.NewState(slash.Options{
		CustomPrompts:   opts.CustomPrompts,
		SkillsAvailable: opts.SkillsAvailable,
		Debug:           opts.Debug,
	})

	m := Model{
		textarea:   ti,
		viewport:   vp,
		eventsPane: evp,
		search:     search,
		sessions:   sessions,
		eqCtx: tuirender.Context{
			SessionID:  opts.ResumeSessionID,
			Transcript: tuirender.NewTranscript(90),
		},
		eqRenderers:     tuirender.DefaultRenderers(),
		modelName:       opts.Model,
		reasoning:       opts.Reasoning,
		language:        i18n.Normalize(opts.Language).Code(),
		workdir:         opts.Workdir,
		runner:          runner,
		toolRuntime:     toolRuntime,
		initSend:        opts.InitialPrompt,
		streamIdx:       -1,
		mentionAt:       -1,
		pickingSession:  opts.ResumePicker,
		resumeSessionID: opts.ResumeSessionID,
		width:           90,
		height:          24,
		eventsWidth:     32,
		spin:            spin,
		transcriptDirty: true,
		slash:           sl,
		conversationLog: opts.ConversationLog,
	}
	// TUI doesn't render submission.accepted into transcript because user input is
	// already echoed locally. Still keep ActiveSub in sync.
	m.eqRenderers[events.EventSubmissionAccepted] = tuiSubmissionAcceptedRenderer{}
	// TUI renders plan.updated in a fixed section instead of appending to the transcript.
	m.eqRenderers[events.EventPlanUpdated] = tuiPlanUpdatedRenderer{}
	m.eqCtx.EmitLines = func(lines []string) {
		if len(lines) == 0 {
			return
		}
		m.messages = m.eqCtx.Transcript.Messages()
		m.refreshTranscript()
	}
	if len(opts.InitialMessages) > 0 {
		m.eqCtx.Transcript.LoadMessages(opts.InitialMessages)
		m.messages = m.eqCtx.Transcript.Messages()
		m.refreshTranscript()
	}
	if opts.Events != nil {
		m.eventsSub = opts.Events.Subscribe()
	}
	if opts.Gateway != nil {
		m.gateway = opts.Gateway
		m.eqSub = opts.Gateway.Events()
	}

	if hs, err := history.NewDefault(); err == nil {
		m.historyStore = hs
		if texts, err := hs.LoadTexts(); err == nil {
			m.history.Set(texts)
		} else {
			m.logEvent("history_error", fmt.Sprintf("load history failed: %v", err))
		}
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
	defer m.syncSlashState()

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
		m.recordHistory(msg.Text)
		m.appendUserMessage(msg.Text)
		m.appendAssistantPlaceholder()
		m.streamIdx = len(m.messages) - 1
		m.pending = true
		m.pendingSince = time.Now()
		cmds = append(cmds, m.startStream(msg.Text))
		return m.finish(cmds...)
	case assistantReplyMsg:
		if cmd := m.finishStream(msg.Text); cmd != nil {
			cmds = append(cmds, cmd)
		}
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
		m.appendAssistantMessage(msg.Text)
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
		if m.approvalActive != nil {
			if cmd := m.handleApprovalKey(msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m.finish(cmds...)
		}
		if msg.Type == tea.KeyEnter && msg.Alt {
			break
		}
		if msg.Type == tea.KeyCtrlY {
			m.copyConversation()
			return m.finish(cmds...)
		}
		if msg.String() == "ctrl+t" {
			if cmd := m.toggleChrome(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m.finish(cmds...)
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
					m.resumeSessionID = rec.ID
					m.eqCtx.SessionID = rec.ID
					m.pickingSession = false
					m.loadTranscriptMessages(rec.Messages)
					return m.finish(cmds...)
				}
			case "esc", "ctrl+c":
				m.pickingSession = false
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
		if m.slash != nil && m.slash.Open() {
			if action, handled := m.handleSlashKey(msg); handled {
				if cmd := m.applySlashAction(action); cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m.finish(cmds...)
			}
		}
		if m.history.Browsing() && isHistoryEditingKey(msg) {
			m.history.ResetBrowsing()
		}
		if m.handleHistoryKeys(msg) {
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
				input := strings.TrimSpace(m.textarea.Value())
				if input == "" {
					return m.finish(cmds...)
				}
				m.recordHistory(input)
				m.enqueueQueued(input)
				m.textarea.Reset()
				m.setComposerHeight()
				return m.finish(cmds...)
			}
			value := m.textarea.Value()
			input := strings.TrimSpace(value)
			if input == "" {
				return m.finish(cmds...)
			}
			if action := m.resolveSlashSubmit(value); action.Kind != slash.ActionNone {
				if cmd := m.applySlashAction(action); cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m.finish(cmds...)
			}
			m.recordHistory(input)
			m.appendUserMessage(input)
			m.appendAssistantPlaceholder()
			m.streamIdx = len(m.messages) - 1
			m.textarea.Reset()
			m.setComposerHeight()
			m.pending = true
			m.pendingSince = time.Now()
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

func isHistoryEditingKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes, tea.KeyBackspace, tea.KeyDelete:
		return true
	default:
		return false
	}
}

func (m *Model) handleHistoryKeys(msg tea.KeyMsg) bool {
	if msg.Alt {
		return false
	}
	if msg.Type != tea.KeyUp && msg.Type != tea.KeyDown {
		return false
	}
	// 多行输入默认保留上下移动光标的行为；单行输入才接管为历史浏览。
	if m.textarea.LineCount() > 1 {
		return false
	}

	switch msg.Type {
	case tea.KeyUp:
		if next, ok := m.history.Prev(m.textarea.Value()); ok {
			m.textarea.SetValue(next)
			m.moveCursorToColumn(len(next))
			m.setComposerHeight()
			return true
		}
	case tea.KeyDown:
		if next, ok := m.history.Next(); ok {
			m.textarea.SetValue(next)
			m.moveCursorToColumn(len(next))
			m.setComposerHeight()
			return true
		}
	}
	return false
}

func (m *Model) recordHistory(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if m.historyStore != nil {
		if err := m.historyStore.Append(text); err != nil {
			m.logEvent("history_error", fmt.Sprintf("append history failed: %v", err))
		}
	}
	m.history.Add(text)
}

func (m *Model) View() string {
	header := m.headerSection(m.width)
	quick := m.quickHelpSection(m.width)
	plan := m.planSection(m.width)
	history := renderConversation(m.viewport.View(), m.conversationWidth())
	composer := renderPane("Prompt", m.textarea.View(), m.width, m.textarea.Height())
	status := m.statusLine(m.width)
	queue := renderQueuedPreview(m.queuedMessages, m.pending, m.width)
	hints := renderHints(m.width)

	bottomParts := []string{}
	if status != "" {
		bottomParts = append(bottomParts, status)
	}
	if queue != "" {
		bottomParts = append(bottomParts, queue)
	}
	bottomParts = append(bottomParts, composer, hints)
	bottom := lipgloss.JoinVertical(lipgloss.Left, bottomParts...)

	sections := []string{header}
	if quick != "" {
		sections = append(sections, quick)
	}
	if plan != "" {
		sections = append(sections, plan)
	}
	sections = append(sections, history, bottom)
	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	if m.approvalActive != nil {
		width := m.width - 4
		if width < 20 {
			width = m.width
		}
		overlay := modalStyle.Render(m.approvalView(width))
		return lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}
	if m.searching {
		overlay := modalStyle.Render(m.search.View())
		return lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}
	if m.pickingSession {
		overlay := modalStyle.Render(m.sessions.View())
		return lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}
	if m.slash != nil && m.slash.Open() {
		width := m.width - 4
		if width < 20 {
			width = m.width
		}
		overlay := modalStyle.Render(m.slash.View(width))
		return lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}
	if m.showHelp {
		help := strings.Join([]string{
			"快捷键",
			"Enter 发送 • Ctrl+C 退出 • @ 搜索文件 • /sessions 恢复会话 • /run 执行命令 • /apply 应用补丁",
			"? 切换帮助 • /status 查看状态",
		}, "\n")
		overlay := modalStyle.Render(help)
		return lipgloss.JoinVertical(lipgloss.Left, content, overlay)
	}
	return content
}

func (m *Model) headerSection(width int) string {
	if m.chromeCollapsed {
		return renderSessionBanner(m.modelName, m.reasoning, m.workdir, width)
	}
	return renderSessionCard(m.modelName, m.reasoning, m.workdir, width)
}

func (m *Model) quickHelpSection(width int) string {
	if m.chromeCollapsed {
		return ""
	}
	return renderQuickHelp(width)
}

func (m *Model) planSection(width int) string {
	if m.planUpdate == nil {
		return ""
	}
	if width <= 0 {
		width = 80
	}
	lines := tuirender.RenderPlanUpdate(*m.planUpdate, width)
	if len(lines) == 0 {
		return ""
	}
	body := strings.Join(tuirender.LinesToStrings(lines), "\n")
	return lipgloss.NewStyle().
		Padding(0, 1).
		Width(maxInt(20, width)).
		Render(body)
}

// History returns a copy of the chat history.
func (m *Model) History() []agent.Message {
	if m.eqCtx.Transcript != nil {
		return m.eqCtx.Transcript.ViewMessages()
	}
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
	return m.startSubmission(input, m.defaultInputContext())
}

func (m *Model) startSubmission(input string, inputCtx events.InputContext) tea.Cmd {
	if m.gateway == nil {
		m.err = fmt.Errorf("submission gateway not configured")
		m.pending = false
		m.pendingSince = time.Time{}
		return nil
	}
	if m.resumeSessionID == "" {
		if inputCtx.SessionID != "" {
			m.resumeSessionID = inputCtx.SessionID
		} else {
			m.resumeSessionID = uuid.NewString()
		}
	}
	if inputCtx.SessionID == "" {
		inputCtx.SessionID = m.resumeSessionID
	}
	m.eqCtx.SessionID = inputCtx.SessionID
	id, err := m.gateway.SubmitUserInput(context.Background(), []events.InputMessage{
		{Role: "user", Content: input},
	}, inputCtx)
	if err != nil {
		m.err = err
		m.pending = false
		m.pendingSince = time.Time{}
		return nil
	}
	m.activeSub = id
	m.eqCtx.ActiveSub = id
	return tea.Batch(m.listenQueues()...)
}

func (m *Model) defaultInputContext() events.InputContext {
	return events.InputContext{
		SessionID:       m.resumeSessionID,
		Model:           m.modelName,
		Language:        m.language,
		ReasoningEffort: m.reasoning,
		ReviewMode:      m.reviewMode,
	}
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
	// Filter out other sessions when a session id is fixed.
	if m.eqCtx.SessionID != "" && evt.SessionID != m.eqCtx.SessionID {
		return nil
	}

	// First, dispatch to per-event transcript renderer.
	if renderer := m.eqRenderers[evt.Type]; renderer != nil {
		renderer.Handle(&m.eqCtx, evt)
	}

	// Then update TUI-specific pending/queue state.
	switch evt.Type {
	case events.EventToolEvent:
		toolEv, ok := evt.Payload.(tools.ToolEvent)
		if !ok {
			return nil
		}
		if toolEv.Type == "item.updated" && strings.EqualFold(strings.TrimSpace(toolEv.Result.Status), "requires_approval") {
			m.enqueueApprovalRequest(toolEv.Result, evt.SessionID)
		}
	case events.EventPlanUpdated:
		args, ok := evt.Payload.(tools.UpdatePlanArgs)
		if !ok {
			return nil
		}
		// Keep only the latest snapshot; updates replace previous content immediately.
		next := tools.UpdatePlanArgs{
			Explanation: args.Explanation,
			Plan:        append([]tools.PlanItem(nil), args.Plan...),
		}
		m.planUpdate = &next
		m.refreshTranscript() // plan section affects available viewport height
	case events.EventAgentOutput:
		msg, ok := evt.Payload.(events.AgentOutput)
		if !ok || evt.SubmissionID != m.activeSub {
			return nil
		}
		if msg.Final {
			m.pending = false
			m.pendingSince = time.Time{}
			m.err = nil
			m.streamCh = nil
			m.streamIdx = -1
			m.activeSub = ""
			m.eqCtx.ActiveSub = ""
			m.logEvent("agent_message", "assistant reply completed")
			return m.startQueuedIfAny()
		}
	case events.EventError:
		if evt.SubmissionID != m.activeSub {
			return nil
		}
		m.pending = false
		m.pendingSince = time.Time{}
		m.activeSub = ""
		m.eqCtx.ActiveSub = ""
		m.err = fmt.Errorf("%v", evt.Payload)
		return m.startQueuedIfAny()
	case events.EventTaskCompleted:
		if evt.SubmissionID != m.activeSub {
			return nil
		}
		m.pending = false
		m.pendingSince = time.Time{}
		m.activeSub = ""
		m.eqCtx.ActiveSub = ""
		return m.startQueuedIfAny()
	}
	return nil
}

// tuiSubmissionAcceptedRenderer keeps ActiveSub in sync for TUI without re-echoing user input.
type tuiSubmissionAcceptedRenderer struct{}

func (tuiSubmissionAcceptedRenderer) Type() events.EventType { return events.EventSubmissionAccepted }

func (tuiSubmissionAcceptedRenderer) Handle(ctx *tuirender.Context, evt events.Event) {
	if ctx == nil {
		return
	}
	if evt.SubmissionID != "" {
		ctx.ActiveSub = evt.SubmissionID
	}
}

// tuiPlanUpdatedRenderer suppresses transcript rendering for plan.updated.
// The TUI displays the latest plan snapshot in a fixed section instead.
type tuiPlanUpdatedRenderer struct{}

func (tuiPlanUpdatedRenderer) Type() events.EventType { return events.EventPlanUpdated }

func (tuiPlanUpdatedRenderer) Handle(*tuirender.Context, events.Event) {}

func (m *Model) enqueueQueued(text string) {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return
	}
	m.queuedMessages = append(m.queuedMessages, normalized)
	m.logEvent("queued", normalized)
}

func (m *Model) startQueuedIfAny() tea.Cmd {
	if m.pending || len(m.queuedMessages) == 0 {
		return nil
	}
	next := m.queuedMessages[0]
	m.queuedMessages = m.queuedMessages[1:]
	m.appendUserMessage(next)
	m.appendAssistantPlaceholder()
	m.streamIdx = len(m.messages) - 1
	m.pending = true
	m.pendingSince = time.Now()
	return m.startStream(next)
}

func (m *Model) finishStream(finalText string) tea.Cmd {
	m.pending = false
	m.pendingSince = time.Time{}
	m.err = nil
	m.streamCh = nil
	m.streamIdx = -1
	m.finalizeAssistantInTranscript(finalText)
	m.logEvent("agent_message", "assistant reply completed")
	return m.startQueuedIfAny()
}

func (m *Model) appendChunk(chunk string) {
	m.appendAssistantChunkToTranscript(chunk)
}

func (m *Model) resize(width, height int) []tea.Cmd {
	cmds := []tea.Cmd{}
	m.width = width
	m.height = height

	convWidth := m.conversationWidth()
	convHeight := m.conversationHeight(convWidth)

	if width > 0 {
		m.textarea.SetWidth(width)
	}
	if m.eqCtx.Transcript != nil && convWidth != m.viewport.Width {
		m.eqCtx.Transcript.SetWidth(convWidth)
	}
	if resizeCmd := m.viewport.Resize(convWidth, convHeight); resizeCmd != nil {
		cmds = append(cmds, resizeCmd)
	}
	m.eventsPane.SetContent(renderEvents(m.events, m.eventsPane.Width, m.eventsPane.Height))
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

func (m *Model) syncSlashState() {
	if m.slash == nil {
		return
	}
	lineInfo := m.textarea.LineInfo()
	cursorCol := lineInfo.StartColumn + lineInfo.ColumnOffset
	m.slash.SyncInput(slash.Input{
		Value:        m.textarea.Value(),
		CursorLine:   m.textarea.Line(),
		CursorColumn: cursorCol,
		Blocked:      m.searching || m.pickingSession,
	})
}

func (m *Model) moveCursorToColumn(col int) {
	if col < 0 {
		col = 0
	}
	for m.textarea.Line() > 0 {
		m.textarea.CursorUp()
	}
	m.textarea.SetCursor(col)
}

func (m *Model) handleSlashKey(msg tea.KeyMsg) (slash.Action, bool) {
	if m.slash == nil {
		return slash.Action{}, false
	}
	return m.slash.HandleKey(msg.String())
}

func (m *Model) resolveSlashSubmit(value string) slash.Action {
	if m.slash == nil {
		return slash.Action{Kind: slash.ActionNone}
	}
	trimmed := strings.TrimLeft(value, " \t")
	if !strings.HasPrefix(trimmed, "/") {
		return slash.Action{Kind: slash.ActionNone}
	}
	return m.slash.ResolveSubmit(value)
}

func (m *Model) applySlashAction(action slash.Action) tea.Cmd {
	switch action.Kind {
	case slash.ActionInsert:
		if action.NewValue != "" {
			m.textarea.SetValue(action.NewValue)
			m.moveCursorToColumn(action.CursorColumn)
			m.setComposerHeight()
		}
	case slash.ActionSubmitCommand:
		cmdText := "/" + string(action.Command)
		if strings.TrimSpace(action.Args) != "" {
			cmdText += " " + action.Args
		}
		m.recordHistory(cmdText)
		m.textarea.Reset()
		m.setComposerHeight()
		return m.executeSlashCommand(action.Command, action.Args)
	case slash.ActionSubmitPrompt:
		m.textarea.Reset()
		m.setComposerHeight()
		return m.submitSlashPrompt(action)
	case slash.ActionError:
		if action.Message != "" {
			m.appendAssistantMessage(action.Message)
		}
	case slash.ActionClose, slash.ActionNone:
		// no-op
	}
	return nil
}

func (m *Model) submitSlashPrompt(action slash.Action) tea.Cmd {
	text := strings.TrimSpace(action.SubmitText)
	if text == "" {
		text = strings.TrimSpace(action.Args)
	}
	if text == "" {
		return nil
	}
	m.recordHistory(text)
	if m.pending {
		m.enqueueQueued(text)
		return nil
	}
	m.appendUserMessage(text)
	m.appendAssistantPlaceholder()
	m.streamIdx = len(m.messages) - 1
	m.pending = true
	m.pendingSince = time.Now()
	return m.startStream(text)
}

// Conversation helpers backed by eqCtx.Transcript.
func (m *Model) appendUserMessage(text string) {
	if m.eqCtx.Transcript == nil {
		m.eqCtx.Transcript = tuirender.NewTranscript(m.conversationWidth())
	}
	m.eqCtx.Transcript.AppendUser(text)
	m.messages = m.eqCtx.Transcript.Messages()
	m.refreshTranscript()
}

func (m *Model) appendAssistantMessage(text string) {
	if m.eqCtx.Transcript == nil {
		m.eqCtx.Transcript = tuirender.NewTranscript(m.conversationWidth())
	}
	m.eqCtx.Transcript.FinalizeAssistant(text)
	m.messages = m.eqCtx.Transcript.Messages()
	m.refreshTranscript()
}

func (m *Model) appendAssistantPlaceholder() {
	if m.eqCtx.Transcript == nil {
		m.eqCtx.Transcript = tuirender.NewTranscript(m.conversationWidth())
	}
	// Empty chunk creates a placeholder assistant bullet.
	m.eqCtx.Transcript.AppendAssistantChunk("")
	m.messages = m.eqCtx.Transcript.Messages()
	m.refreshTranscript()
}

func (m *Model) appendAssistantChunkToTranscript(chunk string) {
	if m.eqCtx.Transcript == nil {
		return
	}
	m.eqCtx.Transcript.AppendAssistantChunk(chunk)
	m.messages = m.eqCtx.Transcript.Messages()
	m.refreshTranscript()
}

func (m *Model) finalizeAssistantInTranscript(finalText string) {
	if m.eqCtx.Transcript == nil {
		return
	}
	m.eqCtx.Transcript.FinalizeAssistant(finalText)
	m.messages = m.eqCtx.Transcript.Messages()
	m.refreshTranscript()
}

func (m *Model) loadTranscriptMessages(msgs []agent.Message) {
	if m.eqCtx.Transcript == nil {
		m.eqCtx.Transcript = tuirender.NewTranscript(m.conversationWidth())
	}
	m.eqCtx.Transcript.LoadMessages(msgs)
	m.messages = m.eqCtx.Transcript.Messages()
	m.refreshTranscript()
}

func (m *Model) resetTranscriptMessages() {
	if m.eqCtx.Transcript == nil {
		m.messages = nil
		m.refreshTranscript()
		return
	}
	m.eqCtx.Transcript.Reset()
	m.messages = nil
	m.refreshTranscript()
}

func (m *Model) refreshTranscript() {
	m.transcriptDirty = true
}

func (m *Model) flushTranscript() tea.Cmd {
	if !m.transcriptDirty {
		return nil
	}
	lines, plain := m.renderTranscriptLines()
	m.logConversationSnapshot(plain)
	m.transcriptDirty = false
	width := m.conversationWidth()
	height := m.conversationHeight(width)
	resizeCmd := m.viewport.Resize(width, height)
	setCmd := m.viewport.SetLines(lines)
	if resizeCmd == nil {
		return setCmd
	}
	if setCmd == nil {
		return resizeCmd
	}
	return tea.Batch(resizeCmd, setCmd)
}

func (m *Model) conversationWidth() int {
	width := m.width
	if width <= 0 {
		width = m.viewport.Width
	}
	if width <= 0 {
		width = 80
	}
	return width
}

func (m *Model) conversationHeight(width int) int {
	if m.height <= 0 {
		if m.viewport.Height > 0 {
			return m.viewport.Height
		}
		return 12
	}

	header := m.headerSection(width)
	quick := m.quickHelpSection(width)
	plan := m.planSection(width)
	status := m.statusLine(width)
	queue := renderQueuedPreview(m.queuedMessages, m.pending, width)
	composer := renderPane("Prompt", m.textarea.View(), width, m.textarea.Height())
	hints := renderHints(width)

	bottomParts := []string{}
	if status != "" {
		bottomParts = append(bottomParts, status)
	}
	if queue != "" {
		bottomParts = append(bottomParts, queue)
	}
	bottomParts = append(bottomParts, composer, hints)

	reservedHeight := lipgloss.Height(header) + lipgloss.Height(quick) + lipgloss.Height(plan) + lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, bottomParts...))
	available := m.height - reservedHeight - conversationMarginBottom // renderConversation has a bottom margin
	if available < 3 {
		available = 3
	}
	return available
}

func (m *Model) renderTranscriptLines() ([]string, []string) {
	width := m.viewport.Width
	if width <= 0 {
		width = 80
	}
	var lines []tuirender.Line
	if m.eqCtx.Transcript != nil {
		lines = m.eqCtx.Transcript.RenderViewLines(width)
	} else {
		lines = tuirender.RenderMessages(m.messages, width)
	}
	if len(lines) == 0 {
		lines = []tuirender.Line{{Spans: []tuirender.Span{{Text: "Welcome to Echo (Go). Type a message to start."}}}}
	}
	return tuirender.LinesToStrings(lines), tuirender.LinesToPlainStrings(lines)
}

func (m *Model) logConversationSnapshot(lines []string) {
	if m.conversationLog == nil {
		return
	}
	snapshot := strings.Join(lines, "\n")
	if strings.TrimSpace(snapshot) == "" {
		return
	}
	if snapshot == m.lastConversationSnapshot {
		return
	}
	m.lastConversationSnapshot = snapshot
	m.conversationLog.Infof("conversation snapshot\n%s", snapshot)
}

func (m *Model) copyConversation() {
	if m.eqCtx.Transcript == nil && len(m.messages) == 0 {
		m.logEvent("copy", "conversation is empty")
		return
	}
	width := m.viewport.Width
	if width <= 0 {
		width = 80
	}
	var lines []tuirender.Line
	if m.eqCtx.Transcript != nil {
		lines = m.eqCtx.Transcript.RenderViewLines(width)
	} else {
		lines = tuirender.RenderMessages(m.messages, width)
	}
	plain := tuirender.LinesToPlainStrings(lines)
	text := strings.Join(plain, "\n")
	if strings.TrimSpace(text) == "" {
		m.logEvent("copy", "conversation is empty")
		return
	}
	if err := clipboard.WriteAll(text); err != nil {
		m.err = fmt.Errorf("copy conversation failed: %w", err)
		m.logEvent("copy_error", m.err.Error())
		return
	}
	m.logEvent("copy", "conversation copied to clipboard")
}

func (m *Model) statusLine(width int) string {
	parts := []string{}
	if m.pending {
		elapsed := ""
		if !m.pendingSince.IsZero() {
			secs := int(time.Since(m.pendingSince).Seconds())
			if secs < 0 {
				secs = 0
			}
			elapsed = fmt.Sprintf("%ds", secs)
		}
		label := "Working… " + m.spin.View()
		if elapsed != "" {
			label = fmt.Sprintf("%s (%s)", label, elapsed)
		}
		parts = append(parts, label+" • Esc to interrupt")
	}
	if len(m.queuedMessages) > 0 {
		parts = append(parts, fmt.Sprintf("Queued:%d", len(m.queuedMessages)))
	}
	scrollLabel := "Scroll:PgUp/PgDn"
	if m.viewport.ContentOverflow() {
		scrollLabel = fmt.Sprintf("Scroll:%3d%%", m.viewport.PercentScrolled())
		if !m.viewport.FollowingBottom() {
			scrollLabel += " (锁定视图)"
		}
	}
	parts = append(parts, scrollLabel)
	if m.err != nil {
		parts = append(parts, fmt.Sprintf("Error: %v", m.err))
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D7A85")).
		Padding(0, 1).
		Width(maxInt(20, width)).
		Render(strings.Join(parts, " • "))
}

const (
	tuiVersion               = "v0.65.0"
	conversationMarginBottom = 1
)

func renderSessionCard(model, reasoning, workdir string, width int) string {
	modelText := model
	if reasoning != "" {
		modelText = fmt.Sprintf("%s %s", model, reasoning)
	}
	lines := []string{
		fmt.Sprintf(">_ Echo Team Echo (%s)", tuiVersion),
		"",
		fmt.Sprintf("model:     %s   /model to change", modelText),
		fmt.Sprintf("directory: %s", workdir),
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Width(maxInt(40, width)).
		Render(strings.Join(lines, "\n"))
}

func renderSessionBanner(model, reasoning, workdir string, width int) string {
	modelText := model
	if reasoning != "" {
		modelText = fmt.Sprintf("%s %s", model, reasoning)
	}
	parts := []string{
		fmt.Sprintf("Echo %s", tuiVersion),
		fmt.Sprintf("model %s", modelText),
	}
	if workdir != "" {
		parts = append(parts, workdir)
	}
	parts = append(parts, "Ctrl+T 折叠/展开顶部")
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D7A85")).
		Padding(0, 1).
		Width(maxInt(40, width)).
		Render(strings.Join(parts, " • "))
}

func renderConversation(body string, width int) string {
	style := lipgloss.NewStyle().
		Padding(0, 1).
		MarginBottom(conversationMarginBottom)
	if width > 0 {
		style = style.Width(maxInt(20, width))
	}
	return style.Render(strings.TrimRight(body, "\n"))
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

func renderQuickHelp(width int) string {
	help := []string{
		"Quick actions:",
		"/init 创建 AGENTS.md • /status 查看状态",
		"/model 选择模型 • /sessions 恢复会话 • @ 文件搜索",
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D7A85")).
		Padding(0, 1).
		Width(maxInt(30, width)).
		Render(strings.Join(help, "\n"))
}

func renderHints(width int) string {
	hint := "Enter 发送 • Alt+Enter 换行 • Ctrl+T 折叠顶部 • Ctrl+C 退出 • Ctrl+Y 复制对话 • @ 搜索文件 • ? 帮助 • /sessions 恢复会话"
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D7A85")).
		Padding(0, 1).
		Width(maxInt(20, width)).
		Render(hint)
}

func renderQueuedPreview(queue []string, pending bool, width int) string {
	if len(queue) == 0 || width <= 4 || !pending {
		return ""
	}
	limit := len(queue)
	if limit > 3 {
		limit = 3
	}
	lines := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		text := strings.ReplaceAll(queue[i], "\n", " ")
		if len(text) > width-4 && width > 4 {
			text = text[:width-7] + "..."
		}
		lines = append(lines, fmt.Sprintf("↳ %s", text))
	}
	if len(queue) > limit {
		lines = append(lines, fmt.Sprintf("… %d more queued", len(queue)-limit))
	}
	hint := lipgloss.NewStyle().Faint(true).Render("Alt+↑ edit queued")
	lines = append(lines, hint)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D7A85")).
		Italic(true).
		Padding(0, 1).
		Width(maxInt(20, width)).
		Render(strings.Join(lines, "\n"))
}

func (m *Model) handleScrollKeys(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyPgUp:
		return m.viewport.ScrollPageUp(), true
	case tea.KeyPgDown:
		return m.viewport.ScrollPageDown(), true
	case tea.KeySpace:
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
	action := m.resolveSlashSubmit(input)
	switch action.Kind {
	case slash.ActionSubmitCommand:
		return m.executeSlashCommand(action.Command, action.Args)
	case slash.ActionSubmitPrompt:
		return m.submitSlashPrompt(action)
	case slash.ActionInsert:
		if action.NewValue != "" {
			m.textarea.SetValue(action.NewValue)
			m.moveCursorToColumn(action.CursorColumn)
			m.setComposerHeight()
		}
	case slash.ActionError:
		if action.Message != "" {
			m.appendAssistantMessage(action.Message)
		}
	}
	return nil
}

func (m *Model) executeSlashCommand(cmd slash.Command, args string) tea.Cmd {
	switch cmd {
	case slash.CommandQuit, slash.CommandExit:
		return tea.Quit
	case slash.CommandClear:
		m.resetTranscriptMessages()
		return nil
	case slash.CommandStatus:
		info := fmt.Sprintf("model=%s dir=%s", m.modelName, m.workdir)
		m.appendAssistantMessage(info)
		return nil
	case slash.CommandSessions:
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
	case slash.CommandModel:
		if arg := firstArg(args); arg != "" {
			m.modelName = arg
		}
		m.appendAssistantMessage(fmt.Sprintf("using model %s", m.modelName))
		return nil
	case slash.CommandInit:
		return m.handleInitCommand()
	case slash.CommandResume:
		rec, err := session.Last()
		if err != nil {
			return func() tea.Msg { return systemMsg{Text: fmt.Sprintf("resume error: %v", err)} }
		}
		m.resumeSessionID = rec.ID
		m.eqCtx.SessionID = rec.ID
		m.loadTranscriptMessages(rec.Messages)
		return nil
	case slash.CommandDiff:
		return m.runTool(tools.ToolRequest{
			ID:      "local-diff",
			Kind:    tools.ToolCommand,
			Command: "git diff --stat",
		})
	case slash.CommandRun:
		if args == "" {
			m.appendAssistantMessage("usage: /run <command>")
			return nil
		}
		return m.runTool(tools.ToolRequest{
			ID:      "local-run",
			Kind:    tools.ToolCommand,
			Command: args,
		})
	case slash.CommandApply:
		if args == "" {
			m.appendAssistantMessage("usage: /apply <patch-file>")
			return nil
		}
		patchPath := firstArg(args)
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
	case slash.CommandAttach:
		if args == "" {
			m.appendAssistantMessage("usage: /attach <file>")
			return nil
		}
		target := firstArg(args)
		display := target
		if !filepath.IsAbs(target) && m.workdir != "" {
			target = filepath.Join(m.workdir, target)
		}
		data, err := os.ReadFile(target)
		if err != nil {
			return func() tea.Msg { return systemMsg{Text: fmt.Sprintf("attach failed: %v", err)} }
		}
		m.appendUserMessage(fmt.Sprintf("Attachment %s:\n%s", display, string(data)))
		return nil
	case slash.CommandMention:
		return m.loadSearch()
	case slash.CommandFeedback:
		msg := "feedback captured (local only); thank you!"
		if strings.TrimSpace(args) != "" {
			msg = strings.TrimSpace(strings.TrimPrefix(args, "/feedback "))
		}
		m.appendAssistantMessage(msg)
		return nil
	case slash.CommandNew:
		m.resetSession()
		m.appendAssistantMessage("Started a new session.")
		return nil
	case slash.CommandReview:
		m.reviewMode = true
		m.appendAssistantMessage("Review mode enabled for subsequent turns.")
		return nil
	case slash.CommandCompact:
		cmd := m.toggleChrome()
		if m.chromeCollapsed {
			m.appendAssistantMessage("已折叠顶部信息，留出更多会话空间。")
		} else {
			m.appendAssistantMessage("已恢复完整顶部信息。")
		}
		return cmd
	case slash.CommandUndo:
		m.appendAssistantMessage("undo is not available in this build.")
		return nil
	case slash.CommandMCP:
		m.appendAssistantMessage("MCP UI not implemented in Go TUI yet.")
		return nil
	case slash.CommandLogout:
		m.appendAssistantMessage("Use `echo-cli logout` to clear credentials.")
		return nil
	case slash.CommandSkills:
		m.appendAssistantMessage("No skills metadata available.")
		return nil
	case slash.CommandRollout:
		m.appendAssistantMessage("Debug-only command not supported in this build.")
		return nil
	}
	return nil
}

func (m *Model) toggleChrome() tea.Cmd {
	m.chromeCollapsed = !m.chromeCollapsed
	state := "展开顶部信息"
	if m.chromeCollapsed {
		state = "折叠顶部信息"
	}
	m.logEvent("layout", state)
	cmds := m.resize(m.width, m.height)
	m.refreshTranscript()
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) resetSession() {
	m.resetTranscriptMessages()
	m.streamIdx = -1
	m.pending = false
	m.pendingSince = time.Time{}
	m.activeSub = ""
	m.eqCtx.ActiveSub = ""
	m.queuedMessages = nil
	m.resumeSessionID = ""
	m.eqCtx.SessionID = ""
	m.reviewMode = false
	m.planUpdate = nil
}

func firstArg(args string) string {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func (m *Model) runTool(req tools.ToolRequest) tea.Cmd {
	return func() tea.Msg {
		if m.toolRuntime != nil {
			_, _ = m.toolRuntime.Dispatch(context.Background(), req.ToCall(), func(ev tools.ToolEvent) {
				m.handleBusEvent(ev)
			})
		}
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
