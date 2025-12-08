package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
)

// RunCommand executes a shell command in the given directory. Sandbox hooks can be wired here.
func RunCommand(ctx context.Context, workdir string, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("empty command")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	if workdir != "" {
		cmd.Dir = workdir
	}
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to start pty: %w", err)
	}
	defer ptmx.Close()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, ptmx)
		close(done)
	}()

	err = cmd.Wait()
	ptmx.Close()
	<-done
	out := buf.String()
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
