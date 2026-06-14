package modeling

import (
	"math"
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// EventProcessor defines the processing logic for an EventDrivenComponent.
// S is the Spec type, T is the State type, R is the Resources type.
type EventProcessor[S any, T any, R any] interface {
	Process(comp *EventDrivenComponent[S, T, R], now timing.VTimeInPicoSec) bool
}

// TimerFiredEvent is the event scheduled by EventDrivenComponent to wake
// itself up at a future time.
type TimerFiredEvent struct {
	timing.EventBase
}

// MakeTimerFiredEvent creates a new TimerFiredEvent.
func MakeTimerFiredEvent(handlerID string, time timing.VTimeInPicoSec) TimerFiredEvent {
	return TimerFiredEvent{EventBase: timing.MakeEventBase(time, handlerID)}
}

// EventDrivenComponent is a generic component that reacts to events rather
// than ticking at a fixed frequency.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
// R is the Resources type (references to shared resources; None when unused).
//
// Instead of periodic ticking, EventDrivenComponent schedules wakeup events
// via [ScheduleWakeAt] or [ScheduleWakeNow]. A dedup guard (pendingWakeup)
// prevents redundant event scheduling.
type EventDrivenComponent[S any, T any, R any] struct {
	sync.Mutex
	hooking.HookableBase
	*messaging.PortOwnerBase

	engine    timing.EventScheduler
	name      string
	spec      S
	State     T
	resources R
	processor EventProcessor[S, T, R]

	pendingWakeup timing.VTimeInPicoSec
}

// Spec returns the component's immutable configuration (a copy), so callers
// cannot mutate the builder-established configuration.
func (c *EventDrivenComponent[S, T, R]) Spec() S {
	return c.spec
}

// Resources returns the component's shared-resource references. The references
// are fixed at construction; only the state they point to mutates.
func (c *EventDrivenComponent[S, T, R]) Resources() R {
	return c.resources
}

// Name returns the component name.
func (c *EventDrivenComponent[S, T, R]) Name() string {
	return c.name
}

// ScheduleWakeAt schedules a wakeup at time t. If a wakeup is already
// pending at the same or earlier time, this is a no-op (dedup guard).
func (c *EventDrivenComponent[S, T, R]) ScheduleWakeAt(t timing.VTimeInPicoSec) {
	if c.pendingWakeup != math.MaxUint64 && c.pendingWakeup <= t {
		return
	}

	c.pendingWakeup = t

	c.engine.Schedule(MakeTimerFiredEvent(c.Name(), t))
}

// ScheduleWakeNow schedules a wakeup at the current engine time.
func (c *EventDrivenComponent[S, T, R]) ScheduleWakeNow() {
	c.ScheduleWakeAt(c.engine.CurrentTime())
}

// Handle processes an event. For TimerFiredEvent, it resets the dedup guard
// and calls the processor.
func (c *EventDrivenComponent[S, T, R]) Handle(e timing.Event) error {
	c.Lock()
	defer c.Unlock()

	c.pendingWakeup = math.MaxUint64
	c.processor.Process(c, e.Time())

	return nil
}

// NotifyRecv is called when a port receives a message. It schedules an
// immediate wakeup.
func (c *EventDrivenComponent[S, T, R]) NotifyRecv(port messaging.Port) {
	c.ScheduleWakeNow()
}

// NotifyPortFree is called when a port becomes free. It schedules an
// immediate wakeup.
func (c *EventDrivenComponent[S, T, R]) NotifyPortFree(port messaging.Port) {
	c.ScheduleWakeNow()
}
