package tools

import "context"

// Handler 定义具体工具的执行入口。
type Handler interface {
	Name() string
	Kind() ToolKind
	SupportsParallel() bool
	IsMutating(inv Invocation) bool
	Describe(inv Invocation) ToolResult
	Handle(ctx context.Context, inv Invocation) (ToolResult, error)
}

type Registry struct {
	handlers map[string]Handler
}

func NewRegistry(handlers ...Handler) *Registry {
	table := make(map[string]Handler, len(handlers))
	for _, h := range handlers {
		if h == nil {
			continue
		}
		table[h.Name()] = h
	}
	return &Registry{handlers: table}
}

func (r *Registry) Handler(name string) (Handler, bool) {
	h, ok := r.handlers[name]
	return h, ok
}
