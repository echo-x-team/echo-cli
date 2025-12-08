package tools

import "testing"

func TestParseMarkersBlock(t *testing.T) {
	text := "hello\n```tool\n{\"tool\":\"command\",\"id\":\"call-1\",\"args\":{\"command\":\"ls\"}}\n```"
	markers, err := ParseMarkers(text)
	if err != nil {
		t.Fatalf("parse markers: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(markers))
	}
	if markers[0].Tool != "command" || markers[0].ID != "call-1" {
		t.Fatalf("unexpected marker %+v", markers[0])
	}
}

func TestParseMarkersInline(t *testing.T) {
	text := `{"tool":"file_search","id":"fs1","args":{}}`
	markers, err := ParseMarkers(text)
	if err != nil {
		t.Fatalf("parse markers: %v", err)
	}
	if len(markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(markers))
	}
	if markers[0].Tool != "file_search" || markers[0].ID != "fs1" {
		t.Fatalf("unexpected marker %+v", markers[0])
	}
}
