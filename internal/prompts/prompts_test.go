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

func TestBuildLanguagePrompt(t *testing.T) {
	zh := BuildLanguagePrompt("")
	if !IsLanguagePrompt(zh) || !strings.Contains(zh, "中文") {
		t.Fatalf("expected chinese language prompt, got %q", zh)
	}
	en := BuildLanguagePrompt(i18n.LanguageEnglish)
	if !strings.Contains(en, "English") {
		t.Fatalf("expected english language prompt, got %q", en)
	}
	fr := BuildLanguagePrompt(i18n.Language("fr"))
	if !strings.Contains(fr, "fr") {
		t.Fatalf("expected to mention raw language code, got %q", fr)
	}
}

func TestIsLanguagePrompt(t *testing.T) {
	if IsLanguagePrompt("") {
		t.Fatalf("empty string should not match language prompt")
	}
	sample := BuildLanguagePrompt(i18n.LanguageChinese)
	if !IsLanguagePrompt(sample) {
		t.Fatalf("should detect rendered language prompt")
	}
	if IsLanguagePrompt("language: zh") {
		t.Fatalf("false positive for unrelated text")
	}
}
