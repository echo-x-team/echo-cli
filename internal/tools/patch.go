package tools

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type patchOpKind int

const (
	patchOpAdd patchOpKind = iota + 1
	patchOpDelete
	patchOpUpdate
)

type patchOp struct {
	kind    patchOpKind
	path    string
	newPath string
	lines   []string
}

// ApplyPatch runs the system `patch` command with unified diff content.
func ApplyPatch(ctx context.Context, workdir string, diff string) error {
	if strings.TrimSpace(diff) == "" {
		return fmt.Errorf("empty patch content")
	}
	if strings.HasPrefix(strings.TrimSpace(diff), "*** Begin Patch") {
		ops, err := parseBeginPatch(diff)
		if err != nil {
			return err
		}
		return applyParsedPatch(ctx, workdir, ops)
	}
	return applySystemPatch(ctx, workdir, diff)
}

func applySystemPatch(ctx context.Context, workdir string, diff string) error {
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

func parseBeginPatch(text string) ([]patchOp, error) {
	lines := strings.Split(text, "\n")
	idx := 0
	for idx < len(lines) && strings.TrimSpace(lines[idx]) == "" {
		idx++
	}
	if idx >= len(lines) || strings.TrimSpace(lines[idx]) != "*** Begin Patch" {
		return nil, fmt.Errorf("invalid patch: missing *** Begin Patch")
	}
	idx++

	var ops []patchOp
	for idx < len(lines) {
		line := strings.TrimSpace(lines[idx])
		switch {
		case line == "*** End Patch":
			return ops, nil
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			idx++
			start := idx
			for idx < len(lines) {
				trim := strings.TrimSpace(lines[idx])
				if strings.HasPrefix(trim, "*** ") {
					break
				}
				idx++
			}
			ops = append(ops, patchOp{kind: patchOpAdd, path: path, lines: lines[start:idx]})
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			idx++
			ops = append(ops, patchOp{kind: patchOpDelete, path: path})
		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			idx++
			newPath := ""
			if idx < len(lines) {
				if trim := strings.TrimSpace(lines[idx]); strings.HasPrefix(trim, "*** Move to: ") {
					newPath = strings.TrimSpace(strings.TrimPrefix(trim, "*** Move to: "))
					idx++
				}
			}
			start := idx
			for idx < len(lines) {
				trim := strings.TrimSpace(lines[idx])
				if strings.HasPrefix(trim, "*** ") {
					if trim == "*** End of File" {
						idx++
						continue
					}
					break
				}
				idx++
			}
			ops = append(ops, patchOp{kind: patchOpUpdate, path: path, newPath: newPath, lines: lines[start:idx]})
		case line == "":
			idx++
		default:
			return nil, fmt.Errorf("invalid patch directive: %s (supported: *** Add File:, *** Delete File:, *** Update File:)", line)
		}
	}
	return nil, fmt.Errorf("invalid patch: missing *** End Patch")
}

func applyParsedPatch(ctx context.Context, workdir string, ops []patchOp) error {
	for _, op := range ops {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		switch op.kind {
		case patchOpAdd:
			if err := applyAdd(workdir, op); err != nil {
				return err
			}
		case patchOpDelete:
			if err := applyDelete(workdir, op); err != nil {
				return err
			}
		case patchOpUpdate:
			if err := applyUpdate(workdir, op); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported patch operation for %s", op.path)
		}
	}
	return nil
}

func applyAdd(workdir string, op patchOp) error {
	target := filepath.Join(workdir, op.path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	content := stripPrefixes(op.lines, '+')
	return os.WriteFile(target, []byte(content), 0o644)
}

func applyDelete(workdir string, op patchOp) error {
	target := filepath.Join(workdir, op.path)
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("delete target does not exist: %s", op.path)
		}
		return err
	}
	return nil
}

func applyUpdate(workdir string, op patchOp) error {
	oldPath := filepath.Join(workdir, op.path)
	data, err := os.ReadFile(oldPath)
	if err != nil {
		return err
	}
	fileMode := fs.FileMode(0o644)
	if info, statErr := os.Stat(oldPath); statErr == nil {
		fileMode = info.Mode()
	}

	origLines, hadTrailing := splitLines(string(data))
	updated, err := applyHunks(origLines, op.lines)
	if err != nil {
		return err
	}
	content := strings.Join(updated, "\n")
	if hadTrailing {
		content += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(oldPath, []byte(content), fileMode); err != nil {
		return err
	}
	if op.newPath != "" && op.newPath != op.path {
		newPath := filepath.Join(workdir, op.newPath)
		if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
			return err
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			return err
		}
	}
	return nil
}

func splitLines(text string) ([]string, bool) {
	hasTrailing := strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return []string{}, hasTrailing
	}
	return strings.Split(text, "\n"), hasTrailing
}

func applyHunks(orig []string, lines []string) ([]string, error) {
	hunks, err := collectHunks(lines)
	if err != nil {
		return nil, err
	}

	result := append([]string{}, orig...)
	cursor := 0
	for _, hunk := range hunks {
		oldLines, newLines, err := decodeHunk(hunk)
		if err != nil {
			return nil, err
		}
		idx := findSubslice(result, oldLines, cursor)
		if idx == -1 {
			return nil, fmt.Errorf("patch context not found for %v", oldLines)
		}
		next := make([]string, 0, len(result)-len(oldLines)+len(newLines))
		next = append(next, result[:idx]...)
		next = append(next, newLines...)
		next = append(next, result[idx+len(oldLines):]...)
		result = next
		cursor = idx + len(newLines)
	}
	return result, nil
}

func collectHunks(lines []string) ([][]string, error) {
	var hunks [][]string
	var current []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "*** ") && trim != "*** End of File" {
			return nil, fmt.Errorf("unexpected directive inside hunk: %s", trim)
		}
		if strings.HasPrefix(trim, "@@") {
			if len(current) > 0 {
				hunks = append(hunks, current)
			}
			current = []string{}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		hunks = append(hunks, current)
	}
	return hunks, nil
}

func decodeHunk(lines []string) ([]string, []string, error) {
	oldLines := make([]string, 0, len(lines))
	newLines := make([]string, 0, len(lines))

	for _, line := range lines {
		if len(line) == 0 {
			return nil, nil, fmt.Errorf("invalid empty hunk line")
		}
		switch line[0] {
		case ' ':
			content := line[1:]
			oldLines = append(oldLines, content)
			newLines = append(newLines, content)
		case '-':
			oldLines = append(oldLines, line[1:])
		case '+':
			newLines = append(newLines, line[1:])
		default:
			return nil, nil, fmt.Errorf("invalid hunk line: %s", line)
		}
	}
	return oldLines, newLines, nil
}

func findSubslice(lines []string, target []string, start int) int {
	if len(target) == 0 {
		return start
	}
	for i := start; i+len(target) <= len(lines); i++ {
		match := true
		for j := range target {
			if lines[i+j] != target[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func stripPrefixes(lines []string, prefix byte) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			parts = append(parts, "")
			continue
		}
		if line[0] == prefix {
			parts = append(parts, line[1:])
		} else {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, "\n")
}
