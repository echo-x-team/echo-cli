package execution

import (
	"strings"
	"testing"

	"echo-cli/internal/agent"
	"echo-cli/internal/i18n"
	"echo-cli/internal/prompts"
)

func TestTurnContextBuildOrdersMessages(t *testing.T) {
	ctx := TurnContext{
		Model:        "gpt-test",
		System:       "sys",
		Instructions: []string{"one", "two"},
		History: []agent.Message{
			{Role: agent.RoleUser, Content: "hi"},
		},
	}

	state := ctx.BuildPrompt()
	if state.Model != "gpt-test" {
		t.Fatalf("model mismatch: %s", state.Model)
	}
	if len(state.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(state.Messages))
	}
	if state.Messages[0].Role != agent.RoleSystem {
		t.Fatalf("unexpected system role: %+v", state.Messages[0])
	}
	if !strings.Contains(state.Messages[0].Content, "sys") || !prompts.HasLanguageInstruction(state.Messages[0].Content) {
		t.Fatalf("system message missing content or language directive: %+v", state.Messages[0])
	}
	if state.Messages[1].Role != agent.RoleSystem || state.Messages[1].Content != "one\ntwo" {
		t.Fatalf("unexpected instructions message: %+v", state.Messages[1])
	}
	if state.Messages[2].Role != agent.RoleUser || state.Messages[2].Content != "hi" {
		t.Fatalf("unexpected history message: %+v", state.Messages[2])
	}
}

func TestTurnContextAddsReasoningEffort(t *testing.T) {
	ctx := TurnContext{
		Model:           "gpt-test",
		System:          "sys",
		ReasoningEffort: "高",
		History:         []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 3 {
		t.Fatalf("expected 3 messages with reasoning, got %d", len(state.Messages))
	}
	if state.Messages[0].Content != prompts.BuildReasoningEffort("高") {
		t.Fatalf("reasoning effort missing: %+v", state.Messages[0])
	}
	if !strings.Contains(state.Messages[1].Content, "sys") || !prompts.HasLanguageInstruction(state.Messages[1].Content) {
		t.Fatalf("system prompt missing or language directive absent: %+v", state.Messages[1])
	}
}

func TestTurnContextSkipsDuplicateReasoning(t *testing.T) {
	ctx := TurnContext{
		Model:           "gpt-test",
		System:          prompts.BuildReasoningEffort("中"),
		ReasoningEffort: "高",
		Instructions:    []string{"keep"},
	}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 2 {
		t.Fatalf("expected reuse of existing reasoning, got %d messages", len(state.Messages))
	}
	if strings.Contains(state.Messages[0].Content, "高") {
		t.Fatalf("reasoning effort should not be overridden: %q", state.Messages[0].Content)
	}
	if got := prompts.ExtractReasoningEffort(state.Messages[0].Content); got != "中" {
		t.Fatalf("existing reasoning effort lost: %q", state.Messages[0].Content)
	}
}

func TestTurnContextInjectsOutputSchema(t *testing.T) {
	core, _ := prompts.Builtin(prompts.PromptCore)
	ctx := TurnContext{
		OutputSchema: `{"type":"object"}`,
		History:      []agent.Message{{Role: agent.RoleUser, Content: "task"}},
	}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 3 {
		t.Fatalf("expected default system + schema + history, got %d", len(state.Messages))
	}
	if state.Messages[0].Role != agent.RoleSystem {
		t.Fatalf("schema should be system message, got %s", state.Messages[0].Role)
	}
	if !strings.Contains(state.Messages[0].Content, core) || !prompts.HasLanguageInstruction(state.Messages[0].Content) {
		t.Fatalf("core prompt or language directive missing, got %q", state.Messages[0].Content)
	}
	if state.Messages[1].Content != prompts.OutputSchemaPrefix+`{"type":"object"}` {
		t.Fatalf("schema content mismatch: %q", state.Messages[1].Content)
	}
}

func TestTurnContextAvoidsDuplicateSchema(t *testing.T) {
	core, _ := prompts.Builtin(prompts.PromptCore)
	ctx := TurnContext{
		OutputSchema: "ignored",
		History: []agent.Message{
			{Role: agent.RoleSystem, Content: prompts.OutputSchemaPrefix + "existing"},
			{Role: agent.RoleUser, Content: "ping"},
		},
	}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 3 {
		t.Fatalf("expected default system + existing history reused, got %d", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, core) || !prompts.HasLanguageInstruction(state.Messages[0].Content) {
		t.Fatalf("core prompt missing, got %q", state.Messages[0].Content)
	}
	if state.Messages[1].Content != prompts.OutputSchemaPrefix+"existing" {
		t.Fatalf("existing schema should be preserved, got %q", state.Messages[1].Content)
	}
}

func TestTurnContextResolvesPromptReferences(t *testing.T) {
	compact, _ := prompts.Builtin(prompts.PromptCompact)
	prefix, _ := prompts.Builtin(prompts.PromptCompactSummaryPrefix)
	ctx := TurnContext{
		System:       "@internal/prompts/compact",
		Instructions: []string{"@internal/prompts/compact-summary-prefix"},
	}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 2 {
		t.Fatalf("expected system + instructions messages, got %d", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, compact) || !prompts.HasLanguageInstruction(state.Messages[0].Content) {
		t.Fatalf("system prompt not resolved or missing directive: %q", state.Messages[0].Content)
	}
	if strings.TrimSpace(state.Messages[1].Content) != prefix {
		t.Fatalf("instructions prompt not resolved: %q", state.Messages[1].Content)
	}
}

func TestTurnContextFallsBackToCorePromptWhenSystemMissing(t *testing.T) {
	core, _ := prompts.Builtin(prompts.PromptCore)
	ctx := TurnContext{}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 1 {
		t.Fatalf("expected only default system prompt, got %d messages", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, core) || !prompts.HasLanguageInstruction(state.Messages[0].Content) {
		t.Fatalf("default core prompt missing, got %q", state.Messages[0].Content)
	}
}

func TestResolvePromptTextSkipsMissingReference(t *testing.T) {
	got := resolvePromptText("@internal/prompts/does-not-exist")
	if got != "" {
		t.Fatalf("missing reference should return empty for caller fallback, got %q", got)
	}
}

func TestResolveSystemPromptFallsBackWhenReferenceMissing(t *testing.T) {
	core, _ := prompts.Builtin(prompts.PromptCore)
	got := resolveSystemPrompt("@internal/prompts/does-not-exist")
	if got != core {
		t.Fatalf("system prompt should fall back to core prompt, got %q", got)
	}
}

func TestTurnContextSkipsUnknownInstructionReference(t *testing.T) {
	core, _ := prompts.Builtin(prompts.PromptCore)
	ctx := TurnContext{
		Instructions: []string{"@internal/prompts/does-not-exist"},
	}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 1 {
		t.Fatalf("expected only default system prompt, got %d messages", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, core) || !prompts.HasLanguageInstruction(state.Messages[0].Content) {
		t.Fatalf("default core prompt missing, got %q", state.Messages[0].Content)
	}
}

func TestTurnContextAvoidsDuplicateLanguageDirective(t *testing.T) {
	directive := prompts.BuildLanguageInstruction(i18n.LanguageEnglish)
	ctx := TurnContext{
		System:   directive,
		Language: "en",
	}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 1 {
		t.Fatalf("expected only system message, got %d", len(state.Messages))
	}
	count := strings.Count(strings.ToLower(state.Messages[0].Content), strings.ToLower(directive))
	if count != 1 {
		t.Fatalf("expected single language directive, got %d occurrences: %q", count, state.Messages[0].Content)
	}
}
