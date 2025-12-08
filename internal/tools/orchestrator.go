package tools

import (
	"context"
	"fmt"

	"echo-cli/internal/policy"
)

// Orchestrator 负责审批与状态事件封装，抽象对齐 Codex 的“工具执行管线”。
type Orchestrator struct {
	policy    policy.Policy
	approver  Approver
	onFailure map[ToolKind]bool
}

func NewOrchestrator(pol policy.Policy, approver Approver) *Orchestrator {
	return &Orchestrator{
		policy:    pol,
		approver:  approver,
		onFailure: make(map[ToolKind]bool),
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

	if o.policy.ApprovalPolicy == "on-failure" {
		o.onFailure[result.Kind] = result.Status == "error"
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
	if inv.Policy.ApprovalPolicy == "on-failure" && o.onFailure[handler.Kind()] {
		dec.RequiresApproval = true
		dec.Allowed = false
		dec.Reason = "prior failure in on-failure policy"
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
