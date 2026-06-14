# writethroughcache — Unified GPU Cache

Package `writethroughcache` provides a set-associative, banked, pipelined cache
component for the Akita simulation framework. Despite the name, it is a unified
GPU cache that supports three write policies selected by a single spec field:
`write-around` (default), `write-evict`, and `write-through`. It sits between an
upper-level requester (its `Top` port) and lower-level memory (its `Bottom`
port), serving hits locally and forwarding misses downstream.

## How It Works

Each incoming `mem.ReadReq`/`mem.WriteReq` becomes a `transactionState` that
flows through a multi-stage pipeline, driven once per tick by `pipelineMW`:

```
Top ──► intake ──► directory(+MSHR) ──► bank(s) ──► respond ──► Top
                          │                  ▲
                          └──► Bottom ──► bottomParser
```

1. **intake** — Accepts requests from `Top` (up to `NumReqPerCycle`/tick,
   capped by `MaxNumConcurrentTrans`), creates a transaction, and pushes it into
   the directory buffer.
2. **directory** — After a `DirLatency`-cycle pipeline, looks the line up in the
   shared `cache.DirectoryState` and `cache.MSHRState`. Read hits go to a bank;
   read misses allocate an MSHR entry and fetch from `Bottom` (coalescing
   duplicate misses). Writes dispatch to the active write policy.
3. **bank** — One or more banks apply a `BankLatency`-cycle pipeline, then read
   or write the backing `mem.Storage` and unlock the directory block.
4. **bottomParser** — Parses `mem.DataReadyRsp`/`mem.WriteDoneRsp` from `Bottom`,
   fills MSHR entries, and merges coalesced data.
5. **respond** — Returns `mem.DataReadyRsp`/`mem.WriteDoneRsp` to `Top` once a
   transaction's bank, fetch, and lower-memory dependencies are all satisfied.

A separate `controlMW` runs the **control stage**, which handles flush and
re-enable commands on the `Control` port (see [Ports](#ports)).

## Write Policies

| `WritePolicyType` | On a write hit | On a write miss |
|---|---|---|
| `write-around` | Write line locally and write through to `Bottom`. | Write straight through to `Bottom`; line is not allocated. |
| `write-evict` | Write through to `Bottom` and invalidate the local line. | Write through to `Bottom`; line is not allocated. |
| `write-through` | Write line locally and write through to `Bottom`. | Allocate a victim line; partial writes fetch-and-merge through `Bottom`, full-line writes install directly. |

## Key Types

```go
type Comp = modeling.Component[Spec, State, Resources]
```

- **Spec** — immutable config: `Freq`, `Log2BlockSize`, `WayAssociativity`,
  `NumSets` (derived from `TotalByteSize`), `NumBanks`, `NumMSHREntry`,
  `NumReqPerCycle`, `MaxNumConcurrentTrans`, `BankLatency`, `DirLatency`,
  `WritePolicyType`, and the address-mapper fields. (Port buffer sizes are no
  longer part of Spec — the caller chooses them when assigning each port.)
- **State** — mutable runtime: `cache.DirectoryState`, `cache.MSHRState`, the
  flat `Transactions` list, the directory/bank `queueing.Buffer`/`Pipeline`
  stages, the pause flag, and the in-progress flush request.
- **Resources** — shared wiring: the backing `*mem.Storage` plus the
  `AddressToPortMapper`/`RemotePorts` describing how to reach lower memory (used
  only at build time to populate the spec's mapper fields).

## Builder Pattern

Configuration is supplied as a whole through `WithSpec` (start from
`DefaultSpec()`); the engine and registration come from `WithRegistrar`; storage
and the address-to-port mapping come from `WithResources`. `Build` declares the
component's `Top`, `Bottom`, and `Control` ports; the port instances are built
with `modeling.MakePortBuilder` and attached after `Build` with `AssignPort`, so
the caller chooses each port's buffer size.

```go
spec := writethroughcache.DefaultSpec()
spec.WritePolicyType = "write-through"
spec.TotalByteSize = 256 * mem.KB
spec.WayAssociativity = 8

cache := writethroughcache.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(writethroughcache.Resources{
        AddressMapper: &mem.SinglePortMapper{Port: dramPort},
    }).
    Build("L2Cache")

for _, name := range []string{"Top", "Bottom", "Control"} {
    p := modeling.MakePortBuilder().
        WithRegistrar(sim).
        WithComponent(cache).
        WithSpec(modeling.PortSpec{BufSize: 16}).
        Build(name)
    cache.AssignPort(name, p)
}

topPort := cache.GetPortByName("Top")
```

### Builder Methods

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required). |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()`. |
| `WithResources(r)` | Backing storage and the address-to-port mapper / remote ports. Storage is built internally if omitted. |

## Ports

- **Top** — accepts `mem.ReadReq` and `mem.WriteReq`, returns
  `mem.DataReadyRsp` and `mem.WriteDoneRsp`.
- **Bottom** — sends `mem.ReadReq`/`mem.WriteReq` to lower memory and receives
  `mem.DataReadyRsp`/`mem.WriteDoneRsp`.
- **Control** — accepts `mem.ControlReq` (`CmdFlush` to reset the directory and
  drain in-flight work, `CmdEnable` to restart), returns `mem.ControlRsp`.
