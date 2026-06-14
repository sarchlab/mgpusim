package timing

// VTimeInPicoSec defines the time in the simulated space in the unit of picosecond.
type VTimeInPicoSec uint64

// An Event is something going to happen in the future.
type Event interface {
	// Time returns the time that the event should happen.
	Time() VTimeInPicoSec

	// HandlerID returns the handler ID that can handle the event.
	HandlerID() string

	// IsSecondary tells if the event is a secondary event. Secondary events are
	// handled after all same-time primary events are handled.
	IsSecondary() bool
}

// EventBase provides the basic fields and getters for other events.
type EventBase struct {
	ID         uint64     `json:"id"`
	Time_      VTimeInPicoSec `json:"time"`
	HandlerID_ string     `json:"handler_id"`
	Secondary  bool       `json:"secondary"`
}

// MakeEventBase creates a new EventBase as a value.
func MakeEventBase(t VTimeInPicoSec, handlerID string) EventBase {
	return EventBase{
		ID:         GetIDGenerator().Generate(),
		Time_:      t,
		HandlerID_: handlerID,
		Secondary:  false,
	}
}

// Time returns the time that the event is going to happen.
func (e EventBase) Time() VTimeInPicoSec {
	return e.Time_
}

// HandlerID returns the handler ID to handle the event.
func (e EventBase) HandlerID() string {
	return e.HandlerID_
}

// IsSecondary returns true if the event is a secondary event.
func (e EventBase) IsSecondary() bool {
	return e.Secondary
}

// A Handler defines a domain for the events.
//
// One event is always constrained to one Handler, which means the event can
// only be scheduled by one handler and can only directly modify that handler.
type Handler interface {
	Handle(e Event) error
}
