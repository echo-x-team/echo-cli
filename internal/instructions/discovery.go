package instructions

import (
	"os"
	"path/filepath"
	"strings"
)

// Discover reads AGENTS.md chain: ~/.echo/AGENTS.md and directory tree overrides.
func Discover(workdir string) string {
	var parts []string

	home, _ := os.UserHomeDir()
	if home != "" {
		global := filepath.Join(home, ".echo", "AGENTS.md")
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
		override := filepath.Join(curr, "AGENTS.override.md")
		if data, err := os.ReadFile(override); err == nil {
			parts = append(parts, string(data))
			continue
		}
		path := filepath.Join(curr, "AGENTS.md")
		if data, err := os.ReadFile(path); err == nil {
			parts = append(parts, string(data))
		}
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
