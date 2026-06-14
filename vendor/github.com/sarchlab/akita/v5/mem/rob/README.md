# rob — Reorder Buffer

Package `rob` provides a reorder-buffer component for the Akita simulation
framework. It restores in-order responses on top of a downstream memory unit
that may complete requests out of order. The reorder buffer forwards each
request from its `Top` port to a single bottom unit through its `Bottom` port,
tracks the resulting transactions in FIFO order, and releases responses to the
`Top` port strictly in the order the requests arrived.

## How It Works

A single `middleware` advances the buffer each tick, running three stages up to
`NumReqPerCycle` times. The control port is serviced first so a flush can
quiesce the pipeline before any new traffic moves.

```
Top ──► topDown ──► Bottom ──► (bottom unit) ──► Bottom ──► parseBottom
                                                                │
                          FIFO transaction list ◄──────────────┘
                                  │
                              bottomUp ──► Top   (released in arrival order)
```

1. **topDown** — Peeks a `mem.AccessReq` (a `mem.ReadReq` or `mem.WriteReq`) from
   `Top`, builds a fresh *shadow* request with a new ID, rewrites its `Dst` to
   the configured `BottomUnit`, sends it on `Bottom`, and appends a transaction
   to the FIFO list. Stalls when the list reaches `BufferSize`.
2. **parseBottom** — Matches each `mem.DataReadyRsp`/`mem.WriteDoneRsp` from
   `Bottom` to its transaction by `RspTo`, records the payload, and sets the
   transaction's `HasRsp` flag. Unmatched responses (e.g. left over after a
   flush) are dropped.
3. **bottomUp** — If the head-of-line transaction has its response, forwards a
   matching response to the original requester (`RspTo` set to the original
   request ID) and pops it. A completed transaction behind an incomplete head
   waits, which is what enforces in-order delivery.

## Key Types

```go
type Comp = modeling.Component[Spec, State, modeling.None]
```

- **Spec** — immutable config: `Freq`, `BufferSize` (max in-flight
  transactions), `NumReqPerCycle`, and the `BottomUnit` remote port every shadow
  request targets.
- **State** — mutable runtime: the FIFO `Transactions` list and an `IsFlushing`
  flag. Each `transactionState` remembers the original request's ID and source,
  the shadow request's ID, whether it is a read, and the buffered response data.

The reorder buffer references no shared resources, so it uses `modeling.None`
and exposes no `WithResources`.

## Builder Pattern

Configuration is supplied as a whole through `WithSpec` (start from
`DefaultSpec()`); the engine and registration come from `WithRegistrar`. `Build`
declares the component's `Top`, `Bottom`, and `Control` ports; the port
instances are built and attached externally after `Build` with `AssignPort`, so
the caller chooses the buffer sizes.

```go
spec := rob.DefaultSpec()
spec.BufferSize = 256
spec.BottomUnit = dramPort.AsRemote()

reorderBuffer := rob.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    Build("ROB")

for _, name := range []string{"Top", "Bottom", "Control"} {
    p := modeling.MakePortBuilder().
        WithRegistrar(sim).
        WithComponent(reorderBuffer).
        WithSpec(modeling.PortSpec{BufSize: 8}).
        Build(name)
    reorderBuffer.AssignPort(name, p)
}

topPort := reorderBuffer.GetPortByName("Top")
```

### Builder Methods

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required). |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()`. Set `BottomUnit` to the downstream port. |

## Ports

- **Top** — accepts `mem.ReadReq` and `mem.WriteReq`, returns
  `mem.DataReadyRsp` and `mem.WriteDoneRsp` in arrival order.
- **Bottom** — sends shadow `mem.ReadReq`/`mem.WriteReq` to the `BottomUnit` and
  receives `mem.DataReadyRsp`/`mem.WriteDoneRsp`.
- **Control** — accepts `mem.ControlReq` (`CmdFlush` drops in-flight
  transactions and quiesces the pipeline; `CmdEnable` drains stale port traffic
  and resumes), returns `mem.ControlRsp`.
