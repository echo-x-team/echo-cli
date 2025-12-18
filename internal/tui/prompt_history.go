package tui

import "strings"

// promptHistory 负责输入框历史浏览状态（上下箭头）。
// cursor == len(entries) 表示当前在“最新输入”（非浏览历史）位置。
type promptHistory struct {
	entries []string
	cursor  int
	draft   string
}

func (h *promptHistory) Set(entries []string) {
	h.entries = append([]string(nil), entries...)
	h.cursor = len(h.entries)
	h.draft = ""
}

func (h *promptHistory) Add(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	h.entries = append(h.entries, text)
	h.cursor = len(h.entries)
	h.draft = ""
}

func (h *promptHistory) Browsing() bool {
	return h.cursor < len(h.entries)
}

func (h *promptHistory) ResetBrowsing() {
	h.cursor = len(h.entries)
	h.draft = ""
}

func (h *promptHistory) Prev(current string) (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.cursor == len(h.entries) {
		h.draft = current
	}
	if h.cursor > 0 {
		h.cursor--
	}
	return h.entries[h.cursor], true
}

func (h *promptHistory) Next() (string, bool) {
	if len(h.entries) == 0 {
		return "", false
	}
	if h.cursor == len(h.entries) {
		return "", false
	}
	if h.cursor < len(h.entries)-1 {
		h.cursor++
		return h.entries[h.cursor], true
	}
	h.cursor = len(h.entries)
	return h.draft, true
}
