package context

import (
	"math"
	"strings"
	"unicode/utf8"
)

const approxBytesPerToken = 4

type TruncationPolicyKind int

const (
	TruncationBytes TruncationPolicyKind = iota + 1
	TruncationTokens
)

// TruncationPolicy 对齐 codex-rs：支持按 Bytes 或 Tokens 截断。
// Tokens 为近似 token（bytes/4）预算，并最终转换为 byte budget 执行 UTF-8 安全截断。
type TruncationPolicy struct {
	Kind   TruncationPolicyKind
	Budget int
}

func BytesPolicy(bytes int) TruncationPolicy {
	return TruncationPolicy{Kind: TruncationBytes, Budget: max0(bytes)}
}
func TokensPolicy(tokens int) TruncationPolicy {
	return TruncationPolicy{Kind: TruncationTokens, Budget: max0(tokens)}
}

func (p TruncationPolicy) Mul(multiplier float64) TruncationPolicy {
	if multiplier <= 0 {
		return TruncationPolicy{Kind: p.Kind, Budget: 0}
	}
	return TruncationPolicy{
		Kind:   p.Kind,
		Budget: int(math.Ceil(float64(p.Budget) * multiplier)),
	}
}

func (p TruncationPolicy) tokenBudget() int {
	switch p.Kind {
	case TruncationTokens:
		return p.Budget
	case TruncationBytes:
		return int(approxTokensFromByteCount(p.Budget))
	default:
		return 0
	}
}

func (p TruncationPolicy) byteBudget() int {
	switch p.Kind {
	case TruncationBytes:
		return p.Budget
	case TruncationTokens:
		return approxBytesForTokens(p.Budget)
	default:
		return 0
	}
}

// ApproxTokenCount 对齐 codex-rs：ceil(len_bytes/4) 的粗略估计。
func ApproxTokenCount(text string) int {
	if text == "" {
		return 0
	}
	n := len(text)
	return (n + approxBytesPerToken - 1) / approxBytesPerToken
}

func approxBytesForTokens(tokens int) int {
	if tokens <= 0 {
		return 0
	}
	return tokens * approxBytesPerToken
}

func approxTokensFromByteCount(bytes int) uint64 {
	if bytes <= 0 {
		return 0
	}
	return uint64(bytes+approxBytesPerToken-1) / uint64(approxBytesPerToken)
}

// FormattedTruncateText 对齐 codex-rs：当发生截断时在前面加上总行数信息。
func FormattedTruncateText(content string, policy TruncationPolicy) string {
	if len(content) <= policy.byteBudget() {
		return content
	}
	totalLines := countLinesLikeRust(content)
	return "Total output lines: " + itoa(totalLines) + "\n\n" + TruncateText(content, policy)
}

// countLinesLikeRust 对齐 Rust 的 str::lines().count()：
// - 空字符串返回 0
// - 末尾换行不计入额外空行
func countLinesLikeRust(s string) int {
	if s == "" {
		return 0
	}
	lines := strings.Count(s, "\n") + 1
	if strings.HasSuffix(s, "\n") {
		lines--
	}
	if lines < 0 {
		return 0
	}
	return lines
}

func TruncateText(content string, policy TruncationPolicy) string {
	switch policy.Kind {
	case TruncationBytes:
		return truncateWithByteEstimate(content, policy)
	case TruncationTokens:
		out, _ := truncateWithTokenBudget(content, policy)
		return out
	default:
		return content
	}
}

func truncateWithTokenBudget(s string, policy TruncationPolicy) (string, *uint64) {
	if s == "" {
		return "", nil
	}
	maxTokens := policy.tokenBudget()
	if maxTokens > 0 && len(s) <= approxBytesForTokens(maxTokens) {
		return s, nil
	}

	truncated := truncateWithByteEstimate(s, policy)
	approxTotal := uint64(ApproxTokenCount(s))
	if truncated == s {
		return truncated, nil
	}
	return truncated, &approxTotal
}

func truncateWithByteEstimate(s string, policy TruncationPolicy) string {
	return truncateWithByteBudget(s, policy.byteBudget(), policy.Kind)
}

func splitBudget(budget int) (left int, right int) {
	if budget <= 0 {
		return 0, 0
	}
	left = budget / 2
	right = budget - left
	return left, right
}

func splitStringUTF8(s string, prefixBytes int, suffixBytes int) (removedRunes int, prefix string, suffix string) {
	if s == "" {
		return 0, "", ""
	}
	if prefixBytes < 0 {
		prefixBytes = 0
	}
	if suffixBytes < 0 {
		suffixBytes = 0
	}

	totalBytes := len(s)
	tailStartTarget := totalBytes - suffixBytes
	if tailStartTarget < 0 {
		tailStartTarget = 0
	}

	prefixEnd := 0
	suffixStart := totalBytes
	suffixStarted := false

	for idx := range s {
		r, size := utf8.DecodeRuneInString(s[idx:])
		_ = r
		charEnd := idx + size

		if charEnd <= prefixBytes {
			prefixEnd = charEnd
			continue
		}

		if idx >= tailStartTarget {
			if !suffixStarted {
				suffixStart = idx
				suffixStarted = true
			}
			continue
		}

		removedRunes++
	}

	if suffixStart < prefixEnd {
		suffixStart = prefixEnd
	}

	return removedRunes, s[:prefixEnd], s[suffixStart:]
}

func truncateWithByteBudget(s string, maxBytes int, kind TruncationPolicyKind) string {
	if s == "" {
		return ""
	}
	if maxBytes < 0 {
		maxBytes = 0
	}
	policy := TruncationPolicy{Kind: kind}

	if maxBytes == 0 {
		removed := removedUnitsForSource(policy, len(s), utf8.RuneCountInString(s))
		return formatTruncationMarker(policy, removed)
	}
	if len(s) <= maxBytes {
		return s
	}

	leftBudget, rightBudget := splitBudget(maxBytes)
	removedRunes, prefix, suffix := splitStringUTF8(s, leftBudget, rightBudget)

	removedBytes := len(s) - maxBytes
	removed := removedUnitsForSource(policy, removedBytes, removedRunes)
	marker := formatTruncationMarker(policy, removed)
	return prefix + marker + suffix
}

func formatTruncationMarker(policy TruncationPolicy, removedCount uint64) string {
	switch policy.Kind {
	case TruncationTokens:
		return "…" + u64toa(removedCount) + " tokens truncated…"
	case TruncationBytes:
		return "…" + u64toa(removedCount) + " chars truncated…"
	default:
		return "…truncated…"
	}
}

func removedUnitsForSource(policy TruncationPolicy, removedBytes int, removedRunes int) uint64 {
	switch policy.Kind {
	case TruncationTokens:
		return approxTokensFromByteCount(removedBytes)
	case TruncationBytes:
		if removedRunes <= 0 {
			return 0
		}
		return uint64(removedRunes)
	default:
		return 0
	}
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func u64toa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
