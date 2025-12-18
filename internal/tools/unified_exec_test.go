package tools

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestUnifiedExecManager_InteractiveSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mgr := NewUnifiedExecManager()
	res, err := mgr.ExecCommand(ctx, ExecCommandSpec{
		Command:        `printf "Name: "; read -r name; echo "NAME=$name"`,
		BaseEnv:        os.Environ(),
		YieldTime:      200 * time.Millisecond,
		MaxOutputBytes: 64 * 1024,
	})
	if err != nil {
		t.Fatalf("ExecCommand failed: %v (out=%q)", err, res.Output)
	}
	if strings.TrimSpace(res.SessionID) == "" {
		t.Fatalf("expected session_id, got %+v", res)
	}
	if !strings.Contains(res.Output, "Name:") {
		t.Fatalf("expected prompt output, got %q", res.Output)
	}

	res2, err := mgr.WriteStdin(ctx, WriteStdinSpec{
		SessionID:      res.SessionID,
		Chars:          "bob\n",
		YieldTime:      200 * time.Millisecond,
		MaxOutputBytes: 64 * 1024,
	})
	if err != nil {
		t.Fatalf("WriteStdin failed: %v (out=%q)", err, res2.Output)
	}
	if res2.ExitCode == nil || *res2.ExitCode != 0 {
		t.Fatalf("expected exit_code=0, got %+v", res2)
	}
	if strings.TrimSpace(res2.SessionID) != "" {
		t.Fatalf("expected session to close, got %+v", res2)
	}
	if !strings.Contains(res2.Output, "NAME=bob") {
		t.Fatalf("expected echoed name, got %q", res2.Output)
	}
}

func TestUnifiedExecManager_SessionSurvivesCallerContextCancel(t *testing.T) {
	// ExecCommand 的 ctx 仅用于本次调用的等待/收集输出；会话进程应继续运行，
	// 不能因为调用方 ctx 被取消就被 CommandContext kill。
	ctx, cancel := context.WithCancel(context.Background())

	mgr := NewUnifiedExecManager()
	res, err := mgr.ExecCommand(ctx, ExecCommandSpec{
		Command:        `printf "Name: "; read -r name; echo "NAME=$name"`,
		BaseEnv:        os.Environ(),
		YieldTime:      200 * time.Millisecond,
		MaxOutputBytes: 64 * 1024,
	})
	if err != nil {
		cancel()
		t.Fatalf("ExecCommand failed: %v (out=%q)", err, res.Output)
	}
	if strings.TrimSpace(res.SessionID) == "" {
		cancel()
		t.Fatalf("expected session_id, got %+v", res)
	}
	if !strings.Contains(res.Output, "Name:") {
		cancel()
		t.Fatalf("expected prompt output, got %q", res.Output)
	}

	cancel()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	res2, err := mgr.WriteStdin(ctx2, WriteStdinSpec{
		SessionID:      res.SessionID,
		Chars:          "alice\n",
		YieldTime:      200 * time.Millisecond,
		MaxOutputBytes: 64 * 1024,
	})
	if err != nil {
		t.Fatalf("WriteStdin failed after ctx cancel: %v (out=%q)", err, res2.Output)
	}
	if res2.ExitCode == nil || *res2.ExitCode != 0 {
		t.Fatalf("expected exit_code=0, got %+v", res2)
	}
	if strings.TrimSpace(res2.SessionID) != "" {
		t.Fatalf("expected session to close, got %+v", res2)
	}
	if !strings.Contains(res2.Output, "NAME=alice") {
		t.Fatalf("expected echoed name, got %q", res2.Output)
	}
}
