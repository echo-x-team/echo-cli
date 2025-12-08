package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ApplyPatch runs the system `patch` command with unified diff content.
func ApplyPatch(ctx context.Context, workdir string, diff string) error {
	if strings.TrimSpace(diff) == "" {
		return fmt.Errorf("empty patch content")
	}
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "patch", "-p0", "--force")
	if workdir != "" {
		cmd.Dir = workdir
	}
	cmd.Stdin = bytes.NewBufferString(diff)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("apply patch failed: %w: %s", err, stderr.String())
	}
	return nil
}
