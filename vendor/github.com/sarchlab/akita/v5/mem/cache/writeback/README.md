# writeback — Write-Back Cache Component

Package `writeback` provides a configurable write-back cache for the Akita
simulation framework. Dirty cache lines are written to lower-level memory only
on eviction, reducing write traffic through the memory hierarchy.

## How It Works

The cache is organized as a pipeline of stages, each executing every tick:

```
TopPort ──► TopParser ──► DirectoryStage ──► BankStage ──► (response)
                              │                   ▲
                              ▼                   │
                          MSHRStage ◄── WriteBufferStage ──► BottomPort
```

| Stage | Role |
|---|---|
| **TopParser** | Receives `mem.ReadReq` and `mem.WriteReq` from the top port, creates internal transactions, and pushes them into the directory stage buffer. |
| **DirectoryStage** | Looks up the set-associative directory. On a hit, routes to the bank stage. On a miss, allocates an MSHR entry and sends the transaction to the write buffer for fetching/eviction. |
| **BankStage** | Performs the actual data read/write through a latency pipeline. Handles hits, evictions, and fetched-data writes. Sends responses back through the top port. |
| **MSHRStage** | Processes completed MSHR entries when fetched data returns. Replays all waiting transactions that targeted the same cache line. |
| **WriteBufferStage** | Manages outstanding fetches and evictions to/from lower-level memory via the bottom port. Tracks inflight fetch and eviction counts. |

A separate `controlMW` runs a flusher that handles flush, invalidate, and pause
requests received on the `Control` port. The flusher can evict all dirty blocks,
optionally invalidate the directory, and pause the cache.

## Key Types

- `Spec` — immutable configuration: frequency, geometry, latencies, MSHR/buffer
  capacities, inflight limits, and address mapping. Port buffer sizes are chosen
  by the caller when assigning the port instances, not in the spec.
- `State` — mutable runtime data: the directory state, MSHR state, all
  inter-stage buffers and pipelines, the transaction list, and inflight
  counters. Fully JSON-serializable, as required by the `State` constraint.
- `Resources` — shared wiring; holds the backing `*mem.Storage` and the
  address-to-port mapping used to route fetches/evictions to lower memory
  (supply either `AddressToPortMapper` or `RemotePorts`).
- `Comp` — `modeling.Component[Spec, State, Resources]`.

```go
type Spec struct {
    Freq                timing.Freq // Operating frequency
    NumReqPerCycle      int         // Requests processed per cycle
    Log2BlockSize       uint64      // Cache line size = 1 << Log2BlockSize
    BankLatency         int         // Cycles per bank read/write
    DirLatency          int         // Cycles for directory lookup
    WayAssociativity    int         // Ways per set
    NumBanks            int         // Number of banks
    NumSets             int         // Sets (derived by Build)
    NumMSHREntry        int         // MSHR capacity
    TotalByteSize       uint64      // Total cache storage in bytes
    WriteBufferCapacity int         // Max lines in the write buffer
    MaxInflightFetch    int         // Concurrent fetches to lower memory
    MaxInflightEviction int         // Concurrent evictions to lower memory

    // Address mapping to lower-level memory.
    AddressMapperType string   // "single" or "interleaved"
    RemotePortNames   []string
    InterleavingSize  uint64
}
```

## Builder Pattern

Start from `DefaultSpec()`, tweak the fields you need, and pass the whole spec to
`WithSpec`. Wiring comes from `WithRegistrar` (which provides the engine and
registers the component) and `WithResources` (the backing storage plus the
address-to-port mapping for lower memory). When the storage is omitted, the
component builds its own sized by `Spec.TotalByteSize`. `Build` only *declares*
the `Top`, `Bottom`, and `Control` ports; the caller builds the port instances
(choosing their buffer sizes) and attaches them with `AssignPort` after `Build`.

```go
spec := writeback.DefaultSpec()
spec.TotalByteSize = 64 * mem.KB
spec.WayAssociativity = 4
spec.Log2BlockSize = 6 // 64-byte lines
spec.NumMSHREntry = 16

cache := writeback.MakeBuilder().
    WithRegistrar(sim).
    WithSpec(spec).
    WithResources(writeback.Resources{
        AddressToPortMapper: lowModuleMapper,
    }).
    Build("L1Cache")

// Build only declares the ports; assign the instances (and pick their buffer
// sizes) after Build.
for _, name := range []string{"Top", "Bottom", "Control"} {
    p := modeling.MakePortBuilder().
        WithRegistrar(sim).
        WithComponent(cache).
        WithSpec(modeling.PortSpec{BufSize: 8}).
        Build(name)
    cache.AssignPort(name, p)
}

topPort := cache.GetPortByName("Top")
```

| Method | Description |
|---|---|
| `WithRegistrar(r)` | Source of the engine and component registration (required) |
| `WithSpec(s)` | Full configuration; start from `DefaultSpec()` and tweak |
| `WithResources(Resources{...})` | Backing storage and the lower-memory address mapping (`AddressToPortMapper` or `RemotePorts`) |

### Default Configuration

| Parameter | Default |
|---|---|
| Frequency | 1 GHz |
| Block size | 64 bytes (log2 = 6) |
| Way associativity | 4 |
| Banks | 1 |
| Total size | 512 KB |
| Bank latency | 10 cycles |
| Requests per cycle | 1 |
| MSHR entries | 16 |
| Write buffer capacity | 1024 |
| Max inflight fetch / eviction | 128 / 128 |
| Interleaving size | 4096 |

## Cache States

The cache operates in one of five states (the `cacheState` constants):

- **Running** — normal operation, processing read/write requests
- **PreFlushing** — preparing for a flush operation
- **Flushing** — evicting dirty blocks to lower memory
- **Paused** — pipeline halted (after flush-with-pause)
- **Invalid** — uninitialized

## Ports

`Build` declares these ports by logical name; the caller assigns the instances
with `AssignPort` (see the Builder Pattern section above).

- **Top**: accepts `mem.ReadReq` and `mem.WriteReq`, returns `mem.DataReadyRsp`
  and `mem.WriteDoneRsp`.
- **Bottom**: issues `mem.ReadReq` (fetches) and `mem.WriteReq` (evictions) to
  lower-level memory.
- **Control**: accepts `mem.ControlReq` for flush/invalidate/pause operations,
  returns `mem.ControlRsp`.
