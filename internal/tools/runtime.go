package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Runtime 协调路由与并行控制。
type Runtime struct {
	registry     *Registry
	orchestrator *Orchestrator
	workdir      string
	runner       Runner
	lock         sync.RWMutex
}

func NewRuntime(runner Runner, workdir string, handlers []Handler) *Runtime {
	registry := NewRegistry(handlers...)

	return &Runtime{
		registry:     registry,
		orchestrator: NewOrchestrator(),
		workdir:      workdir,
		runner:       runner,
	}
}

func (r *Runtime) Dispatch(ctx context.Context, call ToolCall, emit func(ToolEvent)) (ToolResult, error) {
	handler, ok := r.registry.Handler(call.Name)
	kind := ToolKind("unknown")
	if ok {
		kind = handler.Kind()
	}
	logToolRequest(call, kind, ok, r.workdir)
	if !ok {
		res := ToolResult{ID: call.ID, Status: "error", Error: "unknown tool", Kind: ToolKind("unknown")}
		emit(ToolEvent{Type: "item.completed", Result: res})
		logToolResult(call, kind, res, r.workdir)
		return res, fmt.Errorf("unknown tool: %s", call.Name)
	}

	inv := Invocation{
		Call:    call,
		Workdir: r.workdir,
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
	logToolResult(call, kind, result, r.workdir)
	return result, nil
}

func logToolRequest(call ToolCall, kind ToolKind, recognized bool, workdir string) {
	ensureToolsLogger()

	status := "received"
	if !recognized {
		status = "unknown"
	}
	payload := "(empty)"
	if len(call.Payload) > 0 {
		payload = sanitizeForLog(call.Payload)
	}
	wd := workdir
	if strings.TrimSpace(wd) == "" {
		wd = "."
	}
	toolsLog.Infof("tool_call id=%s name=%s kind=%s status=%s workdir=%s payload=%s",
		call.ID, call.Name, kind, status, wd, payload)
}

func logToolResult(call ToolCall, kind ToolKind, result ToolResult, workdir string) {
	ensureToolsLogger()

	payload := "(empty)"
	if len(call.Payload) > 0 {
		payload = sanitizeForLog(call.Payload)
	}
	wd := workdir
	if strings.TrimSpace(wd) == "" {
		wd = "."
	}
	errText := sanitizeForLog([]byte(result.Error))
	if strings.TrimSpace(errText) == "" {
		errText = "(empty)"
	}
	toolsLog.Infof("tool_result id=%s name=%s kind=%s status=%s workdir=%s exit_code=%d error=%s payload=%s",
		call.ID, call.Name, kind, result.Status, wd, result.ExitCode, errText, payload)
}

func sanitizeForLog(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "(empty)"
	}
	text = strings.ReplaceAll(text, "\n", `\n`)
	text = strings.ReplaceAll(text, "\r", `\r`)
	return text
}
