package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"echo-cli/internal/tools"
)

type localRunner struct{}

func (localRunner) RunCommand(context.Context, string, string) (string, int, error) {
	return "", 0, nil
}
func (localRunner) ApplyPatch(ctx context.Context, workdir string, diff string) error {
	return tools.ApplyPatch(ctx, workdir, diff)
}
func (localRunner) WithMode(string) tools.Runner { return localRunner{} }

func TestApplyPatchHandler_ReturnsIncrementalUnifiedDiff(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	h := ApplyPatchHandler{}

	run := func(patch string) tools.ToolResult {
		payload, _ := json.Marshal(map[string]any{"patch": patch})
		inv := tools.Invocation{
			Call:    tools.ToolCall{ID: "1", Name: "apply_patch", Payload: payload},
			Workdir: tmp,
			Runner:  localRunner{},
		}
		res, err := h.Handle(context.Background(), inv)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if res.Status != "completed" {
			t.Fatalf("status=%s err=%s", res.Status, res.Error)
		}
		return res
	}

	patch1 := `*** Begin Patch
*** Delete File: test.txt
*** Add File: test.txt
+line1
+line2a
+line3
*** End Patch`
	res1 := run(patch1)
	if strings.Contains(res1.Diff, "*** Begin Patch") {
		t.Fatalf("expected computed diff, got raw patch:\n%s", res1.Diff)
	}
	if strings.Contains(res1.Diff, "+line1") {
		t.Fatalf("expected minimal diff without unchanged additions, got:\n%s", res1.Diff)
	}
	if !strings.Contains(res1.Diff, "--- a/test.txt") || !strings.Contains(res1.Diff, "+++ b/test.txt") {
		t.Fatalf("expected unified diff headers, got:\n%s", res1.Diff)
	}
	if !strings.Contains(res1.Diff, "-line2") || !strings.Contains(res1.Diff, "+line2a") {
		t.Fatalf("expected line2 change, got:\n%s", res1.Diff)
	}

	patch2 := `*** Begin Patch
*** Delete File: test.txt
*** Add File: test.txt
+line1
+line2b
+line3
*** End Patch`
	res2 := run(patch2)
	if strings.Contains(res2.Diff, "+line1") {
		t.Fatalf("expected minimal diff without unchanged additions, got:\n%s", res2.Diff)
	}
	if strings.Contains(res2.Diff, "-line2\n") {
		t.Fatalf("expected incremental diff against current version, got:\n%s", res2.Diff)
	}
	if !strings.Contains(res2.Diff, "-line2a") || !strings.Contains(res2.Diff, "+line2b") {
		t.Fatalf("expected line2a->line2b change, got:\n%s", res2.Diff)
	}
}
