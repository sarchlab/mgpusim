package runtime

import (
	"bytes"
	"slices"
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

// frames returns the physical pages of the pages of a buffer.
func (f *recycleFixture) frames(ptr Ptr, pages uint64) []uint64 {
	f.t.Helper()

	frames := make([]uint64, pages)
	for i := range frames {
		frames[i] = f.page(ptr + Ptr(uint64(i)*recyclePageSize)).PAddr
	}

	return frames
}

// gapOnGPU2 leaves a page of GPU 2 unused below pages that are in use, with
// nothing freed, and returns that page. GPU 2 hands its first page out, and
// then an aligned block of two pages for a buffer that is remapped from
// GPU 1: the page between them stays free.
func (f *recycleFixture) gapOnGPU2() uint64 {
	f.t.Helper()

	first := f.page(f.allocate(2, 1)).PAddr
	buffer := f.allocate(1, 2)
	f.r.Remap(f.ctx, uint64(buffer), 2*recyclePageSize, 2)

	want := []uint64{first + 2*recyclePageSize, first + 3*recyclePageSize}
	if got := f.frames(buffer, 2); !slices.Equal(got, want) {
		f.t.Fatalf("the remapped buffer is in frames %#x, want %#x; the test needs a gap below it", got, want)
	}

	if f.needsFlush(buffer, 2) {
		f.t.Fatal("a buffer that was remapped into unused pages needs a flush")
	}

	return first + recyclePageSize
}

// A page is recycled only if it was freed. The buddy allocator does not hand
// its unused pages out in address order: a page can be used for the first
// time below pages that are in use. A buffer in such a page is clean, and it
// is recycled, like any other page, once it has been freed.
func TestAPageThatIsUsedForTheFirstTimeBelowUsedPagesNeedsNoFlush(t *testing.T) {
	f := newRecycleFixture(t)
	gap := f.gapOnGPU2()

	first := f.allocate(2, 1)
	if got := f.page(first).PAddr; got != gap {
		t.Fatalf("the next page of GPU 2 is %#x, want the unused page %#x", got, gap)
	}

	if f.needsFlush(first, 1) {
		t.Fatal("a buffer in a page that is used for the first time needs a flush")
	}

	f.free(first)

	again := f.allocate(2, 1)
	if got := f.page(again).PAddr; got != gap {
		t.Fatalf("the next page of GPU 2 is %#x, want the freed page %#x", got, gap)
	}

	if !f.needsFlush(again, 1) {
		t.Fatal("a buffer in a page that was used and freed needs no flush")
	}
}

// The record of the freed pages has the pages that FreeMemory returned to a
// GPU and that are still free. Every way a page gets a physical page takes
// the page off the record: an allocation, Remap, Distribute, and a migration.
func TestTakingAFreedPageUsesItsRecordUp(t *testing.T) {
	takers := []struct {
		name string
		take func(f *recycleFixture)
	}{
		{"allocation", func(f *recycleFixture) { f.allocate(2, 1) }},
		{"remap", func(f *recycleFixture) {
			f.r.Remap(f.ctx, uint64(f.allocate(1, 1)), recyclePageSize, 2)
		}},
		{"distribute", func(f *recycleFixture) {
			f.r.Distribute(f.ctx, f.allocate(1, 2), 2*recyclePageSize, []int{1, 2})
		}},
		{"migration", func(f *recycleFixture) { f.r.AllocateFrame(2) }},
	}

	for _, taker := range takers {
		t.Run(taker.name, func(t *testing.T) {
			f := newRecycleFixture(t)

			used := f.allocate(2, 1)
			freed := f.page(used).PAddr
			f.free(used)

			if got := f.r.state.FreedFrames; len(got) != 1 || !got[freed] {
				t.Fatalf("the record of the freed pages is %v, want the page %#x", got, freed)
			}

			taker.take(f)

			if got := f.r.state.FreedFrames; len(got) != 0 {
				t.Fatalf("the record of the freed pages still has %v", got)
			}
		})
	}
}

// The host has no cache in front of its memory, and the driver frees a host
// buffer after every upload that it stages: the pages of the host are not
// recorded.
func TestTheFreedPagesOfTheHostAreNotRecorded(t *testing.T) {
	f := newRecycleFixture(t)

	host := f.r.AllocateHostBuffer(f.r.CreateCommandQueue(f.ctx), recyclePageSize)
	f.free(host.Address)

	if got := f.r.state.FreedFrames; len(got) != 0 {
		t.Fatalf("the record of the freed pages has the pages of the host: %v", got)
	}
}

// Which pages were freed is part of the state of the runtime: after a
// restore in another runtime, a page that was freed before the checkpoint is
// recycled by the allocation that takes it, once, and a page that was never
// used is not.
func TestTheFreedPagesSurviveACheckpoint(t *testing.T) {
	f := newRecycleFixture(t)
	gap := f.gapOnGPU2()
	f.free(f.allocate(2, 1))

	var saved, savedTable bytes.Buffer
	if err := f.r.SaveCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}

	if err := f.table.SaveCheckpoint(&savedTable); err != nil {
		t.Fatal(err)
	}

	restored := newRecycleFixture(t)
	if err := restored.table.LoadCheckpoint(&savedTable); err != nil {
		t.Fatal(err)
	}

	if err := restored.r.LoadCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}

	recycled := restored.allocate(2, 1)
	if got := restored.page(recycled).PAddr; got != gap {
		t.Fatalf("the next page of GPU 2 is %#x, want the freed page %#x", got, gap)
	}

	if !restored.needsFlush(recycled, 1) {
		t.Fatal("after the restore, a buffer in a page that was freed needs no flush")
	}

	// The record is used up, and the pages that follow are new.
	unused := restored.allocate(2, 1)
	if restored.page(unused).PAddr == gap || restored.needsFlush(unused, 1) {
		t.Fatalf("after the restore, a buffer in the unused page %#x needs a flush", restored.page(unused).PAddr)
	}
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
