package sim

import (
	"sync"
)

// TickEvent is a generic event that almost all the component can use to
// update their status.
type TickEvent struct {
	EventBase
}

// MakeTickEvent creates a new TickEvent
func MakeTickEvent(handlerID string, time VTimeInSec) TickEvent {
	evt := TickEvent{}
	evt.ID = GetIDGenerator().Generate()
	evt.HandlerID_ = handlerID
	evt.Time_ = time
	evt.Secondary = false

	return evt
}

// A Ticker is an object that updates states with ticks.
type Ticker interface {
	Tick() bool
}

// TickScheduler can help schedule tick events.
type TickScheduler struct {
	lock      sync.Mutex
	handlerID string
	Freq      Freq
	Engine    EventScheduler
	secondary bool

	nextTickTime     VTimeInSec
	hasScheduledTick bool
}

// NewTickScheduler creates a scheduler for tick events.
func NewTickScheduler(
	handlerID string,
	engine EventScheduler,
	freq Freq,
) *TickScheduler {
	ticker := new(TickScheduler)

	ticker.handlerID = handlerID
	ticker.Engine = engine
	ticker.Freq = freq
	ticker.hasScheduledTick = false

	return ticker
}

// NewSecondaryTickScheduler creates a scheduler that always schedule secondary
// tick events.
func NewSecondaryTickScheduler(
	handlerID string,
	engine EventScheduler,
	freq Freq,
) *TickScheduler {
	ticker := new(TickScheduler)

	ticker.handlerID = handlerID
	ticker.Engine = engine
	ticker.Freq = freq
	ticker.secondary = true
	ticker.hasScheduledTick = false

	return ticker
}

// TickNow schedule a Tick event at the current time.
func (t *TickScheduler) TickNow() {
	t.lock.Lock()
	time := t.CurrentTime()

	if t.hasScheduledTick && t.nextTickTime >= time {
		t.lock.Unlock()
		return
	}

	t.nextTickTime = t.Freq.ThisTick(time)
	t.hasScheduledTick = true
	tick := MakeTickEvent(t.handlerID, t.nextTickTime)

	if t.secondary {
		tick.Secondary = true
	}

	t.Engine.Schedule(tick)
	t.lock.Unlock()
}

// TickLater will schedule a tick event at the cycle after the now time.
func (t *TickScheduler) TickLater() {
	t.lock.Lock()
	time := t.Freq.NextTick(t.CurrentTime())

	if t.hasScheduledTick && t.nextTickTime >= time {
		t.lock.Unlock()
		return
	}

	t.nextTickTime = time
	t.hasScheduledTick = true
	tick := MakeTickEvent(t.handlerID, t.nextTickTime)

	if t.secondary {
		tick.Secondary = true
	}

	t.Engine.Schedule(tick)
	t.lock.Unlock()
}

// Reset resets the TickScheduler so that TickLater can schedule new events
// after a restore.
func (t *TickScheduler) Reset() {
	t.hasScheduledTick = false
}

func (t *TickScheduler) CurrentTime() VTimeInSec {
	return t.Engine.CurrentTime()
}

// TickingComponent is a type of component that update states from cycle to
// cycle. A programmer would only need to program a tick function for a ticking
// component.
type TickingComponent struct {
	*ComponentBase
	*TickScheduler

	ticker Ticker
}

// NotifyPortFree triggers the TickingComponent to start ticking again.
func (c *TickingComponent) NotifyPortFree(
	_ Port,
) {
	c.TickLater()
}

// NotifyRecv triggers the TickingComponent to start ticking again.
func (c *TickingComponent) NotifyRecv(
	_ Port,
) {
	c.TickLater()
}

// SetTicker replaces the Ticker used by Handle. This allows wrapper types
// (e.g. endpoint.Comp) to override the Tick method after construction.
func (c *TickingComponent) SetTicker(t Ticker) {
	c.ticker = t
}

// Handle triggers the tick function of the TickingComponent
func (c *TickingComponent) Handle(e Event) error {
	// Clear the dedup guard so that TickNow/TickLater calls arriving
	// while Tick() executes (e.g. via NotifyRecv, NotifySend) can
	// schedule a fresh event.  Without this, a component whose
	// previous tick returned false (no-progress) at time T blocks
	// all TickNow(T) attempts because the guard still records T.
	c.TickScheduler.Reset()

	madeProgress := c.ticker.Tick()
	if madeProgress {
		c.TickLater()
	}

	return nil
}

// NewTickingComponent creates a new ticking component
func NewTickingComponent(
	name string,
	engine EventScheduler,
	freq Freq,
	ticker Ticker,
) *TickingComponent {
	tc := new(TickingComponent)
	tc.TickScheduler = NewTickScheduler(name, engine, freq)
	tc.ComponentBase = NewComponentBase(name)
	tc.ticker = ticker

	if registrar, ok := engine.(HandlerRegistrar); ok {
		registrar.RegisterHandler(name, tc)
	}

	return tc
}

// NewSecondaryTickingComponent creates a new ticking component
func NewSecondaryTickingComponent(
	name string,
	engine EventScheduler,
	freq Freq,
	ticker Ticker,
) *TickingComponent {
	tc := new(TickingComponent)
	tc.TickScheduler = NewSecondaryTickScheduler(name, engine, freq)
	tc.ComponentBase = NewComponentBase(name)
	tc.ticker = ticker

	if registrar, ok := engine.(HandlerRegistrar); ok {
		registrar.RegisterHandler(name, tc)
	}

	return tc
}
