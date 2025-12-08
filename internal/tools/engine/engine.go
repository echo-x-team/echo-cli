package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"echo-cli/internal/policy"
	"echo-cli/internal/sandbox"
	"echo-cli/internal/search"
	"echo-cli/internal/tools"
)

type Approver interface {
	Approve(req tools.ToolRequest) bool
}

type Engine struct {
	runner    sandbox.Runner
	policy    policy.Policy
	approver  Approver
	workdir   string
	onFailure map[tools.ToolKind]bool
}

func New(policy policy.Policy, runner sandbox.Runner, approver Approver, workdir string) *Engine {
	return &Engine{
		policy:    policy,
		runner:    runner,
		approver:  approver,
		workdir:   workdir,
		onFailure: make(map[tools.ToolKind]bool),
	}
}

func (e *Engine) Run(ctx context.Context, req tools.ToolRequest, emit func(tools.ToolEvent)) {
	switch req.Kind {
	case tools.ToolCommand:
		e.handleCommand(ctx, req, emit)
	case tools.ToolApplyPatch:
		e.handlePatch(ctx, req, emit)
	case tools.ToolFileRead:
		e.handleFileRead(ctx, req, emit)
	case tools.ToolSearch:
		e.handleSearch(ctx, req, emit)
	default:
		emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "error", Error: "unknown tool"}})
	}
}

func (e *Engine) handleCommand(ctx context.Context, req tools.ToolRequest, emit func(tools.ToolEvent)) {
	dec := e.policy.AllowCommand()
	if !e.resolveApproval(req, dec, emit) {
		return
	}
	emit(tools.ToolEvent{Type: "item.started", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "started", Command: req.Command}})
	out, code, err := e.runner.RunCommand(ctx, e.workdir, req.Command)
	if err != nil {
		if e.policy.ApprovalPolicy == "on-failure" {
			e.onFailure[req.Kind] = true
		}
		emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "error", Error: err.Error(), Command: req.Command, ExitCode: code}})
		return
	}
	if e.policy.ApprovalPolicy == "on-failure" {
		e.onFailure[req.Kind] = false
	}
	emit(tools.ToolEvent{Type: "item.updated", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "updated", Command: req.Command, Output: out, ExitCode: code}})
	emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "completed", Command: req.Command, Output: out, ExitCode: code}})
}

func (e *Engine) handlePatch(ctx context.Context, req tools.ToolRequest, emit func(tools.ToolEvent)) {
	dec := e.policy.AllowWrite()
	if !e.resolveApproval(req, dec, emit) {
		return
	}
	emit(tools.ToolEvent{Type: "item.started", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "started", Path: req.Path}})
	if err := e.runner.ApplyPatch(ctx, e.workdir, req.Patch); err != nil {
		if e.policy.ApprovalPolicy == "on-failure" {
			e.onFailure[req.Kind] = true
		}
		emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "error", Error: err.Error(), Path: req.Path}})
		return
	}
	if e.policy.ApprovalPolicy == "on-failure" {
		e.onFailure[req.Kind] = false
	}
	emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "completed", Path: req.Path}})
}

func (e *Engine) handleFileRead(ctx context.Context, req tools.ToolRequest, emit func(tools.ToolEvent)) {
	dec := e.policy.AllowCommand()
	if !dec.Allowed {
		emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "error", Error: dec.Reason}})
		return
	}
	target := req.Path
	if !filepath.IsAbs(target) && e.workdir != "" {
		target = filepath.Join(e.workdir, target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "error", Error: err.Error(), Path: req.Path}})
		return
	}
	emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "completed", Output: string(data), Path: req.Path}})
}

func (e *Engine) handleSearch(ctx context.Context, req tools.ToolRequest, emit func(tools.ToolEvent)) {
	root := e.workdir
	if root == "" {
		root = "."
	}
	paths, err := search.FindFiles(root, 200)
	if err != nil {
		emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "error", Error: err.Error()}})
		return
	}
	emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "completed", Output: strings.Join(paths, "\n")}})
}

func (e *Engine) resolveApproval(req tools.ToolRequest, dec policy.Decision, emit func(tools.ToolEvent)) bool {
	if e.policy.ApprovalPolicy == "on-failure" && e.onFailure[req.Kind] {
		dec.RequiresApproval = true
		dec.Allowed = false
		dec.Reason = "prior failure in on-failure policy"
	}
	if dec.Allowed {
		return true
	}
	if dec.RequiresApproval {
		result := tools.ToolResult{ID: req.ID, Kind: req.Kind, Command: req.Command, Path: req.Path}
		emit(tools.ToolEvent{Type: "approval.requested", Result: result, Reason: dec.Reason})
		if e.approver != nil && e.approver.Approve(req) {
			emit(tools.ToolEvent{Type: "approval.completed", Result: result, Reason: "approved"})
			return true
		}
		denyReason := fmt.Sprintf("denied: %s", dec.Reason)
		emit(tools.ToolEvent{Type: "approval.completed", Result: result, Reason: denyReason})
		emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{
			ID:     req.ID,
			Kind:   req.Kind,
			Status: "error",
			Error:  denyReason,
			Path:   req.Path,
		}})
		return false
	}
	emit(tools.ToolEvent{Type: "item.completed", Result: tools.ToolResult{ID: req.ID, Kind: req.Kind, Status: "error", Error: dec.Reason}})
	return false
}
