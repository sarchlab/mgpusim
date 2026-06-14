# tlb — Translation Lookaside Buffer

Package `tlb` provides a translation lookaside buffer for the Akita simulation
framework. It is part of the virtual-memory subsystem: it caches recent
virtual-to-physical address translations so that repeated translations hit
locally, and forwards misses down to the MMU (or another translation provider).

## How It Works

The TLB is a set-associative cache of `vm.Page` entries keyed by `(PID, VAddr)`.
Translation requests arrive on the `Top` port as `vm.TranslationReq` messages and
flow through a fixed-latency pipeline before lookup:

1. **Insert / pipeline** — Incoming requests are accepted into a
   `queueing.Pipeline` that imposes `Latency` cycles. Up to `NumReqPerCycle`
   requests move per tick.
2. **Lookup** — On exit from the pipeline the request is looked up in the set
   selected by `vAddr / PageSize % NumSets`.
   - **Hit** (entry present and `Valid`): a `vm.TranslationRsp` carrying the
     `vm.Page` is sent back on `Top`, and the matched way is marked
     most-recently-used.
   - **MSHR hit**: an outstanding miss already covers this `(PID, VAddr)`, so the
     request is attached to the existing MSHR entry.
   - **Miss**: an MSHR entry is allocated and a `vm.TranslationReq` is forwarded
     on the `Bottom` port to the translation provider resolved by the
     `TranslationProviderMapper`.
3. **Fill / respond** — Responses returning on `Bottom` evict an LRU way, install
   the page, and replay every request queued in the matching MSHR entry back to
   `Top`.

Replacement order within each set is tracked by `lruset.Set`; in-flight misses
are tracked by the shared `mshr` package.

## Key Types

- `Spec` — immutable configuration (frequency, geometry, latency, MSHR size).
- `State` — mutable runtime data: per-set blocks and LRU order, MSHR entries, the
  request pipeline, and flush bookkeeping. The TLB runs a small state machine:
  `enable`, `drain`, `pause`, and `flush`.
- `Resources` — external wiring; holds the `TranslationProviderMapper`
  (`mem.AddressToPortMapper`) used to locate the downstream provider for a
  virtual address.
- `Comp` — `modeling.Component[Spec, State, Resources]`.

## Builder Pattern

```go
spec := tlb.DefaultSpec()
spec.NumSets = 64
spec.NumWays = 4
spec.MSHRSize = 8

t := tlb.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(tlb.Resources{
        TranslationProviderMapper: mmuMapper,
    }).
    Build("L2TLB")
```

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{...})` | External wiring (the translation provider mapper) |

## Ports

`Build` declares the ports below by logical name; it does not create the port
instances. After `Build`, the caller builds each port with
`modeling.MakePortBuilder()` (choosing the buffer size) and attaches it with
`comp.AssignPort(name, port)`:

```go
t := tlb.MakeBuilder().WithRegistrar(sim).Build("L2TLB")
for _, name := range []string{"Top", "Bottom", "Control"} {
    p := modeling.MakePortBuilder().
        WithRegistrar(sim).
        WithComponent(t).
        WithSpec(modeling.PortSpec{BufSize: 4}).
        Build(name)
    t.AssignPort(name, p)
}
```

- **Top**: accepts `vm.TranslationReq`, returns `vm.TranslationRsp`.
- **Bottom**: forwards `vm.TranslationReq` on a miss, receives `vm.TranslationRsp`.
- **Control**: accepts `mem.ControlReq` (enable / drain / pause / flush / reset)
  and returns `mem.ControlRsp` for flush and reset.
