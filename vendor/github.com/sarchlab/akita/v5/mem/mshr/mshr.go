// Package mshr provides generic MSHR (Miss Status Holding Register) operations
// shared by both cache and TLB implementations.
package mshr

import "github.com/sarchlab/akita/v5/mem/vm"

// Entry is the interface that any MSHR entry must implement for use with the
// generic MSHR operations.
type Entry interface {
	GetPID() uint32
	GetAddress() uint64
}

// Find returns the index and true if an entry with the given pid and addr
// exists in entries, or (-1, false) otherwise.
func Find[E Entry](entries []E, pid vm.PID, addr uint64) (int, bool) {
	for i, e := range entries {
		if vm.PID(e.GetPID()) == pid && e.GetAddress() == addr {
			return i, true
		}
	}

	return -1, false
}

// IsPresent returns true if an entry with the given pid and addr exists.
func IsPresent[E Entry](entries []E, pid vm.PID, addr uint64) bool {
	_, found := Find(entries, pid, addr)
	return found
}

// IsFull returns true if the number of entries has reached capacity.
func IsFull[E Entry](entries []E, capacity int) bool {
	return len(entries) >= capacity
}

// IsEmpty returns true if there are no entries.
func IsEmpty[E Entry](entries []E) bool {
	return len(entries) == 0
}

// Remove removes the entry matching pid and addr from entries and returns the
// updated slice. Panics if no matching entry exists.
func Remove[E Entry](entries []E, pid vm.PID, addr uint64) []E {
	for i, e := range entries {
		if vm.PID(e.GetPID()) == pid && e.GetAddress() == addr {
			return append(entries[:i], entries[i+1:]...)
		}
	}

	panic("trying to remove non-exist MSHR entry")
}
