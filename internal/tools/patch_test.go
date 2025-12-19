package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatchBeginPatchUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	patch := `*** Begin Patch
*** Update File: file.txt
@@
-hello
+hi
*** End Patch`

	if err := ApplyPatch(context.Background(), dir, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	if got := string(data); got != "hi\nworld\n" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestApplyPatchBeginPatchUpdateReplaceWholeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("old\ncontent\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	patch := `*** Begin Patch
*** Update File: file.txt
alpha
beta
*** End Patch`

	if err := ApplyPatch(context.Background(), dir, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	if got := string(data); got != "alpha\nbeta\n" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestApplyPatchBeginPatchUpdateReplaceWholeFileWithHunkHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("old\ncontent\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	patch := `*** Begin Patch
*** Update File: file.txt
@@
alpha
beta
*** End Patch`

	if err := ApplyPatch(context.Background(), dir, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}
	if got := string(data); got != "alpha\nbeta\n" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestApplyPatchBeginPatchAddAndDelete(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(oldPath, []byte("keep me\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	patch := `*** Begin Patch
*** Add File: new.txt
+line1
+line2
*** Delete File: old.txt
*** End Patch`

	if err := ApplyPatch(context.Background(), dir, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old file removed, got err=%v", err)
	}
	newPath := filepath.Join(dir, "new.txt")
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if got := string(data); got != "line1\nline2" {
		t.Fatalf("unexpected new file content: %q", got)
	}
}

func TestApplyPatchBeginPatchMove(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.txt")
	if err := os.WriteFile(oldPath, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	patch := `*** Begin Patch
*** Update File: old.txt
*** Move to: new.txt
@@
-two
+TWO
*** End Patch`

	if err := ApplyPatch(context.Background(), dir, patch); err != nil {
		t.Fatalf("apply patch: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old file removed after move, got err=%v", err)
	}
	newPath := filepath.Join(dir, "new.txt")
	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read moved file: %v", err)
	}
	if got := string(data); got != "one\nTWO\n" {
		t.Fatalf("unexpected moved content: %q", got)
	}
}

func TestApplyPatchBeginPatch_InvalidDirective(t *testing.T) {
	dir := t.TempDir()

	patch := `*** Begin Patch
*** Create File: bad.txt
+nope
*** End Patch`

	err := ApplyPatch(context.Background(), dir, patch)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "invalid patch directive") || !strings.Contains(err.Error(), "*** Add File") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyPatchBeginPatch_MissingEndPatch(t *testing.T) {
	dir := t.TempDir()

	patch := `*** Begin Patch
*** Add File: x.txt
+hello`

	err := ApplyPatch(context.Background(), dir, patch)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "missing *** End Patch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
