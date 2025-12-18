package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
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
	cmd.Env = withNonInteractiveEnv(os.Environ(), command)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to start pty: %w", err)
	}
	defer ptmx.Close()

	var buf bytes.Buffer
	responder := newPromptResponder(command)
	window := &byteWindow{max: 16 * 1024}
	done := make(chan struct{})
	go func() {
		tmp := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(tmp)
			if n > 0 {
				chunk := tmp[:n]
				_, _ = buf.Write(chunk)
				window.Append(chunk)
				responder.MaybeRespond(ptmx, window.String())
			}
			if readErr != nil {
				if readErr == io.EOF {
					break
				}
				break
			}
		}
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

type byteWindow struct {
	buf []byte
	max int
}

func (w *byteWindow) Append(p []byte) {
	if w.max <= 0 {
		w.buf = append(w.buf, p...)
		return
	}
	if len(p) > w.max {
		p = p[len(p)-w.max:]
	}
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.max {
		w.buf = w.buf[len(w.buf)-w.max:]
	}
}

func (w *byteWindow) String() string {
	return string(w.buf)
}

type promptResponder struct {
	seen map[string]int
}

func newPromptResponder(command string) *promptResponder {
	_ = command
	return &promptResponder{seen: map[string]int{}}
}

func (r *promptResponder) MaybeRespond(w io.Writer, window string) {
	if r == nil {
		return
	}
	// npm 的典型确认提示：Need to install ... Ok to proceed?
	// 为避免误伤，只在同时出现两段固定文案时自动输入 y。
	if r.seen["npm_install_ok"] == 0 &&
		strings.Contains(window, "Need to install the following packages:") &&
		strings.Contains(window, "Ok to proceed?") {
		r.seen["npm_install_ok"]++
		_, _ = io.WriteString(w, "y\n")
		ensureToolsLogger()
		toolsLog.Infof("command_auto_response rule=%s response=%q", "npm_install_ok", "y")
	}
}

func withNonInteractiveEnv(env []string, command string) []string {
	// 为 npm/npx 的安装确认提供默认 yes，避免阻塞工具链路。
	// 仅在检测到相关命令时注入，降低对其他命令的影响面。
	if !looksLikeNpmCommand(command) {
		return env
	}
	out := append([]string{}, env...)
	out = appendEnvIfMissing(out, "npm_config_yes", "true")
	out = appendEnvIfMissing(out, "npm_config_fund", "false")
	out = appendEnvIfMissing(out, "npm_config_audit", "false")
	out = appendEnvIfMissing(out, "npm_config_update_notifier", "false")
	out = appendEnvIfMissing(out, "CI", "1")
	return out
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

func looksLikeNpmCommand(command string) bool {
	s := " " + command + " "
	return strings.Contains(s, " npm ") || strings.Contains(s, " npx ")
}
