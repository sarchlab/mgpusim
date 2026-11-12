package runtime

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

const recyclePageSize = 4096

// recycleFixture is a Runtime of two GPUs whose buddy allocators hand a freed
// page out again at once.
type recycleFixture struct {
	t     *testing.T
	r     *Runtime
	table *vm.PageTable
	ctx   Context
}

func newRecycleFixture(t *testing.T) *recycleFixture {
	t.Helper()

	s := sim.MakeBuilder().Build()
	storage := mem.MakeStorageBuilder().WithSimulation(s).WithCapacity(12 * mem.GB).Build("GlobalStorage")
	spec := DefaultRuntimeSpec()
	spec.Allocator = AllocatorBuddy
	spec.Devices = []DeviceProperties{{CUCount: 4, DRAMSize: 4 * mem.GB}, {CUCount: 4, DRAMSize: 4 * mem.GB}}
	table := vm.NewPageTable(12)
	r := NewRuntime(s, "Runtime", spec, table, storage)

	return &recycleFixture{t: t, r: r, table: table, ctx: r.Init()}
}

func (f *recycleFixture) allocate(gpu int, pages uint64) Ptr {
	f.r.SelectGPU(f.ctx, gpu)
	return f.r.AllocateMemory(f.ctx, pages*recyclePageSize)
}

func (f *recycleFixture) free(ptr Ptr) {
	f.t.Helper()

	if err := f.r.FreeMemory(f.ctx, ptr); err != nil {
		f.t.Fatal(err)
	}
}

func (f *recycleFixture) page(ptr Ptr) vm.Page {
	f.t.Helper()

	page, ok := f.table.Find(f.r.context(f.ctx).PID, uint64(ptr))
	if !ok {
		f.t.Fatalf("no page at %#x", ptr)
	}

	return page
}

func (f *recycleFixture) needsFlush(ptr Ptr, pages uint64) bool {
	return f.r.NeedsFlush(f.ctx, ptr, pages*recyclePageSize)
}

// The caches of a GPU can still hold lines of a page that was used and
// freed. A buffer that Distribute moves into such a page needs a flush before
// a host copy, like a buffer that is allocated in one.
func TestDistributeIntoAPageThatWasUsedBeforeNeedsAFlush(t *testing.T) {
	f := newRecycleFixture(t)

	// A page of GPU 2 is used and freed.
	used := f.allocate(2, 1)
	freed := f.page(used).PAddr
	f.free(used)

	// A buffer in unused pages of GPU 1 is clean.
	buffer := f.allocate(1, 2)
	if f.needsFlush(buffer, 2) {
		t.Fatal("a buffer in unused pages needs a flush")
	}

	// Its second page goes to GPU 2, into the freed page.
	f.r.Distribute(f.ctx, buffer, 2*recyclePageSize, []int{1, 2})

	if second := f.page(buffer + recyclePageSize); second.DeviceID != 2 || second.PAddr != freed {
		t.Fatalf("the second page is in %#x of device %d, want the freed page %#x of GPU 2",
			second.PAddr, second.DeviceID, freed)
	}

	if !f.needsFlush(buffer, 2) {
		t.Fatal("a buffer that was distributed into a used page needs no flush")
	}
}

// Distribute records the pages it takes, so a buffer that is allocated in
// one of them later needs a flush.
func TestAPageThatDistributeUsedIsRecycledByTheNextAllocation(t *testing.T) {
	f := newRecycleFixture(t)

	// The second page of the buffer goes to an unused page of GPU 2: nothing
	// needs a flush.
	buffer := f.allocate(1, 2)
	f.r.Distribute(f.ctx, buffer, 2*recyclePageSize, []int{1, 2})

	distributed := f.page(buffer + recyclePageSize)
	if distributed.DeviceID != 2 || f.needsFlush(buffer, 2) {
		t.Fatalf("the second page is on device %d; the buffer needs a flush: %v",
			distributed.DeviceID, f.needsFlush(buffer, 2))
	}

	f.free(buffer)

	// The next buffer of GPU 2 gets that page.
	next := f.allocate(2, 1)
	if got := f.page(next).PAddr; got != distributed.PAddr {
		t.Fatalf("the next page of GPU 2 is %#x, want %#x", got, distributed.PAddr)
	}

	if !f.needsFlush(next, 1) {
		t.Fatal("a buffer in a page that Distribute used needs no flush")
	}
}
