package modeling

import (
	"math"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// EventDrivenBuilder constructs [EventDrivenComponent] instances.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
// R is the Resources type (references to shared resources; None when unused).
type EventDrivenBuilder[S any, T any, R any] struct {
	engine    timing.EventScheduler
	spec      S
	resources R
	processor EventProcessor[S, T, R]
}

// NewEventDrivenBuilder creates a new EventDrivenBuilder.
func NewEventDrivenBuilder[S any, T any, R any]() EventDrivenBuilder[S, T, R] {
	return EventDrivenBuilder[S, T, R]{}
}

// WithEngine sets the simulation engine.
func (b EventDrivenBuilder[S, T, R]) WithEngine(engine timing.EventScheduler) EventDrivenBuilder[S, T, R] {
	b.engine = engine
	return b
}

// WithSpec sets the component specification.
func (b EventDrivenBuilder[S, T, R]) WithSpec(spec S) EventDrivenBuilder[S, T, R] {
	b.spec = spec
	return b
}

// WithResources sets the component's shared-resource references.
func (b EventDrivenBuilder[S, T, R]) WithResources(resources R) EventDrivenBuilder[S, T, R] {
	b.resources = resources
	return b
}

// WithProcessor sets the event processor.
func (b EventDrivenBuilder[S, T, R]) WithProcessor(
	processor EventProcessor[S, T, R],
) EventDrivenBuilder[S, T, R] {
	b.processor = processor
	return b
}

// Build creates the EventDrivenComponent with the given name.
func (b EventDrivenBuilder[S, T, R]) Build(name string) *EventDrivenComponent[S, T, R] {
	naming.MustBeValid(name)
	validateForCheckpoint[S, T](name, b.spec)

	comp := &EventDrivenComponent[S, T, R]{
		PortOwnerBase: messaging.NewPortOwnerBase(),
		engine:        b.engine,
		name:          name,
		spec:          b.spec,
		resources:     b.resources,
		processor:     b.processor,
		pendingWakeup: math.MaxUint64,
	}

	if registrar, ok := b.engine.(timing.HandlerRegistrar); ok {
		registrar.RegisterHandler(name, comp)
	}

	return comp
}
