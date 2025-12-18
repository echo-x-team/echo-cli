package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreAppendAndLoadTexts(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "history.jsonl")
	s := &Store{Path: path}

	if got, err := s.LoadTexts(); err != nil || len(got) != 0 {
		t.Fatalf("LoadTexts on missing file: got=%v err=%v", got, err)
	}

	if err := s.Append("   "); err != nil {
		t.Fatalf("Append whitespace: %v", err)
	}

	if err := s.Append("one"); err != nil {
		t.Fatalf("Append one: %v", err)
	}
	if err := s.Append("two"); err != nil {
		t.Fatalf("Append two: %v", err)
	}

	// Inject garbage line; loader should skip it.
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		`{"text":"one","ts":"2025-01-01T00:00:00Z"}`,
		`{not json}`,
		`{"text":"two","ts":"2025-01-01T00:00:00Z"}`,
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := s.LoadTexts()
	if err != nil {
		t.Fatalf("LoadTexts: %v", err)
	}
	want := []string{"one", "two"}
	if len(got) != len(want) {
		t.Fatalf("LoadTexts len=%d want=%d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("LoadTexts[%d]=%q want=%q", i, got[i], want[i])
		}
	}
}

func TestStoreAppendErrors(t *testing.T) {
	t.Parallel()

	var s *Store
	if err := s.Append("hi"); err == nil {
		t.Fatalf("expected error for nil store")
	}

	s = &Store{}
	if err := s.Append("hi"); err == nil {
		t.Fatalf("expected error for empty path")
	}
}
