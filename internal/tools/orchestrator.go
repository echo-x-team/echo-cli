package tools

import (
	"context"
	"fmt"
	"strings"
)

type Orchestrator struct {
	reviewer  CommandReviewer
	approvals *ApprovalStore
}

type OrchestratorOptions struct {
	Reviewer  CommandReviewer
	Approvals *ApprovalStore
}

func NewOrchestrator() *Orchestrator { return &Orchestrator{} }

func NewOrchestratorWith(opts OrchestratorOptions) *Orchestrator {
	return &Orchestrator{reviewer: opts.Reviewer, approvals: opts.Approvals}
}

func (o *Orchestrator) Run(ctx context.Context, inv Invocation, handler Handler, emit func(ToolEvent)) ToolResult {
	base := handler.Describe(inv)
	base.ID = inv.Call.ID
	base.Kind = handler.Kind()

	emit(ToolEvent{
		Type:   "item.started",
		Result: base,
	})

	if o != nil && o.shouldRequireApproval(handler) {
		if err := o.waitForApproval(ctx, inv, base, emit); err != nil {
			result := ToolResult{
				ID:       inv.Call.ID,
				Kind:     handler.Kind(),
				Status:   "error",
				Error:    err.Error(),
				Command:  base.Command,
				ExitCode: -1,
			}
			emit(ToolEvent{Type: "item.completed", Result: result})
			return result
		}
	}

	result, err := handler.Handle(ctx, inv)
	result = normalizeResult(result, err, inv, handler)

	emit(ToolEvent{
		Type:   "item.completed",
		Result: result,
	})
	return result
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

func (o *Orchestrator) shouldRequireApproval(handler Handler) bool {
	if o == nil || o.reviewer == nil {
		return false
	}
	// 对齐 codex：只对“启动一个新命令”的入口做安全审查；write_stdin 不重复审查。
	return handler.Kind() == ToolCommand && handler.Name() == "exec_command"
}

func (o *Orchestrator) waitForApproval(ctx context.Context, inv Invocation, base ToolResult, emit func(ToolEvent)) error {
	if o == nil || o.reviewer == nil {
		return nil
	}
	approvalID := inv.Call.ID

	review, err := o.reviewer.Review(ctx, inv.Workdir, base.Command)
	if err != nil {
		// Fail closed: 审查失败时要求人工审批。
		review = CommandReview{
			RiskLevel:   "high",
			Description: fmt.Sprintf("审查失败，默认按高风险处理：%v", err),
		}
	}
	if strings.ToLower(strings.TrimSpace(review.RiskLevel)) != "high" {
		return nil
	}
	if o.approvals == nil {
		return fmt.Errorf("approval required but approval store not configured")
	}

	msg := strings.TrimSpace(review.Description)
	if msg == "" {
		msg = "命令被判定为高风险，需要人工审批"
	}
	emit(ToolEvent{
		Type: "item.updated",
		Result: ToolResult{
			ID:             inv.Call.ID,
			Kind:           handlerKindFallback(base.Kind, ToolCommand),
			Status:         "requires_approval",
			Command:        base.Command,
			ApprovalID:     approvalID,
			ApprovalReason: "risk_level=high: " + msg,
			Output:         "approval_required: risk_level=high: " + msg,
		},
	})

	approved, err := o.approvals.Wait(ctx, approvalID)
	if err != nil {
		return err
	}
	if !approved {
		return fmt.Errorf("approval denied")
	}
	emit(ToolEvent{
		Type: "item.updated",
		Result: ToolResult{
			ID:         inv.Call.ID,
			Kind:       handlerKindFallback(base.Kind, ToolCommand),
			Status:     "approved",
			Command:    base.Command,
			ApprovalID: approvalID,
		},
	})
	return nil
}

func handlerKindFallback(got ToolKind, fallback ToolKind) ToolKind {
	if got != "" {
		return got
	}
	return fallback
}
