package tools

import (
	"context"
	"fmt"
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

func NewRuntime(pol policy.Policy, runner Runner, approver Approver, workdir string) *Runtime {
	registry := NewRegistry(
		CommandHandler{},
		ApplyPatchHandler{},
		FileReadHandler{},
		FileSearchHandler{},
		PlanHandler{},
	)

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
