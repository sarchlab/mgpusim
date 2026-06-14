# idealmemcontroller — Ideal Memory Controller

Package `idealmemcontroller` provides a simplified memory controller for the
Akita simulation framework. Every request completes after a fixed number of
cycles with no bandwidth or concurrency limitations, making it a good fit for
testing and for simulations where memory-timing detail is not important.

## How It Works

The controller processes `mem.ReadReq` and `mem.WriteReq` messages on its `Top`
port. Each request is assigned a countdown timer equal to the configured
`Latency` (in cycles). Every tick, all inflight countdowns decrement by one.
When a countdown reaches zero, the controller reads from or writes to its
backing `mem.Storage` and sends the response.

There is no limit on the number of concurrent inflight transactions — all
requests proceed in parallel. At most `Width` new requests are accepted per
tick.

Two middlewares run each tick:

| Middleware | Role |
|---|---|
| **ctrlMiddleware** | Handles `mem.ControlReq` on the `Control` port (enable, pause, drain). |
| **memMiddleware** | Accepts new read/write requests (up to `Width` per tick), decrements countdowns, performs storage I/O, and sends responses. |

### Controller States

| State | Behavior |
|---|---|
| `"enable"` | Normal operation — accepts new requests and processes countdowns. |
| `"pause"` | Stops accepting new requests and freezes all countdowns. |
| `"drain"` | Stops accepting new requests but continues processing inflight transactions. Sends `mem.ControlRsp` when all are complete, then transitions to `"pause"`. |

## Key Types

- `Spec` — immutable configuration: frequency, latency, width, cache-line size,
  and capacity.
- `State` — mutable runtime data: the list of inflight transactions and the
  current control state (`"enable"`, `"pause"`, or `"drain"`).
- `Resources` — shared wiring; holds the backing `*mem.Storage`.
- `Comp` — `modeling.Component[Spec, State, Resources]`.

```go
type Spec struct {
    Freq          timing.Freq // Operating frequency
    Width         int         // Max new requests accepted per tick
    Latency       int         // Fixed response latency in cycles
    CacheLineSize int         // Access granularity in bytes
    Capacity      uint64      // Backing-storage size when built internally
    StorageRef    string      // Storage resource name (set by Build)
}
```

The controller uses **global storage**: a request's address indexes the backing
store directly, with no per-controller address conversion. When several
controllers are interleaved over one shared `mem.Storage`, the sender's
destination map selects the controller; each controller simply reads and writes
the shared store at the request's global address.

## Builder Pattern

Start from `DefaultSpec()`, tweak the fields you need, and pass the whole spec
to `WithSpec`. Wiring comes from `WithRegistrar` (which provides the engine and
registers the component) and `WithResources` (the shared backing storage). When
`WithResources` is omitted, the controller builds its own storage sized by
`Spec.Capacity`. `Build` declares the `Top` and `Control` ports but does not
create their instances. Build each port with `modeling.MakePortBuilder` (which
registers the port with the simulation) and attach it with `AssignPort`,
choosing the buffer size.

```go
spec := idealmemcontroller.DefaultSpec()
spec.Latency = 50

ctrl := idealmemcontroller.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(idealmemcontroller.Resources{Storage: storage}).
    Build("IdealMem")

topPort := modeling.MakePortBuilder().
    WithRegistrar(sim).
    WithComponent(ctrl).
    WithSpec(modeling.PortSpec{BufSize: 16}).
    Build("Top")
ctrl.AssignPort("Top", topPort)

ctrlPort := modeling.MakePortBuilder().
    WithRegistrar(sim).
    WithComponent(ctrl).
    WithSpec(modeling.PortSpec{BufSize: 16}).
    Build("Control")
ctrl.AssignPort("Control", ctrlPort)

topPort = ctrl.GetPortByName("Top")
```

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{Storage: s})` | Shared backing storage (built internally if omitted) |

### Default Configuration

| Parameter | Default |
|---|---|
| Frequency | 1 GHz |
| Latency | 100 cycles |
| Width | 1 request/tick |
| Cache line size | 64 bytes |
| Storage capacity | 4 GB |

## Ports

- **Top**: accepts `mem.ReadReq` and `mem.WriteReq`, returns `mem.DataReadyRsp`
  and `mem.WriteDoneRsp`.
- **Control**: accepts `mem.ControlReq` (enable / pause / drain), returns
  `mem.ControlRsp`.
