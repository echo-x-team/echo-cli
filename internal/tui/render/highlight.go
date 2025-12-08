package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	dimStyle = lipgloss.NewStyle().Faint(true)
)

// HighlightBashToLines 使用轻量规则高亮 Bash。
// 注释、运算符与字符串被 dim，异常时回退为纯文本。
func HighlightBashToLines(script string) []Line {
	lines := []Line{}
	for _, rawLine := range strings.Split(script, "\n") {
		if rawLine == "" {
			lines = append(lines, Line{})
			continue
		}
		spans := []Span{}
		if strings.HasPrefix(strings.TrimSpace(rawLine), "#") {
			spans = append(spans, Span{Text: rawLine, Style: dimStyle})
			lines = append(lines, Line{Spans: spans})
			continue
		}
		tokens := strings.Fields(rawLine)
		if len(tokens) == 0 {
			lines = append(lines, Line{})
			continue
		}
		rest := rawLine
		for _, tok := range tokens {
			idx := strings.Index(rest, tok)
			if idx > 0 {
				spans = append(spans, Span{Text: rest[:idx]})
			}
			style := lipgloss.Style{}
			if isOperator(tok) || isString(tok) {
				style = dimStyle
			}
			spans = append(spans, Span{Text: tok, Style: style})
			rest = rest[idx+len(tok):]
		}
		if rest != "" {
			spans = append(spans, Span{Text: rest})
		}
		lines = append(lines, Line{Spans: spans})
	}
	if len(lines) == 0 {
		return []Line{{}}
	}
	return lines
}

func isOperator(tok string) bool {
	switch tok {
	case "&&", "||", "|", "&", ">", ">>", "<", "<<":
		return true
	default:
		return false
	}
}

func isString(tok string) bool {
	if strings.HasPrefix(tok, "\"") && strings.HasSuffix(tok, "\"") {
		return true
	}
	if strings.HasPrefix(tok, "'") && strings.HasSuffix(tok, "'") {
		return true
	}
	return false
}
