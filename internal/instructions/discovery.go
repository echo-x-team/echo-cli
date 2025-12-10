package instructions

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// ProjectDocFilename 是默认的仓库说明文件名称，供代理加载与 /init 生成。
	ProjectDocFilename = "AGENTS.md"
	// ProjectOverrideFilename 用于提供覆盖父级的说明文件。
	ProjectOverrideFilename = "AGENTS.override.md"
)

// Discover reads AGENTS.md chain: ~/.echo/AGENTS.md and directory tree overrides.
func Discover(workdir string) string {
	var parts []string

	home, _ := os.UserHomeDir()
	if home != "" {
		global := filepath.Join(home, ".echo", ProjectDocFilename)
		if data, err := os.ReadFile(global); err == nil {
			parts = append(parts, string(data))
		}
	}

	dir := workdir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	dir = filepath.Clean(dir)

	chain := []string{}
	prev := ""
	for dir != prev && dir != string(filepath.Separator) {
		chain = append(chain, dir)
		prev = dir
		dir = filepath.Dir(dir)
	}
	// top-down precedence
	for i := len(chain) - 1; i >= 0; i-- {
		curr := chain[i]
		override := filepath.Join(curr, ProjectOverrideFilename)
		if data, err := os.ReadFile(override); err == nil {
			parts = append(parts, string(data))
			continue
		}
		path := filepath.Join(curr, ProjectDocFilename)
		if data, err := os.ReadFile(path); err == nil {
			parts = append(parts, string(data))
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
