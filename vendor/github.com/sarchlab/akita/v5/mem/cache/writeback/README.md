# writeback — Write-Back Cache Component

Package `writeback` implements a configurable write-back cache for the Akita
simulation framework. Dirty cache lines are written to lower-level memory only
on eviction, reducing write traffic to the memory hierarchy.

## Architecture

The cache is organized as a pipeline of stages, each executing every tick:

```
TopPort ──► TopParser ──► DirectoryStage ──► BankStage ──► (response)
                              │                   ▲
                              ▼                   │
                          MSHRStage ◄── WriteBufferStage ──► BottomPort
```

### Pipeline Stages

| Stage | Role |
|---|---|
| **TopParser** | Receives `mem.ReadReq` and `mem.WriteReq` from the top port, creates internal transactions, and pushes them into the directory stage buffer. |
| **DirectoryStage** | Looks up the set-associative directory. On a hit, routes to the bank stage. On a miss, allocates an MSHR entry and sends the transaction to the write buffer for fetching/eviction. |
| **BankStage** | Performs the actual data read/write through a latency pipeline. Handles hits, evictions, and fetched-data writes. Sends responses back through the top port. |
| **MSHRStage** | Processes completed MSHR entries when fetched data returns. Replays all waiting transactions that targeted the same cache line. |
| **WriteBufferStage** | Manages outstanding fetches and evictions to/from lower-level memory via the bottom port. Tracks inflight fetch and eviction counts. |

### Control Middleware

A separate `controlMW` handles flush, invalidate, and pause requests received
on the control port. The flusher can evict all dirty blocks, optionally
invalidate the directory, and pause the cache.

## Key Types

### Spec (immutable configuration)

```go
type Spec struct {
    Freq                sim.Freq   // Operating frequency (Hz)
    NumReqPerCycle      int        // Requests processed per cycle
    Log2BlockSize       uint64     // Cache line size = 1 << Log2BlockSize
    BankLatency         int        // Cycles per bank read/write
    DirLatency          int        // Cycles for directory lookup
    WayAssociativity    int        // Number of ways per set
    NumBanks            int        // Number of banks
    NumMSHREntry        int        // MSHR capacity
    TotalByteSize       uint64     // Total cache storage in bytes
    WriteBufferCapacity int        // Max lines in the write buffer
    MaxInflightFetch    int        // Concurrent fetches to lower memory
    MaxInflightEviction int        // Concurrent evictions to lower memory
}
```

### State (mutable runtime data)

Contains the directory state, MSHR state, all inter-stage buffers and
pipelines, transaction list, and inflight counters. Fully JSON-serializable
for checkpointing.

## Builder Pattern

```go
cache := writeback.MakeBuilder().
    WithEngine(engine).
    WithFreq(1 * sim.GHz).
    WithByteSize(64 * mem.KB).
    WithWayAssociativity(4).
    WithLog2BlockSize(6).          // 64-byte lines
    WithBankLatency(10).
    WithNumMSHREntry(16).
    WithNumReqPerCycle(1).
    WithTopPort(topPort).
    WithBottomPort(bottomPort).
    WithControlPort(controlPort).
    WithRemotePorts(lowModulePort).
    Build("L1Cache")
```

### Builder Options

| Method | Description |
|---|---|
| `WithEngine(e)` | Event scheduler (required) |
| `WithFreq(f)` | Operating frequency |
| `WithByteSize(n)` | Total cache capacity |
| `WithWayAssociativity(n)` | Ways per set |
| `WithLog2BlockSize(n)` | Line size as power of 2 |
| `WithBankLatency(n)` | Bank pipeline depth in cycles |
| `WithDirectoryLatency(n)` | Directory pipeline depth in cycles |
| `WithNumMSHREntry(n)` | Number of MSHR entries |
| `WithNumReqPerCycle(n)` | Throughput per cycle |
| `WithWriteBufferSize(n)` | Write buffer capacity |
| `WithMaxInflightFetch(n)` | Max concurrent fetches |
| `WithMaxInflightEviction(n)` | Max concurrent evictions |
| `WithTopPort(p)` | Port facing higher-level requestors |
| `WithBottomPort(p)` | Port facing lower-level memory |
| `WithControlPort(p)` | Port for flush/invalidate control |
| `WithRemotePorts(p...)` | Lower-level memory ports for address mapping |
| `WithInterleavingSize(n)` | Address interleaving granularity |

## Default Configuration

| Parameter | Default |
|---|---|
| Frequency | 1 GHz |
| Block size | 64 bytes (log2 = 6) |
| Way associativity | 4 |
| Total size | 512 KB |
| Bank latency | 10 cycles |
| MSHR entries | 16 |
| Write buffer capacity | 1024 |
| Max inflight fetch | 128 |
| Max inflight eviction | 128 |

## Cache States

The cache operates in one of five states:

- **Running** — normal operation, processing read/write requests
- **PreFlushing** — preparing for a flush operation
- **Flushing** — evicting dirty blocks to lower memory
- **Paused** — pipeline halted (after flush-with-pause)
- **Invalid** — uninitialized

## Protocol

- **Top port**: accepts `mem.ReadReq` and `mem.WriteReq`, returns
  `mem.DataReadyRsp` and `mem.WriteDoneRsp`
- **Bottom port**: issues `mem.ReadReq` (fetches) and `mem.WriteReq`
  (evictions) to lower-level memory
- **Control port**: accepts `mem.ControlReq` for flush/invalidate/pause
  operations, returns `mem.ControlRsp`
