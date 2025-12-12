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
	if len(state.Messages) != 4 {
		t.Fatalf("expected 4 messages with language prompt, got %d", len(state.Messages))
	}
	if state.Messages[0].Role != agent.RoleSystem {
		t.Fatalf("unexpected system role: %+v", state.Messages[0])
	}
	if !strings.Contains(state.Messages[0].Content, "sys") || prompts.IsLanguagePrompt(state.Messages[0].Content) {
		t.Fatalf("system message missing content or contains language prompt: %+v", state.Messages[0])
	}
	if state.Messages[1].Role != agent.RoleSystem || state.Messages[1].Content != "one\ntwo" {
		t.Fatalf("unexpected instructions message: %+v", state.Messages[1])
	}
	if state.Messages[2].Role != agent.RoleUser || state.Messages[2].Content != "hi" {
		t.Fatalf("unexpected history message: %+v", state.Messages[2])
	}
	if state.Messages[3].Role != agent.RoleSystem || !prompts.IsLanguagePrompt(state.Messages[3].Content) || !strings.Contains(state.Messages[3].Content, "中文") {
		t.Fatalf("language prompt missing or misplaced: %+v", state.Messages[3])
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
	if len(state.Messages) != 4 {
		t.Fatalf("expected 4 messages with reasoning and language prompt, got %d", len(state.Messages))
	}
	if state.Messages[0].Content != prompts.BuildReasoningEffort("高") {
		t.Fatalf("reasoning effort missing: %+v", state.Messages[0])
	}
	if !strings.Contains(state.Messages[1].Content, "sys") || prompts.IsLanguagePrompt(state.Messages[1].Content) {
		t.Fatalf("system prompt missing or language prompt misplaced: %+v", state.Messages[1])
	}
	if state.Messages[3].Role != agent.RoleSystem || !prompts.IsLanguagePrompt(state.Messages[3].Content) {
		t.Fatalf("language prompt missing at tail: %+v", state.Messages[3])
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
	if len(state.Messages) != 3 {
		t.Fatalf("expected reuse of existing reasoning plus language prompt, got %d messages", len(state.Messages))
	}
	if strings.Contains(state.Messages[0].Content, "高") {
		t.Fatalf("reasoning effort should not be overridden: %q", state.Messages[0].Content)
	}
	if got := prompts.ExtractReasoningEffort(state.Messages[0].Content); got != "中" {
		t.Fatalf("existing reasoning effort lost: %q", state.Messages[0].Content)
	}
	if state.Messages[2].Role != agent.RoleSystem || !prompts.IsLanguagePrompt(state.Messages[2].Content) {
		t.Fatalf("language prompt missing at tail: %+v", state.Messages[2])
	}
}

func TestTurnContextInjectsOutputSchema(t *testing.T) {
	core, _ := prompts.Builtin(prompts.PromptCore)
	ctx := TurnContext{
		OutputSchema: `{"type":"object"}`,
		History:      []agent.Message{{Role: agent.RoleUser, Content: "task"}},
	}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 4 {
		t.Fatalf("expected default system + schema + history + language, got %d", len(state.Messages))
	}
	if state.Messages[0].Role != agent.RoleSystem {
		t.Fatalf("schema should be system message, got %s", state.Messages[0].Role)
	}
	if !strings.Contains(state.Messages[0].Content, core) || prompts.IsLanguagePrompt(state.Messages[0].Content) {
		t.Fatalf("core prompt missing or language prompt misplaced, got %q", state.Messages[0].Content)
	}
	if state.Messages[1].Content != prompts.OutputSchemaPrefix+`{"type":"object"}` {
		t.Fatalf("schema content mismatch: %q", state.Messages[1].Content)
	}
	if state.Messages[3].Role != agent.RoleSystem || !prompts.IsLanguagePrompt(state.Messages[3].Content) {
		t.Fatalf("language prompt missing at tail: %+v", state.Messages[3])
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
	if len(state.Messages) != 4 {
		t.Fatalf("expected default system + existing history reused + language, got %d", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, core) || prompts.IsLanguagePrompt(state.Messages[0].Content) {
		t.Fatalf("core prompt missing or language prompt misplaced, got %q", state.Messages[0].Content)
	}
	if state.Messages[1].Content != prompts.OutputSchemaPrefix+"existing" {
		t.Fatalf("existing schema should be preserved, got %q", state.Messages[1].Content)
	}
	if state.Messages[3].Role != agent.RoleSystem || !prompts.IsLanguagePrompt(state.Messages[3].Content) {
		t.Fatalf("language prompt missing at tail: %+v", state.Messages[3])
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
	if len(state.Messages) != 3 {
		t.Fatalf("expected system + instructions + language messages, got %d", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, compact) || prompts.IsLanguagePrompt(state.Messages[0].Content) {
		t.Fatalf("system prompt not resolved or language prompt misplaced: %q", state.Messages[0].Content)
	}
	if strings.TrimSpace(state.Messages[1].Content) != prefix {
		t.Fatalf("instructions prompt not resolved: %q", state.Messages[1].Content)
	}
	if state.Messages[2].Role != agent.RoleSystem || !prompts.IsLanguagePrompt(state.Messages[2].Content) {
		t.Fatalf("language prompt missing at tail: %+v", state.Messages[2])
	}
}

func TestTurnContextFallsBackToCorePromptWhenSystemMissing(t *testing.T) {
	core, _ := prompts.Builtin(prompts.PromptCore)
	ctx := TurnContext{}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 2 {
		t.Fatalf("expected default system and language prompt, got %d messages", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, core) || prompts.IsLanguagePrompt(state.Messages[0].Content) {
		t.Fatalf("default core prompt missing, got %q", state.Messages[0].Content)
	}
	if state.Messages[1].Role != agent.RoleSystem || !prompts.IsLanguagePrompt(state.Messages[1].Content) {
		t.Fatalf("language prompt missing at tail: %+v", state.Messages[1])
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
	if len(state.Messages) != 2 {
		t.Fatalf("expected default system prompt and language prompt, got %d messages", len(state.Messages))
	}
	if !strings.Contains(state.Messages[0].Content, core) || prompts.IsLanguagePrompt(state.Messages[0].Content) {
		t.Fatalf("default core prompt missing, got %q", state.Messages[0].Content)
	}
	if state.Messages[1].Role != agent.RoleSystem || !prompts.IsLanguagePrompt(state.Messages[1].Content) {
		t.Fatalf("language prompt missing at tail: %+v", state.Messages[1])
	}
}

func TestTurnContextAvoidsDuplicateLanguagePrompt(t *testing.T) {
	directive := prompts.BuildLanguagePrompt(i18n.LanguageEnglish)
	ctx := TurnContext{
		System:   directive,
		Language: "en",
	}

	state := ctx.BuildPrompt()
	if len(state.Messages) != 2 {
		t.Fatalf("expected system prompt plus trailing language prompt, got %d", len(state.Messages))
	}
	if prompts.IsLanguagePrompt(state.Messages[0].Content) {
		t.Fatalf("language prompt should not be placed before tail: %+v", state.Messages[0])
	}
	if state.Messages[1].Role != agent.RoleSystem || !prompts.IsLanguagePrompt(state.Messages[1].Content) || !strings.Contains(state.Messages[1].Content, "English") {
		t.Fatalf("language prompt missing or incorrect at tail: %+v", state.Messages[1])
	}
}
