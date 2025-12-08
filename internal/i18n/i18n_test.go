package i18n

import "testing"

func TestNormalize(t *testing.T) {
	if got := Normalize("").Code(); got != DefaultLanguage.Code() {
		t.Fatalf("empty normalize should fall back to default, got %q", got)
	}
	if got := Normalize("EN-us"); got != LanguageEnglish {
		t.Fatalf("expected english normalization, got %q", got)
	}
	if got := Normalize("ja"); got != Language("ja") {
		t.Fatalf("expected passthrough for unknown language, got %q", got)
	}
}

func TestDisplayName(t *testing.T) {
	if name := LanguageChinese.DisplayName(); name != "中文" {
		t.Fatalf("unexpected chinese display name: %q", name)
	}
	if name := LanguageEnglish.DisplayName(); name != "English" {
		t.Fatalf("unexpected english display name: %q", name)
	}
	if name := Language("fr").DisplayName(); name != "fr" {
		t.Fatalf("unexpected passthrough display name: %q", name)
	}
}
