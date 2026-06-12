package modeling

import (
	"encoding/json"
	"io"
	"math"

	"github.com/sarchlab/akita/v5/sim"
)

// EventProcessor defines the processing logic for an EventDrivenComponent.
// S is the Spec type, T is the State type.
type EventProcessor[S any, T any] interface {
	Process(comp *EventDrivenComponent[S, T], now sim.VTimeInSec) bool
}

// TimerFiredEvent is the event scheduled by EventDrivenComponent to wake
// itself up at a future time.
type TimerFiredEvent struct {
	*sim.EventBase
}

// EventDrivenComponent is a generic component that reacts to events rather
// than ticking at a fixed frequency.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
//
// Instead of periodic ticking, EventDrivenComponent schedules wakeup events
// via [ScheduleWakeAt] or [ScheduleWakeNow]. A dedup guard (pendingWakeup)
// prevents redundant event scheduling.
type EventDrivenComponent[S any, T any] struct {
	*sim.ComponentBase

	Engine    sim.EventScheduler
	spec      S
	current   T
	processor EventProcessor[S, T]

	pendingWakeup sim.VTimeInSec
}

// GetSpec returns the immutable specification.
func (c *EventDrivenComponent[S, T]) GetSpec() S {
	return c.spec
}

// GetState returns the current state.
func (c *EventDrivenComponent[S, T]) GetState() T {
	return c.current
}

// SetState sets the current state.
func (c *EventDrivenComponent[S, T]) SetState(s T) {
	c.current = s
}

// GetStatePtr returns a pointer to the current state for direct mutation.
func (c *EventDrivenComponent[S, T]) GetStatePtr() *T {
	return &c.current
}

// ScheduleWakeAt schedules a wakeup at time t. If a wakeup is already
// pending at the same or earlier time, this is a no-op (dedup guard).
func (c *EventDrivenComponent[S, T]) ScheduleWakeAt(t sim.VTimeInSec) {
	if c.pendingWakeup != math.MaxUint64 && c.pendingWakeup <= t {
		return
	}

	c.pendingWakeup = t

	evt := &TimerFiredEvent{
		EventBase: sim.NewEventBase(t, c.Name()),
	}
	c.Engine.Schedule(evt)
}

// ScheduleWakeNow schedules a wakeup at the current engine time.
func (c *EventDrivenComponent[S, T]) ScheduleWakeNow() {
	c.ScheduleWakeAt(c.Engine.CurrentTime())
}

// Handle processes an event. For TimerFiredEvent, it resets the dedup guard
// and calls the processor.
func (c *EventDrivenComponent[S, T]) Handle(e sim.Event) error {
	c.Lock()
	defer c.Unlock()

	c.pendingWakeup = math.MaxUint64
	c.processor.Process(c, e.Time())

	return nil
}

// NotifyRecv is called when a port receives a message. It schedules an
// immediate wakeup.
func (c *EventDrivenComponent[S, T]) NotifyRecv(port sim.Port) {
	c.ScheduleWakeNow()
}

// NotifyPortFree is called when a port becomes free. It schedules an
// immediate wakeup.
func (c *EventDrivenComponent[S, T]) NotifyPortFree(port sim.Port) {
	c.ScheduleWakeNow()
}

// ResetWakeup resets the pending wakeup guard to math.MaxUint64, allowing new
// wakeup events to be scheduled. This is used after loading state from a
// checkpoint.
func (c *EventDrivenComponent[S, T]) ResetWakeup() {
	c.pendingWakeup = math.MaxUint64
}

// SaveState marshals the component's spec and current state as JSON and writes
// it to w.
func (c *EventDrivenComponent[S, T]) SaveState(w io.Writer) error {
	snap := componentSnapshot[S, T]{
		Spec:  c.spec,
		State: c.current,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}

	_, err = w.Write(data)

	return err
}

// LoadState reads JSON from r and restores the component's spec and state.
func (c *EventDrivenComponent[S, T]) LoadState(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	var snap componentSnapshot[S, T]
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}

	c.spec = snap.Spec
	c.current = snap.State

	return nil
}
