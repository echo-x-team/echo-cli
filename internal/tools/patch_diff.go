package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PatchSummary describes which workspace paths a patch intends to touch.
// Paths are workspace-relative (cleaned) where possible.
type PatchSummary struct {
	Paths   []string
	Primary string
}

// ExtractPatchPaths extracts file paths referenced by a patch.
// It supports both the custom "*** Begin Patch" format and unified diffs.
// Returned paths are trimmed but otherwise reflect the patch content (e.g. can be absolute).
func ExtractPatchPaths(patch string) ([]string, error) {
	patch = strings.TrimSpace(patch)
	if patch == "" {
		return nil, fmt.Errorf("empty patch")
	}

	if strings.Contains(patch, "*** Begin Patch") {
		ops, err := parseBeginPatch(patch)
		if err != nil {
			return nil, err
		}
		var paths []string
		for _, op := range ops {
			if p := strings.TrimSpace(op.path); p != "" {
				paths = append(paths, p)
			}
			if p := strings.TrimSpace(op.newPath); p != "" {
				paths = append(paths, p)
			}
		}
		return paths, nil
	}

	// Unified diff: scan --- / +++ headers.
	var paths []string
	var pendingOld string
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "--- ") {
			pendingOld = parseUnifiedHeaderPath(strings.TrimPrefix(line, "--- "))
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			newPath := parseUnifiedHeaderPath(strings.TrimPrefix(line, "+++ "))
			oldPath := pendingOld
			pendingOld = ""

			pick := newPath
			if pick == "" || pick == "/dev/null" {
				pick = oldPath
			}
			if pick == "" || pick == "/dev/null" {
				continue
			}
			// Common "a/.." + "b/.." pairing: prefer the stripped workspace path.
			if strings.HasPrefix(oldPath, "a/") && strings.HasPrefix(newPath, "b/") {
				os := strings.TrimPrefix(oldPath, "a/")
				ns := strings.TrimPrefix(newPath, "b/")
				if os == ns && os != "" {
					pick = os
				}
			}
			paths = append(paths, pick)
		}
	}
	return paths, nil
}

func parseUnifiedHeaderPath(rest string) string {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return ""
	}
	// diff -u may include timestamps separated by tabs/spaces.
	if i := strings.IndexByte(rest, '\t'); i >= 0 {
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		// Keep git-style paths (no timestamps). For timestamped headers, this is safe enough.
		rest = rest[:i]
	}
	return strings.TrimSpace(rest)
}

func resolvePathInWorkdir(workdir string, raw string) (rel string, abs string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/dev/null" {
		return "", "", false
	}
	if workdir == "" {
		workdir = "."
	}
	wdAbs, err := filepath.Abs(workdir)
	if err != nil {
		return "", "", false
	}
	wdAbs = filepath.Clean(wdAbs)

	if filepath.IsAbs(raw) {
		abs = filepath.Clean(raw)
	} else {
		abs = filepath.Clean(filepath.Join(wdAbs, raw))
	}

	// Ensure abs is within workdir; avoid accidentally diffing unrelated filesystem paths.
	sep := string(filepath.Separator)
	if abs != wdAbs && !strings.HasPrefix(abs+sep, wdAbs+sep) {
		return "", "", false
	}
	rel, err = filepath.Rel(wdAbs, abs)
	if err != nil {
		return "", "", false
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == "" {
		return "", "", false
	}
	if strings.HasPrefix(rel, ".."+sep) || rel == ".." {
		return "", "", false
	}
	return rel, abs, true
}

