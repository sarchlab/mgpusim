package modeling

import (
	"math"

	"github.com/sarchlab/akita/v5/sim"
)

// EventDrivenBuilder constructs [EventDrivenComponent] instances.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
type EventDrivenBuilder[S any, T any] struct {
	engine    sim.EventScheduler
	spec      S
	processor EventProcessor[S, T]
}

// NewEventDrivenBuilder creates a new EventDrivenBuilder.
func NewEventDrivenBuilder[S any, T any]() EventDrivenBuilder[S, T] {
	return EventDrivenBuilder[S, T]{}
}

// WithEngine sets the simulation engine.
func (b EventDrivenBuilder[S, T]) WithEngine(engine sim.EventScheduler) EventDrivenBuilder[S, T] {
	b.engine = engine
	return b
}

// WithSpec sets the component specification.
func (b EventDrivenBuilder[S, T]) WithSpec(spec S) EventDrivenBuilder[S, T] {
	b.spec = spec
	return b
}

// WithProcessor sets the event processor.
func (b EventDrivenBuilder[S, T]) WithProcessor(
	processor EventProcessor[S, T],
) EventDrivenBuilder[S, T] {
	b.processor = processor
	return b
}

// Build creates the EventDrivenComponent with the given name.
func (b EventDrivenBuilder[S, T]) Build(name string) *EventDrivenComponent[S, T] {
	comp := &EventDrivenComponent[S, T]{
		ComponentBase: sim.NewComponentBase(name),
		Engine:        b.engine,
		spec:          b.spec,
		processor:     b.processor,
		pendingWakeup: math.MaxUint64,
	}

	if registrar, ok := b.engine.(sim.HandlerRegistrar); ok {
		registrar.RegisterHandler(name, comp)
	}

	return comp
}
