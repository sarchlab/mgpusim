# idealmemcontroller — Ideal Memory Controller

Package `idealmemcontroller` provides a simplified memory controller for the
Akita simulation framework. Every request completes after a fixed number of
cycles with no bandwidth or concurrency limitations, making it ideal for
testing and for simulations where memory timing detail is not important.

## How It Works

The controller processes `mem.ReadReq` and `mem.WriteReq` messages on its top
port. Each request is assigned a countdown timer equal to the configured
`Latency` (in cycles). Every tick, all inflight countdowns decrement by one.
When a countdown reaches zero, the controller reads from or writes to its
backing `mem.Storage` and sends the response.

There is no limit on the number of concurrent inflight transactions — all
requests proceed in parallel.

## Architecture

Two middlewares run each tick:

| Middleware | Role |
|---|---|
| **ctrlMiddleware** | Handles `mem.ControlReq` on the control port (enable, pause, drain). |
| **memMiddleware** | Accepts new read/write requests (up to `Width` per tick), decrements countdowns, performs storage I/O, and sends responses. |

### Controller States

| State | Behavior |
|---|---|
| `"enable"` | Normal operation — accepts new requests and processes countdowns. |
| `"pause"` | Stops accepting new requests and freezes all countdowns. |
| `"drain"` | Stops accepting new requests but continues processing inflight transactions. Sends `mem.ControlRsp` when all are complete, then transitions to `"pause"`. |

## Key Types

### Spec (immutable configuration)

```go
type Spec struct {
    Freq          sim.Freq  // Operating frequency (Hz)
    Latency       int       // Fixed response latency in cycles
    Width         int       // Max new requests accepted per tick
    CacheLineSize int       // Access granularity in bytes
    StorageRef    string    // Storage identifier (set to component name)
}
```

### State (mutable runtime data)

```go
type State struct {
    InflightTransactions []inflightTransaction  // Active requests with countdowns
    CurrentState         string                 // "enable", "pause", or "drain"
}
```

### Comp (component wrapper)

```go
type Comp struct {
    *modeling.Component[Spec, State]
}

func (c *Comp) GetStorage() *mem.Storage   // Access backing storage
```

`Comp` implements the `mem.StorageOwner` interface via `StorageName()`.

## Builder Pattern

```go
ctrl := idealmemcontroller.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    WithTopPort(topPort).
    WithCtrlPort(ctrlPort).
    Build("IdealMem")
```

### Builder Options

| Method | Description |
|---|---|
| `WithEngine(e)` | Event scheduler (required) |
| `WithFreq(f)` | Operating frequency |
| `WithSpec(s)` | Override all spec fields at once |
| `WithNewStorage(n)` | Create a new backing store of `n` bytes (default 4 GB) |
| `WithStorage(s)` | Use an existing `*mem.Storage` |
| `WithTopPort(p)` | Port for read/write requests |
| `WithCtrlPort(p)` | Port for control commands |
| `WithTopBufSize(n)` | Top port buffer capacity |
| `WithAddressConverter(c)` | Address translation for interleaved multi-controller setups |

### Default Configuration

| Parameter | Default |
|---|---|
| Frequency | 1 GHz |
| Latency | 100 cycles |
| Width | 1 request/tick |
| Cache line size | 64 bytes |
| Storage capacity | 4 GB |

## Protocol

- **Top port**: accepts `mem.ReadReq` and `mem.WriteReq`, returns
  `mem.DataReadyRsp` and `mem.WriteDoneRsp`
- **Control port**: accepts `mem.ControlReq` (enable/pause/drain), returns
  `mem.ControlRsp`
