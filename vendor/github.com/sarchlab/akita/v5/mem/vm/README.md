# vm — Virtual Memory Subsystem

Package `vm` provides the virtual memory infrastructure for the Akita
simulation framework. It defines the page table, page structure, process IDs,
and the translation request/response protocol used throughout the memory
hierarchy.

## Core Types

### PID

```go
type PID uint32
```

Process identifier used to distinguish address spaces in a multi-process
simulation.

### Page

```go
type Page struct {
    PID         PID
    PAddr       uint64   // Physical address
    VAddr       uint64   // Virtual address
    PageSize    uint64   // Size of the page in bytes
    Valid       bool
    DeviceID    uint64   // GPU or device hosting this page
    Unified     bool     // Unified memory page
    IsMigrating bool     // Currently being migrated
    IsPinned    bool     // Pinned (non-migratable)
}
```

### PageTable

```go
type PageTable interface {
    Insert(page Page)
    Remove(pid PID, vAddr uint64)
    Find(pid PID, addr uint64) (Page, bool)
    Update(page Page)
    ReverseLookup(pAddr uint64) (Page, bool)
}
```

Create a page table with `NewPageTable(log2PageSize)`. The implementation
is thread-safe and maintains per-process tables internally.

## Translation Protocol

Address translation uses a request/response protocol:

```go
// Request: virtual → physical translation
type TranslationReq struct {
    sim.MsgMeta
    VAddr    uint64
    PID      PID
    DeviceID uint64
}

// Response: carries the resolved Page
type TranslationRsp struct {
    sim.MsgMeta
    Page Page
}
```

Page migration is coordinated between the MMU and the driver via
`PageMigrationReqToDriver` and `PageMigrationRspFromDriver`.

## Address Translation Flow

```
Application (virtual address)
       │
       ▼
  AddressTranslator ──► TLB (fast path)
       │                    │
       │                    ▼ (miss)
       │               MMU Cache
       │                    │
       │                    ▼ (miss)
       │                MMU / GMMU ──► PageTable
       │                    │
       │◄───────────────────┘ (TranslationRsp with Page)
       │
       ▼
  Physical memory access
```

## Sub-Packages

| Package | Description |
|---|---|
| **tlb** | Translation Lookaside Buffer — caches recent virtual-to-physical translations per process. Handles TLB hits locally and forwards misses to the MMU. |
| **mmu** | Memory Management Unit — performs page table walks and manages page allocation. Supports auto-allocation for unmapped pages. |
| **gmmu** | GPU Memory Management Unit — specialized MMU for GPU-side page table walks. Reads page table entries from memory via the bottom port. |
| **addresstranslator** | Sits between a compute unit and memory. Translates virtual addresses in `mem.ReadReq`/`mem.WriteReq` to physical addresses before forwarding to the memory hierarchy. |
| **mmuCache** | Caches translation results between the TLB and MMU to reduce page walk traffic. |
| **lruset** | LRU set implementation used by the TLB for replacement decisions. |

## Usage Example

```go
// Create a page table with 4 KB pages (log2 = 12)
pageTable := vm.NewPageTable(12)

// Insert a mapping
pageTable.Insert(vm.Page{
    PID:      1,
    VAddr:    0x1000,
    PAddr:    0x80000,
    PageSize: 4096,
    Valid:    true,
    DeviceID: 0,
})

// Look up a translation
page, found := pageTable.Find(1, 0x1000)
```

## TLB Tracing

The `TLBTracer` type records TLB access events (hits and misses) and can
be attached to a TLB component for performance analysis.
