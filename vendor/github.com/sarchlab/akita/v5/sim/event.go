package sim

// VTimeInSec defines the time in the simulated space in the unit of picosecond
type VTimeInSec uint64

// An Event is something going to happen in the future.
type Event interface {
	// Return the time that the event should happen
	Time() VTimeInSec

	// Returns the handler ID that can handle the event
	HandlerID() string

	// IsSecondary tells if the event is a secondary event. Secondary event are
	// handled after all same-time primary events are handled.
	IsSecondary() bool
}

// EventBase provides the basic fields and getters for other events
type EventBase struct {
	ID         uint64     `json:"id"`
	Time_      VTimeInSec `json:"time"`
	HandlerID_ string     `json:"handler_id"`
	Secondary  bool       `json:"secondary"`
}

// NewEventBase creates a new EventBase
func NewEventBase(t VTimeInSec, handlerID string) *EventBase {
	e := new(EventBase)
	e.ID = GetIDGenerator().Generate()
	e.Time_ = t
	e.HandlerID_ = handlerID
	e.Secondary = false

	return e
}

// Time return the time that the event is going to happen
func (e EventBase) Time() VTimeInSec {
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

// EventID returns the unique sequential ID of the event, used for
// deterministic tie-breaking when two events share the same timestamp.
func (e EventBase) EventID() uint64 {
	return e.ID
}

// A Handler defines a domain for the events.
//
// One event is always constraint to one Handler, which means the event can
// only be scheduled by one handler and can only directly modify that handler.
type Handler interface {
	Handle(e Event) error
}
