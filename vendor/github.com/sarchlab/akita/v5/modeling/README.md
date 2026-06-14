# modeling — Generic Component Framework

Package `modeling` provides the application-level component framework for the
Akita simulation framework. It builds on the `timing` and `messaging` packages
to offer generic, type-safe components with a structured Spec / State / Resources
separation and a middleware pipeline.

## Key Concepts

Every modeled component is parameterized by three type arguments:

- **Spec (`S`)** — immutable configuration set at build time (e.g., cache size,
  number of banks). Must be a plain struct with primitive fields only.
- **State (`T`)** — mutable runtime data (e.g., queues, counters, in-flight
  tables). May contain nested structs, slices, and maps; must be
  JSON-serializable.
- **Resources (`R`)** — references to shared objects (e.g., backing storage).
  Use `modeling.None` when a component references no shared resources.

Validate values at runtime with `ValidateSpec(v)` and `ValidateState(v)`. Both
reject pointers, interfaces, channels, and functions. `ValidateSpec` additionally
rejects nested structs; `ValidateState` allows them. Map keys must be `string` or
an integer type.

## Key Types

### Component[S, T, R] (tick-driven)

A fixed-frequency component that processes state each tick via a middleware
pipeline.

```go
type MySpec struct {
    Size int
}

type MyState struct {
    Count int
}

comp := modeling.NewBuilder[MySpec, MyState, modeling.None]().
    WithEngine(engine).
    WithFreq(1 * timing.GHz).
    WithSpec(MySpec{Size: 64}).
    Build("MyComponent")

comp.AddMiddleware(&myMiddleware{comp: comp})
```

- `Spec() S` — read-only accessor for the immutable spec (returns a copy).
- `Resources() R` — read-only accessor for the shared-resource references.
- `State` — a plain exported field of type `T`; middleware mutates it in place.
- `Tick() bool` — runs the middleware pipeline (returns true if progress made).

### EventDrivenComponent[S, T, R]

A component that wakes on events rather than ticking at a fixed frequency.

```go
comp := modeling.NewEventDrivenBuilder[MySpec, MyState, modeling.None]().
    WithEngine(engine).
    WithSpec(MySpec{Size: 64}).
    WithProcessor(&myProcessor{}).
    Build("MyEDComponent")
```

The `EventProcessor[S, T, R]` interface has a single method:

```go
Process(comp *EventDrivenComponent[S, T, R], now timing.VTimeInPicoSec) bool
```

Wakeups are scheduled via:

- `comp.ScheduleWakeAt(t)` — schedule at a specific time (with dedup guard).
- `comp.ScheduleWakeNow()` — schedule at the current engine time.

Port notifications (`NotifyRecv`, `NotifyPortFree`) automatically schedule
wakeups.

### Domain

A named bundle of components that exposes selected internal ports at its
boundary. Domains nest — components form a domain (e.g., a shader array),
and domains compose into larger domains (e.g., a GPU) — with hierarchical
names following the `Domain.Domain.Component` convention.

```go
gpu := modeling.NewDomain("GPU[0]")
gpu.DeclarePort("Top")
gpu.AssignPort("Top", commandProcessor.GetPortByName("ToDriver"))

// Outside code addresses the domain, not its internals.
port := gpu.GetPortByName("Top")
```

## Builder Pattern

| Builder | Creates | Key Settings |
|---|---|---|
| `NewBuilder[S, T, R]()` | `*Component[S, T, R]` | `WithEngine`, `WithFreq`, `WithSpec`, `WithResources` |
| `NewEventDrivenBuilder[S, T, R]()` | `*EventDrivenComponent[S, T, R]` | `WithEngine`, `WithSpec`, `WithResources`, `WithProcessor` |

Both builders register the component as an event handler when the engine
implements `timing.HandlerRegistrar`.
