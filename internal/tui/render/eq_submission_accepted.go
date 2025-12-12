package render

import (
	"echo-cli/internal/agent"
	"echo-cli/internal/events"
)

type submissionAcceptedRenderer struct{}

func (submissionAcceptedRenderer) Type() events.EventType { return events.EventSubmissionAccepted }

func (submissionAcceptedRenderer) Handle(ctx *Context, evt events.Event) {
	op, ok := evt.Payload.(events.Operation)
	if !ok {
		return
	}
	if op.UserInput == nil || ctx.Transcript == nil {
		return
	}
	ctx.ActiveSub = evt.SubmissionID
	for _, msg := range op.UserInput.Items {
		if msg.Role != string(agent.RoleUser) {
			continue
		}
		ctx.Emit(ctx.Transcript.AppendUser(msg.Content))
	}
}
