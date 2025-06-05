package rob

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// The portHook hooks on the top port of the reorder buffer to monitor the
// in and out of the reorder buffer. The portHook triggers `AddMilestone` API
// to record the time that a message is added to the reorder buffer and the time
// and the time that a message is swapped to the front of the buffer.
type portHook struct {
}

func (h *portHook) Func(ctx sim.HookCtx) {
	switch ctx.Pos {
	case sim.HookPosPortMsgRecvd:
		h.recordMsgRecvd(ctx)
	case sim.HookPosPortMsgRetrieve:
		h.recordMsgRetrieved(ctx)
	}
}

func (h *portHook) recordMsgRecvd(ctx sim.HookCtx) {
	msg := ctx.Item.(sim.Msg)
	port := ctx.Domain.(sim.Port)
	comp := port.Component().(tracing.NamedHookable)

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, comp),
		tracing.MilestoneKindNetworkTransfer,
		"",
		port.Name(),
		comp,
	)
}

func (h *portHook) recordMsgRetrieved(ctx sim.HookCtx) {
	port := ctx.Domain.(sim.Port)
	comp := port.Component().(tracing.NamedHookable)
	head := port.PeekIncoming()

	if head == nil {
		return
	}

	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(head, comp),
		tracing.MilestoneKindQueue,
		"",
		port.Name(),
		comp,
	)
}
