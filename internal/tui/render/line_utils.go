package render

import "strings"

// LineToStatic 深拷贝行，便于安全缓存。
func LineToStatic(line Line) Line {
	spans := make([]Span, len(line.Spans))
	copy(spans, line.Spans)
	return Line{Spans: spans, Style: line.Style}
}

// PushOwnedLines 将源行拷贝到目标切片。
func PushOwnedLines(src []Line, out *[]Line) {
	if len(src) == 0 || out == nil {
		return
	}
	for _, l := range src {
		*out = append(*out, LineToStatic(l))
	}
}

// IsBlankLineSpacesOnly 判断行是否为空或仅包含空格。
func IsBlankLineSpacesOnly(line Line) bool {
	if len(line.Spans) == 0 {
		return true
	}
	for _, sp := range line.Spans {
		if strings.Trim(sp.Text, " ") != "" {
			return false
		}
	}
	return true
}

// PrefixLines 为首行/续行添加前缀。
func PrefixLines(lines []Line, initial Span, subsequent Span) []Line {
	out := make([]Line, 0, len(lines))
	for i, l := range lines {
		spans := make([]Span, 0, len(l.Spans)+1)
		if i == 0 {
			spans = append(spans, initial)
		} else {
			spans = append(spans, subsequent)
		}
		spans = append(spans, l.Spans...)
		out = append(out, Line{Spans: spans, Style: l.Style})
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
