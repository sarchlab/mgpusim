# cache — Shared Cache Building Blocks

Package `cache` provides the common data structures and helper operations used
by the concrete cache implementations in the Akita simulation framework. It is
the parent package for the cache components (such as
[`writethroughcache`](writethroughcache) and `writeback`) and does not itself
define a runnable component. Instead it supplies the serializable directory and
MSHR state plus the pure functions that operate on them, so every cache shares
the same set-associative lookup, LRU replacement, and miss-handling logic.

## How It Works

A cache implementation embeds a `DirectoryState` and an `MSHRState` inside its
own `State` struct. Each tick the implementation calls the free functions in
this package to look up blocks, pick eviction victims, track LRU order, and
coalesce outstanding misses. All state is plain JSON-serializable data (no
pointers or interfaces), so it satisfies `modeling.ValidateState` and can be
checkpointed.

## Key Types

### Directory state

```go
type BlockState struct {
    PID          uint32
    Tag          uint64   // cache-line-aligned address
    WayID, SetID int
    CacheAddress uint64   // offset into the backing storage
    IsValid      bool
    IsDirty      bool
    ReadCount    int      // outstanding readers (blocks eviction)
    IsLocked     bool     // a fetch/write is in flight
    DirtyMask    []bool
}

type SetState struct {
    Blocks   []BlockState
    LRUOrder []int        // way IDs, LRU first … MRU last
}

type DirectoryState struct {
    Sets []SetState
}
```

### MSHR state

```go
type MSHREntryState struct {
    PID                uint32
    Address            uint64
    TransactionIndices []int   // transactions coalesced onto this miss
    BlockSetID, BlockWayID int
    HasBlock           bool
    ReadReq, DataReady messaging.MsgMeta
    Data               []byte
    // …HasReadReq / HasDataReady flags
}

type MSHRState struct {
    Entries []MSHREntryState
}
```

## Directory Operations

| Function | Purpose |
|---|---|
| `DirectoryReset(ds, numSets, numWays, blockSize)` | Initialize an empty directory with default LRU ordering. |
| `DirectoryLookup(ds, numSets, blockSize, pid, addr)` | Find the `(setID, wayID, found)` for a cache-line-aligned `pid+addr`. |
| `DirectoryFindVictim(ds, numSets, blockSize, addr)` | Pick an eviction victim in LRU order, skipping locked ways and ways with `ReadCount > 0`. |
| `DirectoryVisit(ds, setID, wayID)` | Move a way to the MRU position. |

`DirectoryLookup` maps an address to a set via
`addr / blockSize % numSets` and matches on `IsValid && Tag == addr && PID`.

## MSHR Operations

These delegate to the lower-level `mem/mshr` package, keeping cache state in the
serializable `MSHRState` form.

| Function | Purpose |
|---|---|
| `MSHRQuery(ms, pid, addr)` | Return `(entryIdx, found)` for a matching outstanding miss. |
| `MSHRAdd(ms, capacity, pid, addr)` | Allocate a new entry (panics if full). |
| `MSHRRemove(ms, pid, addr)` | Remove the matching entry (panics if absent). |
| `MSHRIsFull(ms, capacity)` | Report whether capacity has been reached. |

## Related Packages

- [`writethroughcache`](writethroughcache) — a unified GPU cache supporting the
  write-around, write-evict, and write-through policies.
- `writeback` — a write-back cache built on the same directory and MSHR helpers.
- `mem/mshr` — the underlying miss-status-handling-register primitives.
