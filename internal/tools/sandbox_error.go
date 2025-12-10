package tools

import "errors"

// SandboxError 标记因沙箱或权限不足导致的拒绝，用于触发审批或提权重试。
type SandboxError struct {
	Reason string
}

func (e SandboxError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return "sandbox denied"
}

func (e SandboxError) Is(target error) bool {
	_, ok := target.(SandboxError)
	return ok
}

// IsSandboxDenied 返回错误是否为沙箱拒绝，用于 orchestrator 决定是否申请提权。
func IsSandboxDenied(err error) bool {
	var se SandboxError
	return errors.As(err, &se)
}
