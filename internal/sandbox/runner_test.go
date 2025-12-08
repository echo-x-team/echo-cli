package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithinRoots(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside", "dir")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if !withinRoots(inside, []string{root}) {
		t.Fatalf("expected %s inside %s", inside, root)
	}
	sibling := root + "-other"
	if withinRoots(sibling, []string{root}) {
		t.Fatalf("unexpected match for sibling %s", sibling)
	}
	outside := filepath.Join(root, "..", "outside")
	if withinRoots(outside, []string{inside}) {
		t.Fatalf("unexpected match for outside path %s", outside)
	}
}

func TestPatchPathsSafe(t *testing.T) {
	root := t.TempDir()
	okDiff := strings.Join([]string{
		"--- a/file.txt",
		"+++ a/file.txt",
		"@@",
		"+test",
	}, "\n")
	if !patchPathsSafe(root, okDiff, nil) {
		t.Fatalf("expected diff within root to be allowed")
	}

	badDiff := strings.Join([]string{
		"--- a/file.txt",
		"+++ /etc/passwd",
		"@@",
		"+x",
	}, "\n")
	if patchPathsSafe(root, badDiff, nil) {
		t.Fatalf("expected absolute path outside root to be rejected")
	}

	escapeDiff := strings.Join([]string{
		"--- a/file.txt",
		"+++ ../escape.txt",
		"@@",
		"+x",
	}, "\n")
	if patchPathsSafe(root, escapeDiff, nil) {
		t.Fatalf("expected relative escape to be rejected")
	}
}

func TestRunCommandRespectsRoots(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	runner := NewRunner("workspace-write", root)

	if _, _, err := runner.RunCommand(context.Background(), other, "echo ok"); err == nil {
		t.Fatalf("expected command outside root to fail")
	}
	if _, _, err := runner.RunCommand(context.Background(), "", "echo ok"); err == nil {
		t.Fatalf("expected command without workdir to fail when roots set")
	}
	out, _, err := runner.RunCommand(context.Background(), root, "echo ok")
	if err != nil {
		t.Fatalf("expected command inside root to pass: %v", err)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestApplyPatchRespectsRoots(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner("workspace-write", root)

	diff := strings.Join([]string{
		"--- a/file.txt",
		"+++ /etc/passwd",
		"@@",
		"+x",
	}, "\n")
	if err := runner.ApplyPatch(context.Background(), root, diff); err == nil {
		t.Fatalf("expected patch outside root to fail")
	}
	if err := runner.ApplyPatch(context.Background(), "", diff); err == nil {
		t.Fatalf("expected patch without workdir to fail when roots set")
	}
}
