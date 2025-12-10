package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"echo-cli/internal/agent"
	"echo-cli/internal/instructions"
	"echo-cli/internal/prompts"

	tea "github.com/charmbracelet/bubbletea"
)

// handleInitCommand 触发 /init：若不存在 AGENTS.md，则提交生成提示词，避免并发与覆盖。
func (m *Model) handleInitCommand() tea.Cmd {
	if m.pending {
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: "Cannot run /init while another request is in progress."})
		m.refreshTranscript()
		return nil
	}

	workdir := m.workdir
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	if workdir == "" {
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: "Working directory is not set; cannot run /init."})
		m.refreshTranscript()
		return nil
	}

	target := filepath.Join(workdir, instructions.ProjectDocFilename)
	if info, err := os.Stat(target); err == nil {
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		}
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: fmt.Sprintf("Skipping /init: %s already exists (%s).", target, kind)})
		m.refreshTranscript()
		return nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: fmt.Sprintf("Cannot run /init: %v", err)})
		m.refreshTranscript()
		return nil
	}

	prompt, err := buildInitPrompt(workdir)
	if err != nil {
		m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: fmt.Sprintf("Init prompt unavailable: %v", err)})
		m.refreshTranscript()
		return nil
	}

	m.messages = append(m.messages, agent.Message{Role: agent.RoleUser, Content: prompt})
	m.messages = append(m.messages, agent.Message{Role: agent.RoleAssistant, Content: ""})
	m.streamIdx = len(m.messages) - 1
	m.refreshTranscript()
	m.pending = true
	m.setComposerHeight()
	ctx := m.defaultInputContext()
	if ctx.Metadata == nil {
		ctx.Metadata = map[string]string{}
	}
	ctx.Metadata["target"] = "@internal/execution"
	return m.startSubmission(prompt, ctx)
}

func buildInitPrompt(workdir string) (string, error) {
	base, ok := prompts.Builtin(prompts.PromptInitCommand)
	if !ok {
		return "", errors.New("init-command prompt missing")
	}
	summary := summarizeRepository(workdir)
	if summary == "" {
		return base, nil
	}
	return fmt.Sprintf("%s\n\nRepository scan:\n%s", base, summary), nil
}

func summarizeRepository(workdir string) string {
	if workdir == "" {
		return ""
	}
	var parts []string

	if module := readModuleName(workdir); module != "" {
		parts = append(parts, fmt.Sprintf("- Go module: %s", module))
	}
	if dirs := topLevelDirectories(workdir); len(dirs) > 0 {
		parts = append(parts, fmt.Sprintf("- Top-level directories: %s", strings.Join(dirs, ", ")))
	}
	if files := keyProjectFiles(workdir); len(files) > 0 {
		parts = append(parts, fmt.Sprintf("- Notable files: %s", strings.Join(files, ", ")))
	}

	return strings.Join(parts, "\n")
}

func readModuleName(workdir string) string {
	data, err := os.ReadFile(filepath.Join(workdir, "go.mod"))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return ""
}

func topLevelDirectories(workdir string) []string {
	entries, err := os.ReadDir(workdir)
	if err != nil {
		return nil
	}
	ignore := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
	}
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if ignore[name] || strings.HasPrefix(name, ".") {
			continue
		}
		dirs = append(dirs, name)
	}
	sort.Strings(dirs)
	const maxDirs = 12
	if len(dirs) > maxDirs {
		remaining := len(dirs) - maxDirs
		dirs = append(dirs[:maxDirs], fmt.Sprintf("+%d more", remaining))
	}
	return dirs
}

func keyProjectFiles(workdir string) []string {
	candidates := []string{"README.md", "Makefile", "go.mod", "go.work", "Dockerfile", "TODO.md"}
	found := make([]string, 0, len(candidates))
	for _, name := range candidates {
		if _, err := os.Stat(filepath.Join(workdir, name)); err == nil {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	return found
}
