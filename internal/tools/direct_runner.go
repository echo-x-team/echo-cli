package tools

import (
	"context"
)

// DirectRunner 直接在宿主环境执行命令与补丁应用（无沙箱/无审批）。
type DirectRunner struct{}

func (DirectRunner) RunCommand(ctx context.Context, workdir string, command string) (string, int, error) {
	out, err := RunCommand(ctx, workdir, command)
	if err != nil {
		return out, exitCode(err), err
	}
	return out, 0, nil
}

func (DirectRunner) ApplyPatch(ctx context.Context, workdir string, diff string) error {
	return ApplyPatch(ctx, workdir, diff)
}
