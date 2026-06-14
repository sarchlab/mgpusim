package modeling

import (
	"github.com/sarchlab/akita/v5/timing"
)

// Builder constructs [Component] instances.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
// R is the Resources type (references to shared resources; None when unused).
type Builder[S any, T any, R any] struct {
	engine    timing.EventScheduler
	freq      timing.Freq
	spec      S
	resources R
}

// NewBuilder creates a new Builder.
func NewBuilder[S any, T any, R any]() Builder[S, T, R] {
	return Builder[S, T, R]{}
}

// WithEngine sets the simulation engine.
func (b Builder[S, T, R]) WithEngine(engine timing.EventScheduler) Builder[S, T, R] {
	b.engine = engine
	return b
}

// WithFreq sets the component frequency.
func (b Builder[S, T, R]) WithFreq(freq timing.Freq) Builder[S, T, R] {
	b.freq = freq
	return b
}

// WithSpec sets the component specification.
func (b Builder[S, T, R]) WithSpec(spec S) Builder[S, T, R] {
	b.spec = spec
	return b
}

// WithResources sets the component's shared-resource references.
func (b Builder[S, T, R]) WithResources(resources R) Builder[S, T, R] {
	b.resources = resources
	return b
}

// Build creates the Component with the given name.
func (b Builder[S, T, R]) Build(name string) *Component[S, T, R] {
	validateForCheckpoint[S, T](name, b.spec)

	comp := &Component[S, T, R]{
		spec:      b.spec,
		resources: b.resources,
	}
	comp.TickingComponent = NewTickingComponent(
		name, b.engine, b.freq, comp)

	return comp
}
