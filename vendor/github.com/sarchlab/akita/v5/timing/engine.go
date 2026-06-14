package timing

import "github.com/sarchlab/akita/v5/hooking"

// TimeTeller can be used to get the current time.
type TimeTeller interface {
	CurrentTime() VTimeInPicoSec
}

// EventScheduler can be used to schedule future events.
type EventScheduler interface {
	TimeTeller

	Schedule(e Event)
}

// HandlerRegistrar allows registering named handlers for event dispatch.
type HandlerRegistrar interface {
	RegisterHandler(name string, handler Handler)
}

// An Engine is a unit that keeps the discrete event simulation run.
type Engine interface {
	hooking.Hookable
	EventScheduler

	// Run will process all the events until the simulation finishes.
	Run() error

	// Pause will pause the simulation until continue is called.
	Pause()

	// Continue will continue the paused simulation.
	Continue()
}
