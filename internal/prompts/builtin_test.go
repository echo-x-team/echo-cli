package prompts

import (
	"strings"
	"testing"
)

func TestBuiltinPromptsLoaded(t *testing.T) {
	checks := []Name{
		PromptCore,
		PromptLanguage,
		PromptGPT5Echo,
		PromptReview,
		PromptIssueLabeler,
	}
	for _, name := range checks {
		text, ok := Builtin(name)
		if !ok {
			t.Fatalf("missing builtin prompt %q", name)
		}
		if strings.TrimSpace(text) == "" {
			t.Fatalf("empty builtin prompt %q", name)
		}
	}
	if core, ok := Builtin(PromptCore); ok {
		if !strings.Contains(core, "*** Add File:") {
			t.Fatalf("core prompt should include Echo Patch directive guidance")
		}
	}
	if strings.TrimSpace(ReviewModeSystemPrompt) == "" {
		t.Fatalf("review prompt should not be empty")
	}
}

func TestBuiltinsReturnsCopy(t *testing.T) {
	all := Builtins()
	delete(all, PromptCore)
	if _, ok := builtinPrompts[PromptCore]; !ok {
		t.Fatalf("modifying Builtins result should not affect internal map")
	}
}
