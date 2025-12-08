package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"echo-cli/internal/events"
	"echo-cli/internal/policy"
	"echo-cli/internal/sandbox"
	"echo-cli/internal/tools"
	"echo-cli/internal/tools/engine"
)

type Dispatcher struct {
	engine  *engine.Engine
	bus     *events.Bus
	workdir string
}

func New(pol policy.Policy, runner sandbox.Runner, bus *events.Bus, workdir string, approver engine.Approver) *Dispatcher {
	return &Dispatcher{
		engine:  engine.New(pol, runner, approver, workdir),
		bus:     bus,
		workdir: workdir,
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
				req, err := d.toRequest(marker)
				if err != nil {
					continue
				}
				go d.engine.Run(ctx, req, func(ev tools.ToolEvent) {
					d.bus.Publish(ev)
				})
			}
		}
	}()
}

func (d *Dispatcher) toRequest(marker tools.ToolCallMarker) (tools.ToolRequest, error) {
	req := tools.ToolRequest{ID: marker.ID}
	switch marker.Tool {
	case "command":
		req.Kind = tools.ToolCommand
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(marker.Args, &args); err != nil {
			return req, err
		}
		req.Command = args.Command
	case "apply_patch":
		req.Kind = tools.ToolApplyPatch
		var args struct {
			Patch string `json:"patch"`
			Path  string `json:"path"`
		}
		if err := json.Unmarshal(marker.Args, &args); err != nil {
			return req, err
		}
		req.Patch = args.Patch
		req.Path = args.Path
	case "file_read":
		req.Kind = tools.ToolFileRead
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(marker.Args, &args); err != nil {
			return req, err
		}
		req.Path = args.Path
	case "file_search":
		req.Kind = tools.ToolSearch
	default:
		return req, fmt.Errorf("unknown tool: %s", marker.Tool)
	}
	if req.Path != "" && !filepath.IsAbs(req.Path) && d.workdir != "" {
		req.Path = filepath.Join(d.workdir, req.Path)
	}
	return req, nil
}
