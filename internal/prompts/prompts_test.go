package prompts

import (
	"strings"
	"testing"

	"echo-cli/internal/i18n"
)

func TestBuildReasoningEffort(t *testing.T) {
	if out := BuildReasoningEffort(""); out != "" {
		t.Fatalf("expected empty for blank effort, got %q", out)
	}
	out := BuildReasoningEffort("高")
	if out != "推理强度：高" {
		t.Fatalf("unexpected reasoning effort prompt: %q", out)
	}
}

func TestExtractReasoningEffort(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "empty",
			text: "",
			want: "",
		},
		{
			name: "chinese prefix",
			text: "推理强度：中\n其他说明",
			want: "中",
		},
		{
			name: "legacy english prefix",
			text: "Reasoning effort: medium",
			want: "medium",
		},
		{
			name: "leading spaces",
			text: "  推理强度：  高  ",
			want: "高",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractReasoningEffort(tt.text); got != tt.want {
				t.Fatalf("ExtractReasoningEffort() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveReference(t *testing.T) {
	text, ok := ResolveReference("@internal/prompts/compact")
	if !ok || strings.TrimSpace(text) == "" {
		t.Fatalf("expected compact prompt text, got ok=%v len=%d", ok, len(text))
	}
	if _, ok := ResolveReference("@internal/prompts/does-not-exist"); ok {
		t.Fatalf("unexpected ok for unknown prompt")
	}
	if _, ok := ResolveReference("not a ref"); ok {
		t.Fatalf("unexpected ok for non-prefixed text")
	}
}

func TestBuildLanguageInstruction(t *testing.T) {
	zh := BuildLanguageInstruction("")
	if !strings.HasPrefix(zh, "默认语言：中文") {
		t.Fatalf("expected chinese directive when empty, got %q", zh)
	}
	en := BuildLanguageInstruction(i18n.LanguageEnglish)
	if !strings.HasPrefix(en, "Default language: English") {
		t.Fatalf("expected english directive, got %q", en)
	}
	fr := BuildLanguageInstruction(i18n.Language("fr"))
	if !strings.Contains(fr, "fr") {
		t.Fatalf("expected to mention raw language code, got %q", fr)
	}
}

func TestHasLanguageInstruction(t *testing.T) {
	if HasLanguageInstruction("") {
		t.Fatalf("empty string should not contain language directive")
	}
	if !HasLanguageInstruction("默认语言：中文。按此回复") {
		t.Fatalf("failed to detect chinese directive")
	}
	if !HasLanguageInstruction("  Default language: English. Keep it.") {
		t.Fatalf("failed to detect english directive with leading spaces")
	}
	if !HasLanguageInstruction("system intro\nDefault language: English.") {
		t.Fatalf("failed to detect directive when not at prefix")
	}
	if HasLanguageInstruction("language: zh") {
		t.Fatalf("false positive for non-directive text")
	}
}
