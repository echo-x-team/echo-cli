package render

import "echo-cli/internal/events"

type agentOutputRenderer struct{}

func (agentOutputRenderer) Type() events.EventType { return events.EventAgentOutput }

func (agentOutputRenderer) Handle(ctx *Context, evt events.Event) {
	if ctx.Transcript == nil {
		return
	}
	if ctx.ActiveSub != "" && evt.SubmissionID != ctx.ActiveSub {
		return
	}
	msg, ok := evt.Payload.(events.AgentOutput)
	if !ok {
		return
	}
	if msg.Final {
		ctx.Emit(ctx.Transcript.FinalizeAssistant(msg.Content))
		ctx.ActiveSub = ""
		return
	}
	if msg.Content != "" {
		ctx.Emit(ctx.Transcript.AppendAssistantChunk(msg.Content))
	}
}
