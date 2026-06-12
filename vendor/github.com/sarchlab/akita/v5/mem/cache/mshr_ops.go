package cache

import (
	"github.com/sarchlab/akita/v5/mem/mshr"
	"github.com/sarchlab/akita/v5/mem/vm"
)

// MSHRQuery finds the entry matching pid+addr in MSHRState.
// Returns (entryIdx, found).
func MSHRQuery(ms *MSHRState, pid vm.PID, addr uint64) (int, bool) {
	return mshr.Find(ms.Entries, pid, addr)
}

// MSHRAdd creates a new entry in MSHRState for pid+addr.
// Panics if the MSHR is already at capacity.
func MSHRAdd(ms *MSHRState, capacity int, pid vm.PID, addr uint64) int {
	if mshr.IsFull(ms.Entries, capacity) {
		panic("MSHR is full")
	}

	entry := MSHREntryState{
		PID:     uint32(pid),
		Address: addr,
	}

	ms.Entries = append(ms.Entries, entry)

	return len(ms.Entries) - 1
}

// MSHRRemove removes the entry matching pid+addr from MSHRState.
// Panics if no such entry exists.
func MSHRRemove(ms *MSHRState, pid vm.PID, addr uint64) {
	ms.Entries = mshr.Remove(ms.Entries, pid, addr)
}

// MSHRIsFull returns true if the MSHRState has reached its capacity.
func MSHRIsFull(ms *MSHRState, capacity int) bool {
	return mshr.IsFull(ms.Entries, capacity)
}
