package tools

import (
	"context"
	"fmt"

	"echo-cli/internal/policy"
)

// Orchestrator 负责审批与状态事件封装，抽象对齐 Codex 的“工具执行管线”。
type Orchestrator struct {
	policy   policy.Policy
	approver Approver
}

func NewOrchestrator(pol policy.Policy, approver Approver) *Orchestrator {
	return &Orchestrator{
		policy:   pol,
		approver: approver,
	}
}

func (o *Orchestrator) Run(ctx context.Context, inv Invocation, handler Handler, emit func(ToolEvent)) ToolResult {
	base := handler.Describe(inv)
	base.ID = inv.Call.ID
	base.Kind = handler.Kind()

	dec := o.decision(handler, inv)
	if !o.resolveApproval(inv.Call, base, dec, emit) {
		return base
	}

	emit(ToolEvent{
		Type:   "item.started",
		Result: base,
	})

	result, err := handler.Handle(ctx, inv)
	result = normalizeResult(result, err, inv, handler)

	// 沙箱拒绝或 on-failure 策略下的失败，支持审批后提权重试。
	if o.shouldRetryWithoutSandbox(err, inv.Policy) {
		reason := "failed in sandbox"
		if err != nil {
			reason = err.Error()
		}
		emit(ToolEvent{
			Type:   "approval.requested",
			Result: base,
			Reason: fmt.Sprintf("retry without sandbox? %s", reason),
		})
		approved := false
		if o.approver != nil {
			approved = o.approver.Approve(inv.Call)
		}
		status := "denied"
		if approved {
			status = "approved"
		}
		emit(ToolEvent{
			Type:   "approval.completed",
			Result: base,
			Reason: status,
		})
		if approved {
			emit(ToolEvent{
				Type: "item.updated",
				Result: ToolResult{
					ID:     base.ID,
					Kind:   base.Kind,
					Status: "retrying without sandbox",
				},
			})
			escalated := inv
			escalated.Policy.SandboxMode = "danger-full-access"
			if inv.Runner != nil {
				escalated.Runner = inv.Runner.WithMode("danger-full-access")
			}
			result, err = handler.Handle(ctx, escalated)
			result = normalizeResult(result, err, escalated, handler)
		}
	}

	emit(ToolEvent{
		Type:   "item.completed",
		Result: result,
	})
	return result
}

func (o *Orchestrator) decision(handler Handler, inv Invocation) policy.Decision {
	dec := policy.Decision{Allowed: true}
	switch handler.Kind() {
	case ToolApplyPatch:
		dec = inv.Policy.AllowWrite()
	case ToolPlanUpdate:
		dec = policy.Decision{Allowed: true}
	default:
		if handler.IsMutating(inv) {
			dec = inv.Policy.AllowCommand()
		} else {
			dec = inv.Policy.AllowCommand()
		}
	}
	return dec
}

func (o *Orchestrator) resolveApproval(call ToolCall, base ToolResult, dec policy.Decision, emit func(ToolEvent)) bool {
	if dec.Allowed {
		return true
	}

	if dec.RequiresApproval {
		emit(ToolEvent{
			Type:   "approval.requested",
			Result: base,
			Reason: dec.Reason,
		})
		approved := false
		if o.approver != nil {
			approved = o.approver.Approve(call)
		}
		reason := "approved"
		if !approved {
			reason = fmt.Sprintf("denied: %s", dec.Reason)
		}
		emit(ToolEvent{
			Type:   "approval.completed",
			Result: base,
			Reason: reason,
		})
		if approved {
			return true
		}
		emit(ToolEvent{
			Type: "item.completed",
			Result: ToolResult{
				ID:     base.ID,
				Kind:   base.Kind,
				Status: "error",
				Error:  reason,
			},
		})
		return false
	}

	emit(ToolEvent{
		Type: "item.completed",
		Result: ToolResult{
			ID:     base.ID,
			Kind:   base.Kind,
			Status: "error",
			Error:  dec.Reason,
		},
	})
	return false
}

func (o *Orchestrator) shouldRetryWithoutSandbox(err error, pol policy.Policy) bool {
	if err == nil {
		return false
	}
	if pol.ApprovalPolicy == "never" {
		return false
	}
	if pol.SandboxMode == "danger-full-access" {
		return false
	}
	if IsSandboxDenied(err) {
		return true
	}
	return pol.ApprovalPolicy == "on-failure"
}

func normalizeResult(result ToolResult, err error, inv Invocation, handler Handler) ToolResult {
	result.ID = inv.Call.ID
	result.Kind = handler.Kind()

	if err != nil && result.Error == "" {
		result.Error = err.Error()
	}
	if result.Status == "" {
		if result.Error != "" {
			result.Status = "error"
		} else {
			result.Status = "completed"
		}
	}
	return result
}
