package runtime

import "github.com/sarchlab/akita/v5/mem/vm"

// PageTablePool returns the base and length of the reserved prefix of a
// physical device, or a zero length for simplified translation.
func (r *Runtime) PageTablePool(device int) (base, size uint64) {
	size = r.config.GPUPageTableBytes
	if device == 0 {
		size = r.config.CPUPageTableBytes
	}
	return r.devices[device].PhysicalBase(), size
}

// MappingChange describes one host allocation, replacement, or removal in
// submission order. A zero Before or After denotes an absent mapping.
type MappingChange struct {
	Before, After vm.Page
	// AccessedBy requests a usable PTE on this physical GPU for After.
	AccessedBy int
}

// SetAccessedBy requests a direct mapping on gpu for every page touching the
// half-open virtual range in h's address space. A shared-PID context sees the
// same advice. The host submits advice with its other mapping changes; it
// neither moves the data nor advances simulation time. Fixed-latency and
// emulation platforms have no separate GPU page tables to populate.
func (r *Runtime) SetAccessedBy(h Context, addr Ptr, size uint64, gpu int) {
	pid := r.context(h).PID
	if gpu < 1 || gpu > r.GetNumGPUs() {
		panic("runtime: SetAccessedBy requires a physical GPU")
	}
	if size == 0 {
		return
	}
	last := uint64(addr) + (size - 1)
	if last < uint64(addr) {
		panic("runtime: SetAccessedBy address range overflows")
	}
	pageSize := uint64(1) << r.config.Log2PageSize
	var changes []MappingChange
	for va := uint64(addr) &^ (pageSize - 1); ; va += pageSize {
		page, ok := r.pageTable.Find(pid, va)
		if !ok {
			panic("runtime: SetAccessedBy range contains an unmapped page")
		}
		if r.config.GPUPageTableBytes != 0 {
			changes = append(changes, MappingChange{After: page, AccessedBy: gpu})
		}
		if last-va < pageSize {
			break
		}
	}
	r.state.MappingChanges = append(r.state.MappingChanges, changes...)
}

func (r *Runtime) mappingSnapshot(pid vm.PID, addr, size uint64) []vm.Page {
	if r.config.CPUPageTableBytes == 0 {
		return nil
	}
	var pages []vm.Page
	for offset := uint64(0); offset < size; offset += uint64(1) << r.config.Log2PageSize {
		page, ok := r.pageTable.Find(pid, addr+offset)
		if !ok {
			panic("runtime: expected mapped page")
		}
		pages = append(pages, page)
	}
	return pages
}

func (r *Runtime) recordMappingChanges(pid vm.PID, addr, size uint64, before []vm.Page, mapped bool) {
	if r.config.CPUPageTableBytes == 0 {
		return
	}
	var after []vm.Page
	if mapped {
		after = r.mappingSnapshot(pid, addr, size)
	}
	for i := range max(len(before), len(after)) {
		var change MappingChange
		if before != nil {
			change.Before = before[i]
		}
		if after != nil {
			change.After = after[i]
		}
		if change.Before != change.After {
			r.state.MappingChanges = append(r.state.MappingChanges, change)
		}
	}
}

// TakeMappingChanges transfers host-side changes to the Driver at its next tick.
func (r *Runtime) TakeMappingChanges() []MappingChange {
	changes := r.state.MappingChanges
	r.state.MappingChanges = nil
	return changes
}

// PeekCommand returns the next submission without taking ownership.
func (r *Runtime) PeekCommand(q CommandQueue) (Command, bool) {
	pending := r.queue(q).Pending
	if len(pending) == 0 {
		return Command{}, false
	}
	return pending[0], true
}
