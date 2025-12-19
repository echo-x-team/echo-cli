package context

import (
	"strings"
	"testing"
)

func TestSplitStringUTF8(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		input       string
		prefixBytes int
		suffixBytes int
		removed     int
		prefix      string
		suffix      string
	}{
		{
			name:        "ascii",
			input:       "hello world",
			prefixBytes: 5,
			suffixBytes: 5,
			removed:     1,
			prefix:      "hello",
			suffix:      "world",
		},
		{
			name:        "empty",
			input:       "",
			prefixBytes: 4,
			suffixBytes: 4,
			removed:     0,
			prefix:      "",
			suffix:      "",
		},
		{
			name:        "only_prefix",
			input:       "abcdef",
			prefixBytes: 3,
			suffixBytes: 0,
			removed:     3,
			prefix:      "abc",
			suffix:      "",
		},
		{
			name:        "only_suffix",
			input:       "abcdef",
			prefixBytes: 0,
			suffixBytes: 3,
			removed:     3,
			prefix:      "",
			suffix:      "def",
		},
		{
			name:        "overlap_no_removal",
			input:       "abcdef",
			prefixBytes: 4,
			suffixBytes: 4,
			removed:     0,
			prefix:      "abcd",
			suffix:      "ef",
		},
		{
			name:        "utf8_boundaries_mix",
			input:       "😀abc😀",
			prefixBytes: 5,
			suffixBytes: 5,
			removed:     1,
			prefix:      "😀a",
			suffix:      "c😀",
		},
		{
			name:        "utf8_tiny_budgets",
			input:       "😀😀😀😀😀",
			prefixBytes: 1,
			suffixBytes: 1,
			removed:     5,
			prefix:      "",
			suffix:      "",
		},
		{
			name:        "utf8_mid_budgets",
			input:       "😀😀😀😀😀",
			prefixBytes: 7,
			suffixBytes: 7,
			removed:     3,
			prefix:      "😀",
			suffix:      "😀",
		},
		{
			name:        "utf8_larger_budgets",
			input:       "😀😀😀😀😀",
			prefixBytes: 8,
			suffixBytes: 8,
			removed:     1,
			prefix:      "😀😀",
			suffix:      "😀😀",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			removed, prefix, suffix := splitStringUTF8(tc.input, tc.prefixBytes, tc.suffixBytes)
			if removed != tc.removed {
				t.Fatalf("removed mismatch: got %d want %d", removed, tc.removed)
			}
			if prefix != tc.prefix {
				t.Fatalf("prefix mismatch: got %q want %q", prefix, tc.prefix)
			}
			if suffix != tc.suffix {
				t.Fatalf("suffix mismatch: got %q want %q", suffix, tc.suffix)
			}
		})
	}
}

func TestFormattedTruncateText_BytesPlaceholder(t *testing.T) {
	t.Parallel()

	got := FormattedTruncateText("example output", BytesPolicy(1))
	want := "Total output lines: 1\n\n…13 chars truncated…t"
	if got != want {
		t.Fatalf("unexpected output:\n%q\nwant:\n%q", got, want)
	}
}

func TestFormattedTruncateText_TokensPlaceholder(t *testing.T) {
	t.Parallel()

	got := FormattedTruncateText("example output", TokensPolicy(1))
	want := "Total output lines: 1\n\nex…3 tokens truncated…ut"
	if got != want {
		t.Fatalf("unexpected output:\n%q\nwant:\n%q", got, want)
	}
}

func TestFormattedTruncateText_UnderLimitReturnsOriginal(t *testing.T) {
	t.Parallel()

	content := "example output"
	if got := FormattedTruncateText(content, TokensPolicy(10)); got != content {
		t.Fatalf("should return original: %q", got)
	}
	if got := FormattedTruncateText(content, BytesPolicy(20)); got != content {
		t.Fatalf("should return original: %q", got)
	}
}

func TestFormattedTruncateText_ReportsOriginalLineCount(t *testing.T) {
	t.Parallel()

	content := "this is an example of a long output that should be truncated\nalso some other line"
	got := FormattedTruncateText(content, BytesPolicy(30))
	want := "Total output lines: 2\n\nthis is an exam…51 chars truncated…some other line"
	if got != want {
		t.Fatalf("unexpected output:\n%q\nwant:\n%q", got, want)
	}

	got = FormattedTruncateText(content, TokensPolicy(10))
	want = "Total output lines: 2\n\nthis is an example o…11 tokens truncated…also some other line"
	if got != want {
		t.Fatalf("unexpected output:\n%q\nwant:\n%q", got, want)
	}
}

func TestTruncateText_ZeroTokenLimitReturnsMarker(t *testing.T) {
	t.Parallel()

	got := TruncateText("abcdef", TokensPolicy(0))
	want := "…2 tokens truncated…"
	if got != want {
		t.Fatalf("unexpected output: %q want %q", got, want)
	}
}

func TestTruncateText_HandlesUTF8(t *testing.T) {
	t.Parallel()

	content := "😀😀😀😀😀😀😀😀😀😀\nsecond line with text\n"

	got := TruncateText(content, TokensPolicy(8))
	want := "😀😀😀😀…8 tokens truncated… line with text\n"
	if got != want {
		t.Fatalf("unexpected output:\n%q\nwant:\n%q", got, want)
	}

	got = TruncateText(content, BytesPolicy(20))
	want = "😀😀…21 chars truncated…with text\n"
	if got != want {
		t.Fatalf("unexpected output:\n%q\nwant:\n%q", got, want)
	}
}

func TestTruncateFunctionOutputContentItems_TokensBudget(t *testing.T) {
	t.Parallel()

	chunk := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau upsilon phi chi psi omega.\n"
	chunkTokens := ApproxTokenCount(chunk)
	if chunkTokens == 0 {
		t.Fatalf("chunk must consume tokens")
	}
	limit := chunkTokens * 3
	t1 := chunk
	t2 := chunk
	t3 := strings.Repeat(chunk, 10)
	t4 := chunk
	t5 := chunk

	items := []FunctionCallOutputContentItem{
		{Type: ContentItemInputText, Text: t1},
		{Type: ContentItemInputText, Text: t2},
		{Type: ContentItemInputImage, ImageURL: "img:mid"},
		{Type: ContentItemInputText, Text: t3},
		{Type: ContentItemInputText, Text: t4},
		{Type: ContentItemInputText, Text: t5},
	}

	out := truncateFunctionOutputContentItems(items, TokensPolicy(limit))

	if len(out) != 5 {
		t.Fatalf("expected 5 items, got %d", len(out))
	}
	if out[0].Type != ContentItemInputText || out[0].Text != t1 {
		t.Fatalf("unexpected first item: %+v", out[0])
	}
	if out[1].Type != ContentItemInputText || out[1].Text != t2 {
		t.Fatalf("unexpected second item: %+v", out[1])
	}
	if out[2].Type != ContentItemInputImage || out[2].ImageURL != "img:mid" {
		t.Fatalf("unexpected third item: %+v", out[2])
	}
	if out[3].Type != ContentItemInputText || !strings.Contains(out[3].Text, "tokens truncated") {
		t.Fatalf("unexpected fourth item: %+v", out[3])
	}
	if out[4].Type != ContentItemInputText || out[4].Text != "[omitted 2 text items ...]" {
		t.Fatalf("unexpected omitted marker: %+v", out[4])
	}
}
