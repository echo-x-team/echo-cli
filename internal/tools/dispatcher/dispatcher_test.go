package dispatcher

import (
	"encoding/json"
	"testing"

	"echo-cli/internal/events"
	"echo-cli/internal/policy"
	"echo-cli/internal/sandbox"
	"echo-cli/internal/tools"
)

func TestToRequestResolvesPath(t *testing.T) {
	bus := events.NewBus()
	d := New(policy.Policy{}, sandbox.NewRunner("danger-full-access"), bus, "/workspace", nil)
	marker := tools.ToolCallMarker{
		Tool: "apply_patch",
		ID:   "p1",
		Args: json.RawMessage(`{"patch":"diff","path":"file.txt"}`),
	}
	req, err := d.toRequest(marker)
	if err != nil {
		t.Fatalf("toRequest error: %v", err)
	}
	if req.Path != "/workspace/file.txt" {
		t.Fatalf("expected resolved path, got %s", req.Path)
	}
}