func dedupePreserveOrder(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, p := range in {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// SummarizePatch returns a workspace-scoped summary. It resolves patch paths to
// workspace-relative paths when possible.
func SummarizePatch(workdir string, patch string) (PatchSummary, error) {
	raw, err := ExtractPatchPaths(patch)
	if err != nil {
		return PatchSummary{}, err
	}
	relPaths := make([]string, 0, len(raw))
	for _, p := range raw {
		rel, _, ok := resolvePathInWorkdir(workdir, p)
		if !ok {
			continue
		}
		relPaths = append(relPaths, rel)
	}
	relPaths = dedupePreserveOrder(relPaths)
	s := PatchSummary{Paths: relPaths}
	if len(relPaths) > 0 {
		s.Primary = relPaths[0]
	}
	return s, nil
}

// PreviewPatchDiff applies patch to a temp copy of the affected files and returns
// a unified diff against the current workspace version.
//
// This is used for approval previews (before the real patch is applied).
func PreviewPatchDiff(ctx context.Context, workdir string, patch string) (string, PatchSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if workdir == "" {
		workdir = "."
	}

	summary, err := SummarizePatch(workdir, patch)
	if err != nil {
		return "", PatchSummary{}, err
	}
	if len(summary.Paths) == 0 {
		return "", summary, fmt.Errorf("no patch paths found")
	}

	tmp, err := os.MkdirTemp("", "echo-cli-patch-preview-*")
	if err != nil {
		return "", summary, err
	}
	defer os.RemoveAll(tmp)

	applyRoot := filepath.Join(tmp, "work")
	if err := os.MkdirAll(applyRoot, 0o755); err != nil {
		return "", summary, err
	}

	// Seed the temp workspace with current files so ApplyPatch can run without touching real files.
	before := make(map[string][]byte, len(summary.Paths))
	for _, rel := range summary.Paths {
		abs := filepath.Join(workdir, rel)
		data, readErr := os.ReadFile(abs)
		if readErr == nil {
			before[rel] = data
			if err := writeFileAll(filepath.Join(applyRoot, rel), data, abs); err != nil {
				return "", summary, err
			}
		} else if os.IsNotExist(readErr) {
			before[rel] = nil
		} else {
			return "", summary, readErr
		}
	}

	if err := ApplyPatch(ctx, applyRoot, patch); err != nil {
		return "", summary, err
	}

	after := make(map[string][]byte, len(summary.Paths))
	for _, rel := range summary.Paths {
		data, readErr := os.ReadFile(filepath.Join(applyRoot, rel))
		if readErr == nil {
			after[rel] = data
			continue
		}
		if os.IsNotExist(readErr) {
			after[rel] = nil
			continue
		}
		return "", summary, readErr
	}

	diff, err := UnifiedDiffForFiles(ctx, summary.Paths, before, after)
	return diff, summary, err
}

func writeFileAll(dst string, data []byte, srcForMode string) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(srcForMode); err == nil {
		mode = info.Mode()
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode)
}

// UnifiedDiffForFiles produces a unified diff for the given set of workspace-relative paths.
// before/after are keyed by the same relative path; nil represents "missing file".
func UnifiedDiffForFiles(ctx context.Context, paths []string, before map[string][]byte, after map[string][]byte) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	tmp, err := os.MkdirTemp("", "echo-cli-diff-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	rootA := filepath.Join(tmp, "a")
	rootB := filepath.Join(tmp, "b")
	if err := os.MkdirAll(rootA, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(rootB, 0o755); err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, rel := range paths {
		old := before[rel]
		newb := after[rel]
		if bytes.Equal(old, newb) {
			continue
		}
		aPath := filepath.Join(rootA, rel)
		bPath := filepath.Join(rootB, rel)
		if err := os.MkdirAll(filepath.Dir(aPath), 0o755); err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(bPath), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(aPath, old, 0o644); err != nil {
			return "", err
		}
		if err := os.WriteFile(bPath, newb, 0o644); err != nil {
			return "", err
		}

		chunk, err := unifiedDiffOne(ctx, tmp, filepath.Join("a", rel), filepath.Join("b", rel))
		if err != nil {
			return "", err
		}
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(chunk)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

func unifiedDiffOne(ctx context.Context, cwd string, aRel string, bRel string) (string, error) {
	cmd := exec.CommandContext(ctx, "diff", "-u", aRel, bRel)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()

	// diff exit codes: 0 = identical, 1 = different, >1 = error.
	// Treat exit code 1 as success (differences found).
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code := ee.ExitCode()
			if code != 1 {
				return "", fmt.Errorf("diff failed: %w: %s", err, string(out))
			}
		} else {
			return "", fmt.Errorf("diff failed: %w: %s", err, string(out))
		}
	}
	text := string(out)
	if strings.TrimSpace(text) == "" {
		return "", nil
	}

	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) >= 2 && strings.HasPrefix(lines[0], "--- ") && strings.HasPrefix(lines[1], "+++ ") {
		lines[0] = "--- " + aRel
		lines[1] = "+++ " + bRel
	}
	return strings.Join(lines, "\n"), nil
}
