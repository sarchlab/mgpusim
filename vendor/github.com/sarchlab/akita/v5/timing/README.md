# timing — Discrete-Event Simulation Core

Package `timing` provides simulation time, events, and event engines for the
Akita simulation framework. It is the discrete-event kernel that every other
package builds on: an engine holds a priority queue of events, advances virtual
time, and dispatches each event to the handler that owns it.

## Key Concepts

- **Virtual time** is represented by `VTimeInPicoSec`, a `uint64` counting
  picoseconds of simulated time. The engine always moves time forward.
- **Events** describe something that will happen at a future time. Each event
  is bound to exactly one handler.
- **Handlers** define a domain for events. An event may only be scheduled by,
  and may only directly modify, its handler.
- The **engine** repeatedly pops the earliest event, sets the current time to
  that event's time, and calls the handler to process it, until no events
  remain.

## Key Types

### Event and EventBase

```go
type Event interface {
    Time() VTimeInPicoSec   // when the event happens
    HandlerID() string  // name of the handler that processes it
    IsSecondary() bool  // secondary events run after same-time primary events
}
```

Embed `EventBase` to get the standard fields and getters. `MakeEventBase(t, handlerID)`
returns an `EventBase` value with a fresh ID from the global ID generator:

```go
type tickEvent struct {
    timing.EventBase
}

evt := tickEvent{timing.MakeEventBase(now, comp.Name())}
```

### Handler

```go
type Handler interface {
    Handle(e Event) error
}
```

### Engine, EventScheduler, TimeTeller

```go
type TimeTeller interface {
    CurrentTime() VTimeInPicoSec
}

type EventScheduler interface {
    TimeTeller
    Schedule(e Event)
}

type Engine interface {
    hooking.Hookable
    EventScheduler
    Run() error
    Pause()
    Continue()
}
```

`SerialEngine` runs events strictly one after another and is deterministic.
`ParallelEngine` runs same-time, non-conflicting events across goroutines.
Both implement `Engine`, register handlers by name via `RegisterHandler`, and
keep a separate secondary queue for `IsSecondary()` events.

```go
engine := timing.NewSerialEngine()
engine.RegisterHandler(comp.Name(), comp)
engine.Schedule(evt)
err := engine.Run()
```

## Frequency

`Freq` expresses a clock rate in Hz, with the units `Hz`, `KHz`, `MHz`, and
`GHz`. It converts between time and cycles and snaps times onto tick
boundaries.

```go
freq := 1 * timing.GHz
period := freq.Period()           // picoseconds between ticks
next := freq.NextTick(now)        // next tick strictly after now
this := freq.ThisTick(now)        // now rounded up to a tick boundary
later := freq.NCyclesLater(3, now)
```

## ID Generation

`GetIDGenerator().Generate()` returns unique `uint64` IDs. By default IDs are
sequential and deterministic; call `UseParallelIDGenerator()` before first use
for faster but non-deterministic IDs (or `UseSequentialIDGenerator()` to be
explicit). The generator's counter is part of the simulation state snapshot.

## Hooks

Engines are `hooking.Hookable`. They fire `HookPosBeforeEvent` and
`HookPosAfterEvent` around each event, with the event passed as `HookCtx.Item`.
Attach a hook with `engine.AcceptHook(myHook)` to observe or trace execution.
