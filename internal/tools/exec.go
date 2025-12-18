package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RunCommand executes a shell command in the given directory (no sandbox/approvals).
func RunCommand(ctx context.Context, workdir string, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("empty command")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	if workdir != "" {
		cmd.Dir = workdir
	}
	cmd.Env = os.Environ()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdin = bytes.NewReader(nil) // 立刻 EOF，避免等待 stdin 导致挂死
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String() + stderr.String()
	if err != nil {
		return out, fmt.Errorf("command failed: %w", err)
	}
	return out, nil
}

// RunWrapped executes a prepared *exec.Cmd and returns combined output and exit code.
func RunWrapped(cmd *exec.Cmd) (string, int, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String() + stderr.String()
	if err != nil {
		return out, exitCode(err), err
	}
	return out, 0, nil
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func appendEnvIfMissing(env []string, key, value string) []string {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return env
		}
	}
	return append(env, prefix+value)
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			out := append([]string{}, env...)
			out[i] = prefix + value
			return out
		}
	}
	return append(env, prefix+value)
}
