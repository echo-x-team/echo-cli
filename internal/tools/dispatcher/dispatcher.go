package dispatcher

import (
	"context"

	"echo-cli/internal/events"
	"echo-cli/internal/policy"
	"echo-cli/internal/sandbox"
	"echo-cli/internal/tools"
	"echo-cli/internal/tools/handlers"
)

type Dispatcher struct {
	runtime *tools.Runtime
	bus     *events.Bus
}

func New(pol policy.Policy, runner sandbox.Runner, bus *events.Bus, workdir string, approver tools.Approver) *Dispatcher {
	return &Dispatcher{
		runtime: tools.NewRuntime(pol, runner, approver, workdir, handlers.Default()),
		bus:     bus,
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	if d.bus == nil {
		return
	}
	ch := d.bus.Subscribe()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				marker, ok := evt.(tools.ToolCallMarker)
				if !ok {
					continue
				}
				call, err := tools.BuildCallFromMarker(marker)
				if err != nil {
					continue
				}
				go d.runtime.Dispatch(ctx, call, func(ev tools.ToolEvent) {
					d.bus.Publish(ev)
				})
			}
		}
	}()
}
