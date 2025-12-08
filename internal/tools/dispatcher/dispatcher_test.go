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
	_ = New(policy.Policy{}, sandbox.NewRunner("danger-full-access"), bus, "/workspace", nil)
	marker := tools.ToolCallMarker{
		Tool: "apply_patch",
		ID:   "p1",
		Args: json.RawMessage(`{"patch":"diff","path":"file.txt"}`),
	}
	call, err := tools.BuildCallFromMarker(marker)
	if err != nil {
		t.Fatalf("build call error: %v", err)
	}
	if call.Name != "apply_patch" || call.ID != "p1" {
		t.Fatalf("unexpected call %+v", call)
	}
}
