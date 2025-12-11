package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"echo-cli/internal/policy"
)

// Runtime 协调路由、审批与并行控制。
type Runtime struct {
	registry     *Registry
	orchestrator *Orchestrator
	workdir      string
	policy       policy.Policy
	runner       Runner
	lock         sync.RWMutex
}

func NewRuntime(pol policy.Policy, runner Runner, approver Approver, workdir string, handlers []Handler) *Runtime {
	registry := NewRegistry(handlers...)

	return &Runtime{
		registry:     registry,
		orchestrator: NewOrchestrator(pol, approver),
		workdir:      workdir,
		policy:       pol,
		runner:       runner,
	}
}

func (r *Runtime) Dispatch(ctx context.Context, call ToolCall, emit func(ToolEvent)) (ToolResult, error) {
	handler, ok := r.registry.Handler(call.Name)
	kind := ToolKind("unknown")
	if ok {
		kind = handler.Kind()
	}
	logToolRequest(call, kind, ok, r.policy, r.workdir)
	if !ok {
		res := ToolResult{ID: call.ID, Status: "error", Error: "unknown tool", Kind: ToolKind("unknown")}
		emit(ToolEvent{Type: "item.completed", Result: res})
		return res, fmt.Errorf("unknown tool: %s", call.Name)
	}

	inv := Invocation{
		Call:    call,
		Workdir: r.workdir,
		Policy:  r.policy,
		Runner:  r.runner,
	}

	if handler.SupportsParallel() {
		r.lock.RLock()
		defer r.lock.RUnlock()
	} else {
		r.lock.Lock()
		defer r.lock.Unlock()
	}

	result := r.orchestrator.Run(ctx, inv, handler, emit)
	return result, nil
}

func logToolRequest(call ToolCall, kind ToolKind, recognized bool, pol policy.Policy, workdir string) {
	ensureToolsLogger()

	status := "received"
	if !recognized {
		status = "unknown"
	}
	payload := "(empty)"
	if len(call.Payload) > 0 {
		payload = sanitizePayload(call.Payload)
	}
	wd := workdir
	if strings.TrimSpace(wd) == "" {
		wd = "."
	}
	toolsLog.Infof("tool_call id=%s name=%s kind=%s status=%s sandbox=%s approval=%s workdir=%s payload=%s",
		call.ID, call.Name, kind, status, pol.SandboxMode, pol.ApprovalPolicy, wd, payload)
}

func sanitizePayload(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "(empty)"
	}
	text = strings.ReplaceAll(text, "\n", `\n`)
	text = strings.ReplaceAll(text, "\r", `\r`)
	return text
}
