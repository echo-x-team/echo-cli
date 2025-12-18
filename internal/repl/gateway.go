package repl

import (
	"context"
	"errors"

	"echo-cli/internal/events"
)

// Gateway 暴露 REPL 层可用的 SQ/EQ 接口：提交用户输入、订阅事件。
type Gateway struct {
	manager *events.Manager
}

// NewGateway 创建基于共享 events.Manager 的网关。
func NewGateway(manager *events.Manager) *Gateway {
	return &Gateway{manager: manager}
}

func (g *Gateway) managerOrErr() (*events.Manager, error) {
	if g.manager == nil {
		return nil, errors.New("repl gateway manager not configured")
	}
	return g.manager, nil
}

// SubmitUserInput 投递用户输入到 SQ。
func (g *Gateway) SubmitUserInput(ctx context.Context, items []events.InputMessage, inputCtx events.InputContext) (string, error) {
	mgr, err := g.managerOrErr()
	if err != nil {
		return "", err
	}
	return mgr.SubmitUserInput(ctx, items, inputCtx)
}

// SubmitInterrupt 投递中断请求到 SQ。
func (g *Gateway) SubmitInterrupt(ctx context.Context, sessionID string) (string, error) {
	mgr, err := g.managerOrErr()
	if err != nil {
		return "", err
	}
	return mgr.Submit(ctx, events.Submission{
		SessionID: sessionID,
		Operation: events.Operation{Kind: events.OperationInterrupt},
	})
}

// SubmitApprovalDecision 投递审批结果到 SQ。
func (g *Gateway) SubmitApprovalDecision(ctx context.Context, sessionID string, approvalID string, approved bool) (string, error) {
	mgr, err := g.managerOrErr()
	if err != nil {
		return "", err
	}
	return mgr.Submit(ctx, events.Submission{
		SessionID: sessionID,
		Operation: events.Operation{
			Kind: events.OperationApprovalDecision,
			ApprovalDecision: &events.ApprovalDecisionOperation{
				ApprovalID: approvalID,
				Approved:   approved,
			},
		},
	})
}

// Events 返回 EQ 事件订阅。
func (g *Gateway) Events() <-chan events.Event {
	if g.manager == nil {
		return nil
	}
	return g.manager.Subscribe()
}
