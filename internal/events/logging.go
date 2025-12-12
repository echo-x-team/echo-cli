package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"echo-cli/internal/logger"
)

// 默认的 SQ/EQ 日志文件路径。
const (
	DefaultSQLogPath = "logs/sq.log"
	DefaultEQLogPath = "logs/eq.log"
)

// log 复用全局 logger，标记事件组件。
var log = logger.Named("events")

func newQueueLogger(component, path string) (*logger.LogEntry, io.Closer) {
	if path == "" {
		return logger.Named(component), nil
	}
	entry, closer, _, err := logger.SetupComponentFile(component, path)
	if err != nil {
		log.Warnf("failed to set up %s log file (%s): %v", component, path, err)
		return logger.Named(component), nil
	}
	return entry, closer
}

func encodePayload(payload any) string {
	if payload == nil {
		return ""
	}
	// 日志里常见的 payload 为字符串（如 operation kind / error）。优先按可读性处理：
	// - 若字符串本身是 JSON：输出缩进 JSON
	// - 若字符串包含 `\n` 等转义：转为实际换行，避免命令行里看到大量 `\n`
	if s, ok := payload.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}

		// 1) 直接尝试将字符串解析为 JSON（适配 payload 以字符串承载 JSON 的情况）。
		if pretty, ok := prettyJSONString(s); ok {
			return pretty
		}

		// 2) 尝试把它当作 JSON 字符串再解一次引用（适配二次编码）。
		var unquoted string
		if err := json.Unmarshal([]byte(s), &unquoted); err == nil {
			unquoted = strings.TrimSpace(unquoted)
			if unquoted != "" {
				if pretty, ok := prettyJSONString(unquoted); ok {
					return pretty
				}
				return unquoted
			}
		}

		// 3) 将常见转义序列转换为实际字符（最常见的是 `\n`）。
		if unescaped, ok := unescapeCommonSequences(s); ok {
			if pretty, ok := prettyJSONString(strings.TrimSpace(unescaped)); ok {
				return pretty
			}
			return unescaped
		}

		return s
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}

	// 对象/数组 payload 用缩进 JSON 提升可读性。
	if pretty, ok := prettyJSONBytes(b); ok {
		return pretty
	}
	return string(b)
}

func prettyJSONString(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if s[0] != '{' && s[0] != '[' {
		return "", false
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return "", false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	out, ok := prettyJSONBytes(b)
	return out, ok
}

func prettyJSONBytes(b []byte) (string, bool) {
	if len(b) == 0 || (b[0] != '{' && b[0] != '[') {
		return "", false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return "", false
	}
	return buf.String(), true
}

func unescapeCommonSequences(s string) (string, bool) {
	if !strings.Contains(s, `\n`) && !strings.Contains(s, `\t`) && !strings.Contains(s, `\r`) {
		return "", false
	}

	// 尽量用 Unquote 处理完整的转义语义；若失败再回退到最常见的替换。
	escaped := strings.ReplaceAll(s, `"`, `\"`)
	if out, err := strconv.Unquote(`"` + escaped + `"`); err == nil {
		return out, true
	}

	out := strings.ReplaceAll(s, `\n`, "\n")
	out = strings.ReplaceAll(out, `\t`, "\t")
	out = strings.ReplaceAll(out, `\r`, "\r")
	return out, true
}
