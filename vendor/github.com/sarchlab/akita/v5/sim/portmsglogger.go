package sim

import (
	"log"

	"github.com/sarchlab/akita/v5/hooking"
)

// PortMsgLogger is a hook for logging messages as they go across a Port
type PortMsgLogger struct {
	LogHookBase
	TimeTeller
}

// NewPortMsgLogger returns a new PortMsgLogger which will write into the logger
func NewPortMsgLogger(
	logger *log.Logger,
	timeTeller TimeTeller,
) *PortMsgLogger {
	h := new(PortMsgLogger)
	h.Logger = logger
	h.TimeTeller = timeTeller

	return h
}

// Func writes the message information into the logger
func (h *PortMsgLogger) Func(ctx hooking.HookCtx) {
	msg, ok := ctx.Item.(Msg)
	if !ok {
		return
	}

	h.Logger.Printf("%d,%s,%s,%s,%s,%d\n",
		h.CurrentTime(),
		ctx.Domain.(Port).Name(),
		ctx.Pos.Name,
		msg.Meta().Src,
		msg.Meta().Dst,
		msg.Meta().ID)
}
