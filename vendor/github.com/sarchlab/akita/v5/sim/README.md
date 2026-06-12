# sim

Package `sim` provides the core types and simulation kernel for Akita's
discrete-event simulation framework.

## Key Types

### Time and Frequency

- **`VTimeInSec`** — `type VTimeInSec uint64`. Simulated time in
  **picoseconds**. Despite the legacy name, the unit is picoseconds.
- **`Freq`** — `type Freq uint64`. Frequency in **Hz**. Constants `Hz`, `KHz`,
  `MHz`, `GHz` are provided.
  - `Freq.Period()` → tick interval in picoseconds.
  - `Freq.NextTick(now)` → next tick time after `now`.
  - `Freq.NCyclesLater(n, now)` → time `n` cycles after the current tick.

### Messages

- **`Msg`** — interface for all inter-component messages. Must implement
  `Meta() *MsgMeta`.
- **`MsgMeta`** — routing and identification metadata:
  - `ID uint64` — unique message ID (from `GetIDGenerator().Generate()`).
  - `Src, Dst RemotePort` — source and destination port names.
  - `RspTo uint64` — ID of the request this message responds to (0 = not a
    response).
  - `TrafficBytes int` — payload size for bandwidth modeling.

### Events

- **`Event`** — interface with `Time() VTimeInSec`, `HandlerID() string`, and
  `IsSecondary() bool`.
- **`EventBase`** — embeddable base struct implementing `Event`.
- **`Handler`** — processes events via `Handle(e Event) error`.

### Engine

- **`Engine`** — drives the simulation by dispatching events in time order.
  Satisfies `EventScheduler`, `TimeTeller`, `Hookable`, and
  `HandlerRegistrar`.
- **`NewSerialEngine()`** — single-threaded, deterministic engine.
- **`NewParallelEngine()`** — multi-goroutine engine for parallel execution.

Handlers are registered by name via `engine.RegisterHandler(name, handler)`.
Events reference handlers by `HandlerID()` string.

### Ports and Connections

- **`Port`** — bidirectional message endpoint owned by a `Component`. Has
  incoming and outgoing buffers with configurable capacity.
- **`NewPort(comp, inBufCap, outBufCap, name)`** — creates a default port.
- **`Connection`** — delivers messages between ports. Call `PlugIn(port)` to
  attach ports.
- **`RemotePort`** — `type RemotePort string`, a port name used in `MsgMeta`
  for addressing.

### Components

- **`Component`** — unifying interface for simulation elements. Owns ports and
  receives notifications via `NotifyRecv(port)` and `NotifyPortFree(port)`.
- **`ComponentBase`** — embeddable base struct with name, ports, hooks, and a
  mutex.
- **`TickingComponent`** — a component that processes state in fixed-frequency
  ticks. Uses a `Ticker` to implement `Handle(Event)`.

### ID Generation

- **`GetIDGenerator()`** — returns the global `IDGenerator`.
- **`IDGenerator.Generate() uint64`** — produces unique uint64 IDs.
- Sequential (deterministic) by default; call `UseParallelIDGenerator()` before
  first use for non-deterministic parallel operation.

### Hooks

- **`Hook`** — observer callback via `Func(ctx HookCtx)`.
- **`Hookable`** — any object that accepts hooks (`AcceptHook`, `Hooks`).
- **`HookPos`** — named positions like `HookPosBeforeEvent`,
  `HookPosAfterEvent`, `HookPosPortMsgSend`, `HookPosPortMsgRecvd`.

### Middleware

- **`Middleware`** — interface with `Tick() bool`. Used by `MiddlewareHolder`
  to compose tick-based processing pipelines.

## Usage Example

```go
package main

import "github.com/sarchlab/akita/v5/sim"

func main() {
    engine := sim.NewSerialEngine()

    // Register a handler
    engine.RegisterHandler("myHandler", &myHandler{})

    // Schedule an event at time 1000 ps
    evt := sim.NewEventBase(1000, "myHandler")
    engine.Schedule(evt)

    // Run the simulation
    engine.Run()
}

type myHandler struct{}

func (h *myHandler) Handle(e sim.Event) error {
    // process event at e.Time()
    return nil
}
```

## Sentinel Values

- Empty ID: `0` (not `""`)
- `MsgMeta.RspTo == 0` means the message is not a response.
