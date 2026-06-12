package modeling

import (
	"github.com/sarchlab/akita/v5/sim"
)

// Component is a generic component that combines Spec, State, Ports, and
// Middlewares.
//
// S is the Spec type (immutable configuration).
// T is the State type (mutable runtime data).
//
// Component uses in-place state update: 'current' and 'next' refer to the
// same state value. During [Tick], current is assigned to next before the
// middleware pipeline runs; after the pipeline completes, next is assigned
// back to current. Because both point to the same value, middlewares can
// read from [GetState] or [GetNextState] interchangeably.
//
// Component embeds [sim.TickingComponent] for tick-based lifecycle management
// and [sim.MiddlewareHolder] for the middleware pipeline.
type Component[S any, T any] struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	spec    S
	current T
	next    T
}

// GetSpec returns the immutable specification.
func (c *Component[S, T]) GetSpec() S {
	return c.spec
}

// GetState returns the current state. With in-place update semantics,
// this is the same underlying data that [GetNextState] points to.
func (c *Component[S, T]) GetState() T {
	return c.current
}

// GetNextState returns a pointer to the next state, allowing direct
// mutation. With in-place update semantics, this modifies the same
// underlying data visible through [GetState] after assignment.
func (c *Component[S, T]) GetNextState() *T {
	return &c.next
}

// SetNextState sets the next state directly.
func (c *Component[S, T]) SetNextState(state T) {
	c.next = state
}

// SetState sets both current and next to the given state. This is used for
// initialization and save/load scenarios where both must agree.
func (c *Component[S, T]) SetState(state T) {
	c.current = state
	c.next = state
}

// Tick performs the in-place state update cycle:
//  1. Assign current to next (shallow copy).
//  2. Run the middleware pipeline (which may modify next via
//     GetNextState/SetNextState or SetState).
//  3. Assign next back to current.
func (c *Component[S, T]) Tick() bool {
	c.next = c.current
	madeProgress := c.MiddlewareHolder.Tick()
	c.current = c.next

	return madeProgress
}

// CommitTick promotes next to current. This is used by components that
// implement their own Tick method with a custom update strategy.
func (c *Component[S, T]) CommitTick() {
	c.current = c.next
}

// ResetTick resets the TickScheduler so that future TickLater calls can
// schedule new events. This is used after loading state from a checkpoint.
func (c *Component[S, T]) ResetTick() {
	c.TickScheduler.Reset()
}

// ResetAndRestartTick resets the TickScheduler and schedules a new tick.
// This is used after loading state from a checkpoint when the component
// needs to immediately resume ticking.
func (c *Component[S, T]) ResetAndRestartTick() {
	c.TickScheduler.Reset()
	c.TickLater()
}
