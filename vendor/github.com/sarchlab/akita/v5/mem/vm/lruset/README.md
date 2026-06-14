# lruset — LRU Set Utility

Package `lruset` provides a small least-recently-used set data structure for the
Akita simulation framework. It is a shared utility within the virtual-memory
subsystem, used by the `tlb` and `mmuCache` components to track replacement order
and key-to-way mappings for the ways of one cache set.

## How It Works

A `Set` is created with a fixed number of ways. It maintains a visit list ordered
from least- to most-recently-used and a map from a string key to a way ID. The
`Set` is payload-agnostic: each consumer stores its own block data separately and
uses the `Set` only for ordering and key resolution.

All ways start in the visit list, so the first `Evict` returns way 0.

## Key Types

```go
type Set struct { /* unexported fields */ }

func NewSet(numWays int) Set
func KeyString(a uint64, b uint64) string // canonical key from two uint64s

func (s *Set) Lookup(key string) (wayID int, found bool)
func (s *Set) UpdateKey(wayID int, oldKey, newKey string)
func (s *Set) Evict() (wayID int, ok bool)  // remove and return the LRU way
func (s *Set) Visit(wayID int)              // mark a way most-recently-used
```

`KeyString` builds a canonical lookup key from two `uint64` values, for example a
PID and a virtual address (TLB) or a PID and a page-table segment (mmuCache).

## Usage Example

```go
set := lruset.NewSet(4)
key := lruset.KeyString(uint64(pid), vAddr)

if wayID, found := set.Lookup(key); found {
    set.Visit(wayID) // hit: mark most-recently-used
} else {
    wayID, _ := set.Evict()          // miss: take the LRU way
    set.UpdateKey(wayID, oldKey, key) // remap the key, store payload separately
    set.Visit(wayID)
}
```
