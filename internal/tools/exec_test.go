package tools

import (
	"context"
	"testing"
	"time"
)

func TestRunCommand_NonInteractiveUsesStdinEOF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := RunCommand(ctx, "", `bash -lc 'read -r ans; echo "ANSWER=$ans"'`)
	if err != nil {
		t.Fatalf("RunCommand failed: %v (out=%q)", err, out)
	}
	if out != "ANSWER=\n" && out != "ANSWER=\r\n" {
		t.Fatalf("expected stdin EOF, got out=%q", out)
	}
}
