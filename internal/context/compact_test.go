package context

import (
	"strings"
	"testing"
)

func TestCollectUserMessages_FiltersSessionPrefixAndSummary(t *testing.T) {
	t.Parallel()

	summaryPrefix := "SUMMARY_PREFIX"
	items := []ResponseItem{
		NewUserMessageItem("<environment_context>\n<cwd>/tmp</cwd>\n</environment_context>"),
		NewUserMessageItem("# AGENTS.md instructions for /tmp\n\n<INSTRUCTIONS>\nhello\n</INSTRUCTIONS>"),
		NewUserMessageItem("do the work"),
		NewUserMessageItem(summaryPrefix + "\nold summary"),
		NewAssistantMessageItem("ack"),
	}

	initial := collectSessionPrefixItems(items)
	if len(initial) != 2 {
		t.Fatalf("expected 2 session prefix items, got %d", len(initial))
	}
	if FlattenContentItems(initial[0].Message.Content) == "" || !isSessionPrefixMessage(FlattenContentItems(initial[0].Message.Content)) {
		t.Fatalf("unexpected first session prefix item: %+v", initial[0])
	}
	if FlattenContentItems(initial[1].Message.Content) == "" || !isSessionPrefixMessage(FlattenContentItems(initial[1].Message.Content)) {
		t.Fatalf("unexpected second session prefix item: %+v", initial[1])
	}

	userMsgs := collectUserMessages(items, summaryPrefix)
	if len(userMsgs) != 1 || userMsgs[0] != "do the work" {
		t.Fatalf("unexpected user messages: %#v", userMsgs)
	}
}

func TestBuildCompactedHistory_AppendsSummaryAndUserMessages(t *testing.T) {
	t.Parallel()

	initial := []ResponseItem{
		NewUserMessageItem("<environment_context>\n<cwd>/tmp</cwd>\n</environment_context>"),
	}
	userMsgs := []string{"one", "two", "three"}
	summary := "SUMMARY_PREFIX\nsummary body"

	out := buildCompactedHistory(initial, userMsgs, summary, 20_000)
	if len(out) != 5 {
		t.Fatalf("expected 5 items (1 initial + 3 user + 1 summary), got %d", len(out))
	}
	if FlattenContentItems(out[0].Message.Content) != FlattenContentItems(initial[0].Message.Content) {
		t.Fatalf("initial context not preserved: %+v", out[0])
	}
	if FlattenContentItems(out[1].Message.Content) != "one" {
		t.Fatalf("unexpected message[1]: %+v", out[1])
	}
	if FlattenContentItems(out[2].Message.Content) != "two" {
		t.Fatalf("unexpected message[2]: %+v", out[2])
	}
	if FlattenContentItems(out[3].Message.Content) != "three" {
		t.Fatalf("unexpected message[3]: %+v", out[3])
	}
	if FlattenContentItems(out[4].Message.Content) != summary {
		t.Fatalf("unexpected summary message: %+v", out[4])
	}
}

func TestBuildCompactedHistory_TruncatesToTokenBudgetFromTail(t *testing.T) {
	t.Parallel()

	// Ensure the last message alone overflows the budget so the algorithm truncates it and stops.
	long := strings.Repeat("a", 200)
	if ApproxTokenCount(long) <= 5 {
		t.Fatalf("expected long message to exceed small token budget")
	}

	out := buildCompactedHistory(nil, []string{"older", long}, "SUMMARY_PREFIX\nok", 5)
	if len(out) != 2 {
		t.Fatalf("expected 1 truncated message + summary, got %d", len(out))
	}
	first := FlattenContentItems(out[0].Message.Content)
	if !strings.Contains(first, "tokens truncated") {
		t.Fatalf("expected truncation marker, got %q", first)
	}
}

func TestFindLastGhostSnapshot_PicksLast(t *testing.T) {
	t.Parallel()

	items := []ResponseItem{
		NewUserMessageItem("hello"),
		{
			Type:          ResponseItemTypeGhostSnapshot,
			GhostSnapshot: &GhostSnapshotResponseItem{GhostCommit: GhostCommit{ID: "g1"}},
		},
		NewUserMessageItem("world"),
		{
			Type:          ResponseItemTypeGhostSnapshot,
			GhostSnapshot: &GhostSnapshotResponseItem{GhostCommit: GhostCommit{ID: "g2"}},
		},
	}

	got := findLastGhostSnapshot(items)
	if got == nil || got.GhostSnapshot == nil || got.GhostSnapshot.GhostCommit.ID != "g2" {
		t.Fatalf("unexpected ghost snapshot: %#v", got)
	}
}
