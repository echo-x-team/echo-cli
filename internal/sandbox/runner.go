package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"echo-cli/internal/policy"
	"echo-cli/internal/tools"
)

// Runner abstracts running commands/patches under a sandbox mode.
// For now, this is a thin wrapper around tools with a placeholder for future isolation.
type Runner = tools.Runner

type SafeRunner struct {
	mode  policy.Policy
	roots []string
}

func NewRunner(sandboxMode string, roots ...string) Runner {
	return SafeRunner{mode: policy.Policy{SandboxMode: sandboxMode}, roots: cleanRoots(roots)}
}

func (r SafeRunner) allowedRoots(workdir string) []string {
	if len(r.roots) > 0 {
		return r.roots
	}
	if workdir != "" {
		return cleanRoots([]string{workdir})
	}
	return nil
}

func (r SafeRunner) RunCommand(ctx context.Context, workdir string, command string) (string, int, error) {
	if r.mode.SandboxMode == "read-only" {
		return "", -1, tools.SandboxError{Reason: "sandbox read-only: command blocked"}
	}
	roots := r.allowedRoots(workdir)
	if len(roots) > 0 {
		if workdir == "" {
			return "", -1, tools.SandboxError{Reason: "workdir required for sandboxed command"}
		}
		if !withinRoots(workdir, roots) {
			return "", -1, tools.SandboxError{Reason: "workdir outside allowed roots"}
		}
	}
	return r.runWithSandbox(ctx, workdir, command, roots)
}

func (r SafeRunner) ApplyPatch(ctx context.Context, workdir string, diff string) error {
	if r.mode.SandboxMode == "read-only" {
		return tools.SandboxError{Reason: "sandbox read-only: patch blocked"}
	}
	roots := r.allowedRoots(workdir)
	if len(roots) > 0 && workdir == "" {
		return tools.SandboxError{Reason: "workdir required for sandboxed patch"}
	}
	if ok, reason := patchPathsSafe(workdir, diff, roots); !ok {
		return tools.SandboxError{Reason: reason}
	}
	return tools.ApplyPatch(ctx, workdir, diff)
}

func patchPathsSafe(root string, diff string, roots []string) (bool, string) {
	const defaultReason = "patch references paths outside workspace"
	if root == "" {
		root = "."
	}
	roots = cleanRoots(append(roots, root))
	if len(roots) > 0 {
		root = roots[len(roots)-1]
	}
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
			path := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "--- "))
			if path == "" || path == "/dev/null" {
				continue
			}
			if strings.HasPrefix(path, "/") {
				if !withinRoots(path, roots) {
					return false, defaultReason
				}
				continue
			}
			clean := filepath.Clean(filepath.Join(root, path))
			if !withinRoots(clean, roots) {
				return false, defaultReason
			}
		}
	}
	return true, ""
}

func (r SafeRunner) runWithSandbox(ctx context.Context, workdir string, command string, roots []string) (string, int, error) {
	if r.mode.SandboxMode == "danger-full-access" {
		out, err := tools.RunCommand(ctx, workdir, command)
		if err != nil {
			return out, exitCode(err), err
		}
		return out, 0, nil
	}

	if runtime.GOOS == "darwin" {
		if path, err := exec.LookPath("sandbox-exec"); err == nil {
			profile := seatbeltProfile(r.mode.SandboxMode, cleanRoots(roots))
			wrapped := exec.CommandContext(ctx, path, "-p", profile, "bash", "-lc", command)
			if workdir != "" {
				wrapped.Dir = workdir
			}
			return tools.RunWrapped(wrapped)
		}
	}

	if runtime.GOOS == "linux" {
		if path, err := exec.LookPath("landlock-run"); err == nil {
			args := []string{path, "bash", "-lc", command}
			wrapped := exec.CommandContext(ctx, args[0], args[1:]...)
			if workdir != "" {
				wrapped.Dir = workdir
			}
			return tools.RunWrapped(wrapped)
		}
	}

	out, err := tools.RunCommand(ctx, workdir, command)
	if err != nil {
		return out, exitCode(err), err
	}
	return out, 0, nil
}

func (r SafeRunner) WithMode(mode string) tools.Runner {
	return SafeRunner{mode: policy.Policy{SandboxMode: mode}, roots: r.roots}
}

func seatbeltProfile(mode string, roots []string) string {
	if mode == "danger-full-access" {
		return `(version 1)
(allow default)`
	}
	perms := "file-read*"
	if mode != "read-only" {
		perms += " file-write*"
	}
	roots = cleanRoots(roots)
	var builder strings.Builder
	builder.WriteString("(version 1)\n")
	builder.WriteString("(deny default)\n")
	builder.WriteString("(allow process*)\n")
	builder.WriteString("(deny network*)\n")
	builder.WriteString("(allow " + perms)
	if len(roots) == 0 {
		builder.WriteString(")\n")
		return builder.String()
	}
	for _, root := range roots {
		builder.WriteString(` (subpath "` + root + `")`)
	}
	builder.WriteString(")\n")
	return builder.String()
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func cleanRoots(roots []string) []string {
	seen := make(map[string]struct{})
	cleaned := make([]string, 0, len(roots))
	for _, r := range roots {
		if strings.TrimSpace(r) == "" {
			continue
		}
		abs, err := filepath.Abs(r)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		cleaned = append(cleaned, abs)
	}
	return cleaned
}

func withinRoots(path string, roots []string) bool {
	if len(roots) == 0 {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, r := range roots {
		if strings.TrimSpace(r) == "" {
			continue
		}
		rootAbs, err := filepath.Abs(r)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			continue
		}
		if rel == "." || !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}
