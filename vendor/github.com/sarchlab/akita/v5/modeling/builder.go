package modeling

import "github.com/sarchlab/akita/v5/sim"

// Builder constructs [Component] instances.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
type Builder[S any, T any] struct {
	engine sim.EventScheduler
	freq   sim.Freq
	spec   S
}

// NewBuilder creates a new Builder.
func NewBuilder[S any, T any]() Builder[S, T] {
	return Builder[S, T]{}
}

// WithEngine sets the simulation engine.
func (b Builder[S, T]) WithEngine(engine sim.EventScheduler) Builder[S, T] {
	b.engine = engine
	return b
}

// WithFreq sets the component frequency.
func (b Builder[S, T]) WithFreq(freq sim.Freq) Builder[S, T] {
	b.freq = freq
	return b
}

// WithSpec sets the component specification.
func (b Builder[S, T]) WithSpec(spec S) Builder[S, T] {
	b.spec = spec
	return b
}

// Build creates the Component with the given name.
func (b Builder[S, T]) Build(name string) *Component[S, T] {
	comp := &Component[S, T]{
		spec: b.spec,
	}
	comp.TickingComponent = sim.NewTickingComponent(
		name, b.engine, b.freq, comp)

	return comp
}
