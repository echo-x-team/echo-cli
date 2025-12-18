package main

import (
	"testing"

	"echo-cli/internal/agent"
	"echo-cli/internal/prompts"
)

func TestExtractConversationHistory_FiltersNonModelRolesAndInjectedSystem(t *testing.T) {
	t.Parallel()

	input := []agent.Message{
		{Role: agent.RoleUser, Content: "u1"},
		{Role: agent.Role("tool"), Content: "tool block"},
		{Role: agent.RoleAssistant, Content: "a1"},
		{Role: agent.RoleSystem, Content: prompts.OutputSchemaPrefix + `{"type":"object"}`},
		{Role: agent.RoleSystem, Content: prompts.ReviewModeSystemPrompt},
		{Role: agent.RoleSystem, Content: "keep system"},
		{Role: agent.Role("weird"), Content: "unknown role"},
	}

	got := extractConversationHistory(input)

	// Expect: user, assistant, and the non-injected system message only.
	wantRoles := []agent.Role{agent.RoleUser, agent.RoleAssistant, agent.RoleSystem}
	if len(got) != len(wantRoles) {
		t.Fatalf("expected %d messages, got %d: %#v", len(wantRoles), len(got), got)
	}
	for i, want := range wantRoles {
		if got[i].Role != want {
			t.Fatalf("expected got[%d].Role=%q, got %q (msg=%#v)", i, want, got[i].Role, got[i])
		}
	}
	if got[2].Content != "keep system" {
		t.Fatalf("expected to keep system content, got %#v", got[2])
	}
}
