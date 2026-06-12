# mshr — Miss Status Holding Registers

Package `mshr` provides generic MSHR (Miss Status Holding Register) operations
shared by cache and TLB implementations in the Akita simulation framework.

MSHRs track outstanding cache misses so that multiple requests to the same
cache line can be coalesced into a single fetch from lower-level memory.

## Entry Interface

Any type used as an MSHR entry must implement the `Entry` interface:

```go
type Entry interface {
    GetPID() uint32
    GetAddress() uint64
}
```

Entries are identified by the combination of process ID (PID) and address.

## Generic Functions

All functions are generic over any `Entry` type, operating on plain slices:

```go
// Find returns the index of the entry matching (pid, addr), or (-1, false).
func Find[E Entry](entries []E, pid vm.PID, addr uint64) (int, bool)

// IsPresent returns true if a matching entry exists.
func IsPresent[E Entry](entries []E, pid vm.PID, addr uint64) bool

// IsFull returns true if len(entries) >= capacity.
func IsFull[E Entry](entries []E, capacity int) bool

// IsEmpty returns true if the slice is empty.
func IsEmpty[E Entry](entries []E) bool

// Remove removes the matching entry and returns the updated slice.
// Panics if no matching entry exists.
func Remove[E Entry](entries []E, pid vm.PID, addr uint64) []E
```

## Usage Example

```go
type myMSHREntry struct {
    pid  uint32
    addr uint64
    // ... additional fields (waiting transactions, fetched data, etc.)
}

func (e myMSHREntry) GetPID() uint32     { return e.pid }
func (e myMSHREntry) GetAddress() uint64 { return e.addr }

// Check capacity before adding
entries := []myMSHREntry{}
if !mshr.IsFull(entries, 16) {
    entries = append(entries, myMSHREntry{pid: 1, addr: 0x1000})
}

// Look up an existing entry
if idx, found := mshr.Find(entries, 1, 0x1000); found {
    entry := entries[idx]
    // ... coalesce with existing miss
}

// Remove when fetch completes
entries = mshr.Remove(entries, 1, 0x1000)
```

## Design Notes

- **Slice-based**: MSHR entries are stored in plain Go slices rather than
  maps or linked lists, keeping them JSON-serializable for checkpointing.
- **Generic**: Functions work with any concrete entry type, so caches and
  TLBs can define their own entry structs with additional fields.
- **Lookup by (PID, Address)**: Entries are uniquely identified by the
  process ID and the aligned address (typically cache-line or page aligned).
