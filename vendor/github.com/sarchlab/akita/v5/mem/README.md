# mem

Package `mem` and its sub-packages define the memory subsystem for Akita
simulations, including the memory protocol, storage abstraction, address
mapping utilities, and implementations of caches, DRAM, and virtual memory.

## Protocol

The memory protocol defines request/response messages between memory
components (caches, memory controllers, compute units).

### Access Messages

| Type | Direction | Fields |
|------|-----------|--------|
| `ReadReq` | Request | `Address`, `AccessByteSize`, `PID` |
| `WriteReq` | Request | `Address`, `Data []byte`, `DirtyMask []bool`, `PID` |
| `DataReadyRsp` | Response | `Data []byte` |
| `WriteDoneRsp` | Response | *(empty — acknowledgment only)* |

All messages embed `sim.MsgMeta` for routing (Src, Dst, ID, RspTo).

The `AccessReq` interface unifies read/write requests with `GetAddress()`,
`GetByteSize()`, and `GetPID()`.

### Control Messages

`ControlReq` provides a unified control interface for all memory components:

```go
type ControlReq struct {
    sim.MsgMeta
    Command         ControlCommand
    DiscardInflight bool
    InvalidateAfter bool
    PauseAfter      bool
    Addresses       []uint64
    PID             vm.PID
}
```

| Command | Description |
|---------|-------------|
| `CmdFlush` | Write back dirty data |
| `CmdInvalidate` | Invalidate entries (no writeback) |
| `CmdDrain` | Wait for in-flight operations to complete |
| `CmdReset` | Soft reset |
| `CmdPause` | Disable further processing |
| `CmdEnable` | Re-enable processing |

Responses use `ControlRsp` with `Command` and `Success` fields.

## Storage

`Storage` is a sparse, page-based byte store that models physical memory:

```go
storage := mem.NewStorage(4 * mem.GB)

// Write data
storage.Write(0x1000, []byte{0xDE, 0xAD})

// Read data
data, err := storage.Read(0x1000, 2)
```

Storage supports binary checkpoint save/load via `Save(w)` / `Load(r)`.

Capacity constants: `KB`, `MB`, `GB`, `TB`.

## Address Mapping

### AddressConverter

Converts between external (global) and internal (per-element) addresses for
interleaved address spaces:

```go
conv := mem.InterleavingConverter{
    InterleavingSize:    4096,
    TotalNumOfElements:  8,
    CurrentElementIndex: 2,
    Offset:              0,
}
internal := conv.ConvertExternalToInternal(externalAddr)
```

### AddressToPortMapper

Maps a memory address to the `sim.RemotePort` of the component responsible
for that address:

| Mapper | Use Case |
|--------|----------|
| `SinglePortMapper` | Single downstream module |
| `InterleavedAddressPortMapper` | Interleaved address distribution |
| `BankedAddressPortMapper` | Banked address distribution |

## Sub-Packages

| Package | Description |
|---------|-------------|
| `mem/cache` | Cache implementations (L1, L2, etc.) |
| `mem/dram` | DRAM controller models |
| `mem/vm` | Virtual memory / page table management |
| `mem/idealmemcontroller` | Ideal (zero-latency) memory controller |
| `mem/mshr` | Miss Status Holding Registers |
| `mem/datamover` | Data movement between memory components |
| `mem/trace` | Memory access tracing utilities |
| `mem/simplebankedmemory` | Simple banked memory model |
