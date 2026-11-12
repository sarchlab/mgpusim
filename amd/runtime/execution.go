package runtime

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// QueueCount returns the number of registered streams for Driver's ordered scan.
func (r *Runtime) QueueCount() int { return len(r.state.Queues) }

// QueueContext returns the address-space handle used by an executing command.
func (r *Runtime) QueueContext(q CommandQueue) Context { return r.queue(q).Context }

// QueuePID returns the process identity used in Driver's protocol requests.
func (r *Runtime) QueuePID(q CommandQueue) vm.PID { return r.context(r.QueueContext(q)).PID }

// DeviceCUCount returns immutable dispatch metadata for a physical GPU.
func (r *Runtime) DeviceCUCount(gpu int) int { return r.config.Devices[gpu-1].CUCount }

// DeviceIDByPAddr resolves the physical owner for Driver's copy requests.
func (r *Runtime) DeviceIDByPAddr(addr uint64) int { return r.allocator.GetDeviceIDByPAddr(addr) }

// AllocateFrame takes a physical page on GPU gpu for a page that migrates
// there; the caller maps it in the page table. The frame comes from the
// ordinary allocator, so it may be one that FreeMemory returned: the caches
// of the GPU can still hold lines of it. The frame that the page leaves is
// retired: nothing returns it to the allocator.
func (r *Runtime) AllocateFrame(gpu int) uint64 {
	frame := r.allocator.AllocateFrame(gpu)
	r.recycledFrame(gpu, frame)
	return frame
}

// sharedBuffers calls visit with every allocation in the address space of a
// process. The contexts made with InitWithExistingPID share one address
// space: each owns, and frees, the allocations it made, but a kernel or a
// host copy that is submitted through any of them uses the memory of all.
// What the caches hold of an allocation therefore belongs to the address
// space, not to the context that made the allocation.
func (r *Runtime) sharedBuffers(pid vm.PID, visit func(b *buffer)) {
	for i := range r.state.Contexts {
		c := &r.state.Contexts[i]
		if c.PID != pid {
			continue
		}
		for j := range c.Buffers {
			visit(&c.Buffers[j])
		}
	}
}

// markRangeDirty marks the allocations of an address space that have a byte
// of a range dirty.
func (r *Runtime) markRangeDirty(pid vm.PID, addr, size uint64) {
	r.sharedBuffers(pid, func(b *buffer) {
		if b.overlaps(addr, size) {
			b.Dirty = true
		}
	})
}

// MarkBuffersDirty records the effects of a submitted kernel: it may have
// written any allocation in the address space of its context.
func (r *Runtime) MarkBuffersDirty(ctx Context) {
	r.sharedBuffers(r.context(ctx).PID, func(b *buffer) { b.Dirty = true })
}

// MarkPageMigrated records that a page of a process has migrated to another
// GPU. The page is in the memory below the caches of that GPU now, which a
// host copy that goes around the GPUs does not see, so the allocations that
// have the page are dirty from now on.
func (r *Runtime) MarkPageMigrated(pid vm.PID, addr, size uint64) {
	r.markRangeDirty(pid, addr, size)
}

// NeedsFlush reports whether a Driver copy overlaps a dirty allocation: one
// that a kernel may have written, that got a page that was used before, or
// that has a page that migrated. The copy may come through any context of
// the address space that has the allocation.
func (r *Runtime) NeedsFlush(ctx Context, addr Ptr, size uint64) bool {
	dirty := false
	r.sharedBuffers(r.context(ctx).PID, func(b *buffer) {
		dirty = dirty || b.Dirty && b.overlaps(uint64(addr), size)
	})
	return dirty
}

// CompleteEvents triggers consecutive markers at the current Driver boundary.
// It does not advance time or take ownership of the next non-marker command.
func (r *Runtime) CompleteEvents(q CommandQueue, now timing.VTimeInPicoSec) {
	queue := r.queue(q)
	for len(queue.Pending) > 0 && queue.Pending[0].Kind == RecordEvent {
		cmd, _ := r.TakeCommand(q)
		r.CompleteCommand(q, cmd, now)
	}
}

// SimulationTask returns the saved parent task for Driver command tracing.
func (r *Runtime) SimulationTask() uint64 { return r.state.SimulationTask }

// CheckMemory validates Driver construction without exposing mutable resources.
func (r *Runtime) CheckMemory(table *vm.PageTable, storage *mem.Storage) error {
	if r.pageTable != table {
		return fmt.Errorf("driver: Runtime and component page tables differ")
	}
	if r.storage != storage {
		return fmt.Errorf("driver: Runtime and component storage differ")
	}
	return nil
}
