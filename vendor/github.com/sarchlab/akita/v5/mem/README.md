# mem

Package `mem` and its sub-packages define the memory subsystem for Akita
simulations, including the memory protocol, storage abstraction, address
mapping utilities, and implementations of caches, DRAM, and virtual memory.

## Protocol

The memory protocol (package `mem/memprotocol`) defines request/response
messages between memory components (caches, memory controllers, compute
units), with `requester`/`responder` roles that ports bind to.

### Access Messages

| Type | Direction | Fields |
|------|-----------|--------|
| `ReadReq` | Request | `Address`, `AccessByteSize`, `PID` |
| `WriteReq` | Request | `Address`, `Data []byte`, `DirtyMask []bool`, `PID` |
| `DataReadyRsp` | Response | `Data []byte` |
| `WriteDoneRsp` | Response | *(empty — acknowledgment only)* |

All messages embed `messaging.MsgMeta` for routing (Src, Dst, ID, RspTo).

The `AccessReq` interface (also in `mem/memprotocol`) unifies read/write
requests with `GetAddress()`, `GetByteSize()`, and `GetPID()`.

### Control Messages

`memcontrolprotocol.Req` and `memcontrolprotocol.Rsp` (package `mem/memcontrolprotocol`) carry the uniform
control protocol used by every memory agent: Pause, Drain, Enable, Reset,
Invalidate, Flush. Each component exposes a `Control` port that carries
these messages and only these messages.

See [`CONTROL_PROTOCOL.md`](CONTROL_PROTOCOL.md) for verb definitions,
response timing, the support matrix, and how to implement and test the
protocol in a new component. The reusable state enum and conformance
harness live in `mem/memcontrolprotocol/` (see
[`control/README.md`](control/README.md)).

## Storage

`Storage` is a sparse, page-based byte store that models physical memory:

```go
storage := mem.NewStorage(4 * mem.GB)

// Write data
storage.Write(0x1000, []byte{0xDE, 0xAD})

// Read data
data, err := storage.Read(0x1000, 2)
```

A `Storage` can be registered with the simulation as shared state via
`NewStorageResource(name, storage)`, making its contents reachable by name
through the global state manager.

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

Maps a memory address to the `messaging.RemotePort` of the component responsible
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
