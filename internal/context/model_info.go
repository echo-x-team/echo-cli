package context

import (
	"os"
	"strconv"
	"strings"
)

const (
	contextWindow272K int64 = 272_000
)

// ContextWindowForModel 尝试推导模型的上下文窗口（tokens）。
// 优先读取环境变量 `ECHO_MODEL_CONTEXT_WINDOW`（若存在）。
//
// 注意：Echo 的 Go 实现当前无法从 provider 获取真实窗口；这里对齐 codex-rs 的已知映射并提供可覆盖入口。
func ContextWindowForModel(model string) (int64, bool) {
	if v := strings.TrimSpace(os.Getenv("ECHO_MODEL_CONTEXT_WINDOW")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n, true
		}
	}

	slug := strings.TrimSpace(model)
	if slug == "" {
		return 0, false
	}

	switch slug {
	case "gpt-oss-20b", "gpt-oss-120b":
		return 96_000, true
	case "o3", "o4-mini", "codex-mini-latest":
		return 200_000, true
	case "gpt-4.1", "gpt-4.1-2025-04-14":
		return 1_047_576, true
	case "gpt-4o", "gpt-4o-2024-08-06", "gpt-4o-2024-05-13", "gpt-4o-2024-11-20":
		return 128_000, true
	case "gpt-3.5-turbo":
		return 16_385, true
	}

	switch {
	case strings.HasPrefix(slug, "gpt-5-codex"),
		strings.HasPrefix(slug, "gpt-5.1-codex"),
		strings.HasPrefix(slug, "gpt-5.1-codex-max"),
		strings.HasPrefix(slug, "gpt-5"),
		strings.HasPrefix(slug, "codex-"),
		strings.HasPrefix(slug, "exp-"):
		return contextWindow272K, true
	}

	return 0, false
}

func DefaultAutoCompactLimit(contextWindow int64) int64 {
	if contextWindow <= 0 {
		return 0
	}
	return (contextWindow * 9) / 10
}
