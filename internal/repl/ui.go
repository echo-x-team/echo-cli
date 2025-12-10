package repl

import (
	"echo-cli/internal/agent"
	"echo-cli/internal/events"
	"echo-cli/internal/execution"
	"echo-cli/internal/policy"
	"echo-cli/internal/sandbox"
	"echo-cli/internal/tools/engine"
	"echo-cli/internal/tui"
	"echo-cli/internal/tui/slash"
)

// UIOptions 描述启动 TUI 所需的依赖与初始状态。
type UIOptions struct {
	Engine          *execution.Engine
	Gateway         *Gateway
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
	Approver        *engine.UIApprover
	ResumePicker    bool
	ResumeShowAll   bool
	ResumeSessions  []string
	ResumeSessionID string
	CustomPrompts   []slash.CustomPrompt
	SkillsAvailable bool
	Debug           bool
}

// UIResult 返回 TUI 退出时的历史与状态。
type UIResult struct {
	History      []agent.Message
	SessionID    string
	UpdateAction string
}

// RunUI 启动 Bubble Tea 界面并返回结果。
func RunUI(opts UIOptions) (UIResult, error) {
	res, err := tui.Run(tui.Options{
		Engine:          opts.Engine,
		Gateway:         opts.Gateway,
		Model:           opts.Model,
		Reasoning:       opts.Reasoning,
		Sandbox:         opts.Sandbox,
		Workdir:         opts.Workdir,
		InitialPrompt:   opts.InitialPrompt,
		Language:        opts.Language,
		InitialMessages: opts.InitialMessages,
		Roots:           opts.Roots,
		Policy:          opts.Policy,
		Events:          opts.Events,
		Runner:          opts.Runner,
		Approver:        opts.Approver,
		ResumePicker:    opts.ResumePicker,
		ResumeShowAll:   opts.ResumeShowAll,
		ResumeSessions:  opts.ResumeSessions,
		ResumeSessionID: opts.ResumeSessionID,
		CustomPrompts:   opts.CustomPrompts,
		SkillsAvailable: opts.SkillsAvailable,
		Debug:           opts.Debug,
	})
	if err != nil {
		return UIResult{}, err
	}
	return UIResult{
		History:      res.History,
		SessionID:    res.SessionID,
		UpdateAction: res.UpdateAction,
	}, nil
}
