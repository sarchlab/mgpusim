package rob

import (
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/tracing"
)

// The portHook hooks on the top port of the reorder buffer to monitor the
// in and out of the reorder buffer. The portHook triggers the `AddMilestone`
// API to record the time that a message is added to the reorder buffer and
// the time that a message is swapped to the front of the buffer.
type portHook struct {
}

func (h *portHook) Func(ctx hooking.HookCtx) {
	switch ctx.Pos {
	case messaging.HookPosPortMsgRecvd:
		h.recordMsgRecvd(ctx)
	case messaging.HookPosPortMsgRetrieveIncoming:
		h.recordMsgRetrieved(ctx)
	}
}

func (h *portHook) recordMsgRecvd(ctx hooking.HookCtx) {
	msg := ctx.Item.(messaging.Msg)
	port := ctx.Domain.(messaging.Port)
	comp := port.Component().(tracing.NamedHookable)

	tracing.AddMilestone(comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, comp),
		Kind:   tracing.MilestoneKindNetworkTransfer,
		What:   port.Name(),
	})
}

func (h *portHook) recordMsgRetrieved(ctx hooking.HookCtx) {
	port := ctx.Domain.(messaging.Port)
	comp := port.Component().(tracing.NamedHookable)
	head := port.PeekIncoming()

	if head == nil {
		return
	}

	tracing.AddMilestone(comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(head, comp),
		Kind:   tracing.MilestoneKindQueue,
		What:   "head",
	})
}
