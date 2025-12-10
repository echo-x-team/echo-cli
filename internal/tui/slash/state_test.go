package slash

import (
	"strings"
	"testing"
)

func TestSyncInputOpensOnSlashToken(t *testing.T) {
	state := NewState(Options{})
	state.SyncInput(Input{Value: "/mo", CursorLine: 0, CursorColumn: 3})
	if !state.Open() {
		t.Fatalf("expected slash popup to open")
	}
	if len(state.matches) == 0 {
		t.Fatalf("expected matches to be populated")
	}
}

func TestSyncInputOpensOnBareSlash(t *testing.T) {
	state := NewState(Options{})
	state.SyncInput(Input{Value: "/", CursorLine: 0, CursorColumn: 1})
	if !state.Open() {
		t.Fatalf("expected slash popup to open on bare slash")
	}
	if len(state.matches) == 0 {
		t.Fatalf("expected matches to populate on bare slash")
	}
}

func TestHandleKeyTabCompletesBuiltin(t *testing.T) {
	state := NewState(Options{})
	state.SyncInput(Input{Value: "/mo", CursorLine: 0, CursorColumn: 3})
	action, handled := state.HandleKey("tab")
	if !handled {
		t.Fatalf("expected tab handled")
	}
	if action.Kind != ActionInsert {
		t.Fatalf("expected insert action, got %v", action.Kind)
	}
	if strings.TrimSpace(action.NewValue) != "/model" {
		t.Fatalf("unexpected inserted value: %q", action.NewValue)
	}
	if action.CursorColumn == 0 {
		t.Fatalf("expected cursor to move")
	}
}

func TestHandleKeyEnterDispatchesCommand(t *testing.T) {
	state := NewState(Options{})
	state.SyncInput(Input{Value: "/model", CursorLine: 0, CursorColumn: 6})
	action, handled := state.HandleKey("enter")
	if !handled {
		t.Fatalf("expected enter handled")
	}
	if action.Kind != ActionSubmitCommand || action.Command != CommandModel {
		t.Fatalf("unexpected action %+v", action)
	}
}

func TestPromptPlaceholderInsertion(t *testing.T) {
	prompt := CustomPrompt{
		Name:         "foo",
		Placeholders: PromptPlaceholders{Named: []string{"ARG"}},
	}
	state := NewState(Options{CustomPrompts: []CustomPrompt{prompt}})
	state.SyncInput(Input{Value: "/prompts:foo", CursorLine: 0, CursorColumn: 12})
	action, handled := state.HandleKey("tab")
	if !handled {
		t.Fatalf("expected tab handled for prompt")
	}
	if action.Kind != ActionInsert {
		t.Fatalf("expected insert action, got %v", action.Kind)
	}
	if !strings.Contains(action.NewValue, `ARG=""`) {
		t.Fatalf("expected placeholder insertion, got %q", action.NewValue)
	}
	if action.CursorColumn <= len("/prompts:foo ") {
		t.Fatalf("cursor should land inside placeholder, got %d", action.CursorColumn)
	}
}

func TestResolveSubmitFallback(t *testing.T) {
	state := NewState(Options{})
	action := state.ResolveSubmit("/model")
	if action.Kind != ActionSubmitCommand || action.Command != CommandModel {
		t.Fatalf("expected submit command, got %+v", action)
	}
}
