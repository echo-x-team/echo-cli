package engine

import (
	"sync"

	"echo-cli/internal/tools"
)

// UIApprover blocks tool execution until Resolve is called for a request id.
type UIApprover struct {
	mu      sync.Mutex
	pending map[string]chan bool
}

func NewUIApprover() *UIApprover {
	return &UIApprover{pending: make(map[string]chan bool)}
}

func (a *UIApprover) Approve(call tools.ToolCall) bool {
	ch := a.get(call.ID)
	return <-ch
}

func (a *UIApprover) Resolve(id string, allow bool) {
	ch := a.get(id)
	select {
	case ch <- allow:
	default:
	}
	close(ch)
	a.mu.Lock()
	delete(a.pending, id)
	a.mu.Unlock()
}

func (a *UIApprover) get(id string) chan bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ch, ok := a.pending[id]; ok {
		return ch
	}
	ch := make(chan bool, 1)
	a.pending[id] = ch
	return ch
}
