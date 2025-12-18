package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunCommand_AutoRespondsToNpmInstallPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 模拟 npm 在首次执行 npm create 时的确认提示。
	cmd := `printf "Need to install the following packages:\ncreate-vue@3.18.3\nOk to proceed?" ; IFS= read -r ans < /dev/tty ; echo "ANSWER=$ans"`
	out, err := RunCommand(ctx, "", cmd)
	if err != nil {
		t.Fatalf("RunCommand failed: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "ANSWER=y") {
		t.Fatalf("expected auto response y, got out=%q", out)
	}
}
