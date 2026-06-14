package modeling

import (
	"sync"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// TickEvent is a generic event that almost all the component can use to
// update their status.
type TickEvent struct {
	timing.EventBase
}

// MakeTickEvent creates a new TickEvent
func MakeTickEvent(handlerID string, time timing.VTimeInPicoSec) TickEvent {
	evt := TickEvent{}
	evt.ID = timing.GetIDGenerator().Generate()
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
	freq      timing.Freq
	engine    timing.EventScheduler
	secondary bool

	nextTickTime     timing.VTimeInPicoSec
	hasScheduledTick bool
}

// NewTickScheduler creates a scheduler for tick events.
func NewTickScheduler(
	handlerID string,
	engine timing.EventScheduler,
	freq timing.Freq,
) *TickScheduler {
	ticker := new(TickScheduler)

	ticker.handlerID = handlerID
	ticker.engine = engine
	ticker.freq = freq
	ticker.hasScheduledTick = false

	return ticker
}

// NewSecondaryTickScheduler creates a scheduler that always schedule secondary
// tick events.
func NewSecondaryTickScheduler(
	handlerID string,
	engine timing.EventScheduler,
	freq timing.Freq,
) *TickScheduler {
	ticker := new(TickScheduler)

	ticker.handlerID = handlerID
	ticker.engine = engine
	ticker.freq = freq
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

	t.nextTickTime = t.freq.ThisTick(time)
	t.hasScheduledTick = true
	tick := MakeTickEvent(t.handlerID, t.nextTickTime)

	if t.secondary {
		tick.Secondary = true
	}

	t.engine.Schedule(tick)
	t.lock.Unlock()
}

// TickLater will schedule a tick event at the cycle after the now time.
func (t *TickScheduler) TickLater() {
	t.lock.Lock()
	time := t.freq.NextTick(t.CurrentTime())

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

	t.engine.Schedule(tick)
	t.lock.Unlock()
}

func (t *TickScheduler) CurrentTime() timing.VTimeInPicoSec {
	return t.engine.CurrentTime()
}

// snapshot returns the scheduler's dedup guard: whether a tick is pending and at
// what time. It is serialized in the component checkpoint so the guard can be
// restored directly, with no post-load reconciliation against the event queue.
func (t *TickScheduler) snapshot() (nextTickTime timing.VTimeInPicoSec, scheduled bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	return t.nextTickTime, t.hasScheduledTick
}

// restore sets the scheduler's dedup guard from a checkpoint. The matching tick
// event is restored separately by the engine, so the two stay consistent.
func (t *TickScheduler) restore(nextTickTime timing.VTimeInPicoSec, scheduled bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.nextTickTime = nextTickTime
	t.hasScheduledTick = scheduled
}

// TickingComponent is a type of component that update states from cycle to
// cycle. A programmer would only need to program a tick function for a ticking
// component.
type TickingComponent struct {
	sync.Mutex
	hooking.HookableBase
	*messaging.PortOwnerBase
	*TickScheduler

	name   string
	ticker Ticker
}

// Name returns the component name.
func (c *TickingComponent) Name() string {
	return c.name
}

// NotifyPortFree triggers the TickingComponent to start ticking again.
func (c *TickingComponent) NotifyPortFree(
	_ messaging.Port,
) {
	c.TickLater()
}

// NotifyRecv triggers the TickingComponent to start ticking again.
func (c *TickingComponent) NotifyRecv(
	_ messaging.Port,
) {
	c.TickLater()
}

// Handle triggers the tick function of the TickingComponent
func (c *TickingComponent) Handle(e timing.Event) error {
	madeProgress := c.ticker.Tick()
	if madeProgress {
		c.TickLater()
	}

	return nil
}

// NewTickingComponent creates a new ticking component
func NewTickingComponent(
	name string,
	engine timing.EventScheduler,
	freq timing.Freq,
	ticker Ticker,
) *TickingComponent {
	naming.MustBeValid(name)

	tc := new(TickingComponent)
	tc.TickScheduler = NewTickScheduler(name, engine, freq)
	tc.PortOwnerBase = messaging.NewPortOwnerBase()
	tc.name = name
	tc.ticker = ticker

	if registrar, ok := engine.(timing.HandlerRegistrar); ok {
		registrar.RegisterHandler(name, tc)
	}

	return tc
}

// NewSecondaryTickingComponent creates a new ticking component
func NewSecondaryTickingComponent(
	name string,
	engine timing.EventScheduler,
	freq timing.Freq,
	ticker Ticker,
) *TickingComponent {
	naming.MustBeValid(name)

	tc := new(TickingComponent)
	tc.TickScheduler = NewSecondaryTickScheduler(name, engine, freq)
	tc.PortOwnerBase = messaging.NewPortOwnerBase()
	tc.name = name
	tc.ticker = ticker

	if registrar, ok := engine.(timing.HandlerRegistrar); ok {
		registrar.RegisterHandler(name, tc)
	}

	return tc
}
