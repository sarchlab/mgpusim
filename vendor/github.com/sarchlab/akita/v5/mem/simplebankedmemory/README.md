# simplebankedmemory — Banked Memory Controller

Package `simplebankedmemory` provides a configurable banked memory controller
for the Akita simulation framework. It models a banked memory subsystem built on
top of Akita's pipeline primitives and is a good default memory unit for most
simulations: it is not a detailed DRAM model, but provides controllable
bandwidth and latency for the common case.

## How It Works

The component exposes a single `Top` port. All clients share this port, and the
internal logic determines which bank serves each request. Each bank owns a
configurable pipeline (width, depth, per-stage latency) and a post-pipeline
buffer that holds completed items until responses can be delivered.

Requests traverse the following stages:

1. **Ingress** — Messages arriving at the `Top` port remain queued in the port's
   incoming buffer.
2. **Bank selection** — On each tick the dispatch middleware takes requests from
   the port buffer and dispatches them into the selected bank's pipeline. With
   the default `"interleaved"` selector, addresses are interleaved by a
   configurable stride (`2 ^ BankSelectorLog2InterleaveSize` bytes).
3. **Pipeline traversal** — Banks simulate execution latency by advancing all
   in-flight pipeline slots every tick.
4. **Completion** — When items exit the pipeline, reads gather their data from
   the backing `mem.Storage` while writes commit the modified bytes. Both
   generate responses once the `Top` port has space to send.

For a quick approximation, the achievable peak bandwidth is
`NumBanks × BankPipelineWidth × (1 / StageLatency) × Freq`. To keep sequential
traffic saturated, configure more banks than the pipeline latency so a stream of
requests can occupy different banks while earlier ones are still in flight.

## Key Types

- `Spec` — immutable configuration: frequency, bank geometry, pipeline shape,
  post-pipeline buffer depth, capacity, and bank selection.
- `State` — mutable runtime data: the per-bank pipelines and post-pipeline
  buffers.
- `Resources` — shared wiring; holds the backing `*mem.Storage`.
- `Comp` — `modeling.Component[Spec, State, Resources]`.

```go
type Spec struct {
    Freq                timing.Freq // Operating frequency
    NumBanks            int         // Number of banks
    BankPipelineWidth   int         // Items entering a bank pipeline per tick
    BankPipelineDepth   int         // Pipeline stages per bank
    StageLatency        int         // Cycles per pipeline stage
    PostPipelineBufSize int         // Post-pipeline buffer depth per bank
    Capacity            uint64      // Backing-storage size when built internally
    StorageRef          string      // Storage resource name (set by Build)

    BankSelectorKind               string // "interleaved"
    BankSelectorLog2InterleaveSize uint64 // log2 of the bank interleave stride

    // Optional bank-selection address conversion (bank selection only;
    // storage is always global). Empty kind = identity. See "Bank selection
    // across interleaved controllers" below.
    BankAddrConvKind            string
    BankAddrInterleavingSize    uint64
    BankAddrTotalNumOfElements  int
    BankAddrCurrentElementIndex int
    BankAddrOffset              uint64
}
```

The memory uses **global storage**: a request's address indexes the backing
store directly, with no storage address conversion. Bank selection, by default,
also runs on the request's global address.

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
spec := simplebankedmemory.DefaultSpec()
spec.NumBanks = 4
spec.BankPipelineWidth = 2
spec.BankPipelineDepth = 3
spec.StageLatency = 2
spec.BankSelectorLog2InterleaveSize = 6 // 64 B stride

memCtrl := simplebankedmemory.MakeBuilder().
    WithRegistrar(reg).
    WithSpec(spec).
    WithResources(simplebankedmemory.Resources{Storage: storage}).
    Build("MyMemCtrl")

topPort := modeling.MakePortBuilder().
    WithRegistrar(reg).
    WithComponent(memCtrl).
    WithSpec(modeling.PortSpec{BufSize: 16}).
    Build("Top")
memCtrl.AssignPort("Top", topPort)

ctrlPort := modeling.MakePortBuilder().
    WithRegistrar(reg).
    WithComponent(memCtrl).
    WithSpec(modeling.PortSpec{BufSize: 4}).
    Build("Control")
memCtrl.AssignPort("Control", ctrlPort)

topPort = memCtrl.GetPortByName("Top")
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
| Banks | 4 |
| Bank pipeline width / depth | 1 / 1 |
| Stage latency | 10 cycles |
| Post-pipeline buffer | 1 |
| Storage capacity | 4 GB |
| Bank selector | `"interleaved"`, 64 B stride (log2 = 6) |

## Bank selection across interleaved controllers

A common deployment places several of these controllers behind an interleaved
address mapper (the sender's destination map), all sharing one global
`mem.Storage`. Banking is then **two-level**: the upstream mapper selects the
controller (level 1) and each controller's bank selector spreads its traffic
across banks (level 2). Storage is global, so a request's address indexes the
shared store directly regardless of which controller serves it.

Every request reaching a controller carries the inter-controller interleave
bits fixed to that controller's index. The bank selector is a contiguous-bit
modulo, so running it on the **global** address makes those fixed bits overlap
the bank-select bits: bank selection aliases and only a fraction of the banks
is ever used (data stays correct; only timing is wrong, and silently so). For
example, 16 controllers interleaved at 128 B feeding 16-bank memories with a
64 B bank stride reach only 2 of the 16 banks per controller — an ~8× bandwidth
loss.

Fix it with the **bank-selection conversion**: set the `BankAddrConv*` fields to
strip the inter-controller interleaving, so bank selection runs on a contiguous
**controller-local** address and stripes finely across all banks. Storage stays
global (these fields never affect storage):

```go
spec := simplebankedmemory.DefaultSpec()
spec.NumBanks = 16
spec.BankSelectorLog2InterleaveSize = 6 // 64 B bank stride

// This controller is element i of 16, interleaved at 128 B. Strip that
// interleaving for bank selection only.
spec.BankAddrConvKind = "interleaving"
spec.BankAddrInterleavingSize = 128
spec.BankAddrTotalNumOfElements = 16
spec.BankAddrCurrentElementIndex = i
```

Now consecutive controller-local cache lines land on consecutive banks: all 16
banks are used, with full 64 B-granular striping (e.g. element-i lines that were
the two halves of a 128 B block now hit different banks). Only needed when this
memory is one of several interleaved controllers; a standalone memory leaves
`BankAddrConvKind` empty and selects banks on the request address directly. Use
`mem/dram` if you need detailed bank/row timing.

## Ports

- **Top**: accepts `mem.ReadReq` and `mem.WriteReq`, returns `mem.DataReadyRsp`
  and `mem.WriteDoneRsp`.
- **Control**: accepts `mem.ControlReq` (enable / pause / drain / reset),
  returns `mem.ControlRsp`.

## Example

The package ships with a runnable example that issues sequential 64 B reads and
prints the achieved bandwidth:

```sh
go test ./mem/simplebankedmemory -run Example
```
