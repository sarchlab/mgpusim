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

// MarkBuffersDirty records the effects of a submitted kernel in its context.
func (r *Runtime) MarkBuffersDirty(ctx Context) {
	buffers := r.context(ctx).Buffers
	for i := range buffers {
		buffers[i].Dirty = true
	}
}

// NeedsFlush reports whether a Driver copy overlaps a kernel-dirtied allocation.
func (r *Runtime) NeedsFlush(ctx Context, addr Ptr, size uint64) bool {
	for _, b := range r.context(ctx).Buffers {
		if b.Dirty && uint64(b.Address) < uint64(addr)+size && uint64(addr) < uint64(b.Address)+b.Size {
			return true
		}
	}
	return false
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
