package sim

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/hooking"
)

// EventLogger is an hook that prints the event information
type EventLogger struct {
	LogHookBase
}

// NewEventLogger returns a new LogEventHook which will write in to the logger
func NewEventLogger(logger *log.Logger) *EventLogger {
	h := new(EventLogger)
	h.Logger = logger

	return h
}

// Func writes the event information into the logger
func (h *EventLogger) Func(ctx hooking.HookCtx) {
	if ctx.Pos != HookPosBeforeEvent {
		return
	}

	evt, ok := ctx.Item.(Event)
	if !ok {
		return
	}

	h.Logger.Printf("%d, %s -> %s",
		evt.Time(), reflect.TypeOf(evt), evt.HandlerID())
}
