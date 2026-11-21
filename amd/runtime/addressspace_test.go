package runtime

import (
	"testing"
)

// addressSpaceFixture has two contexts of one address space: the owner, which
// makes the allocations, and a sibling made with InitWithExistingPID. A
// stranger has an address space of its own, with a clean allocation at the
// virtual addresses that the tests use in the other one.
type addressSpaceFixture struct {
	*recycleFixture
	owner, sibling, stranger Context
}

func newAddressSpaceFixture(t *testing.T) *addressSpaceFixture {
	t.Helper()

	f := &addressSpaceFixture{recycleFixture: newRecycleFixture(t)}
	f.owner = f.ctx
	f.sibling = f.r.InitWithExistingPID(f.owner)
	f.stranger = f.r.Init()

	// Every address space starts at the same virtual address.
	f.r.SelectGPU(f.stranger, 1)
	f.r.AllocateMemory(f.stranger, 8*recyclePageSize)

	return f
}

// mustNeedFlush checks whether a host copy of pages needs a flush. The
// contexts of the address space agree, whichever of them made the
// allocation; the stranger, whose allocation at the same addresses is clean,
// never needs one.
func (f *addressSpaceFixture) mustNeedFlush(ptr Ptr, pages uint64, want bool) {
	f.t.Helper()

	size := pages * recyclePageSize

	if got := f.r.NeedsFlush(f.owner, ptr, size); got != want {
		f.t.Fatalf("a copy through the owner needs a flush: %v, want %v", got, want)
	}

	if got := f.r.NeedsFlush(f.sibling, ptr, size); got != want {
		f.t.Fatalf("a copy through the sibling needs a flush: %v, want %v", got, want)
	}

	if f.r.NeedsFlush(f.stranger, ptr, size) {
		f.t.Fatal("a copy in another address space needs a flush")
	}
}

// recycledFrameOfGPU2 uses and frees a page of GPU 2, which its allocator
// hands out next, and returns the frame.
func (f *addressSpaceFixture) recycledFrameOfGPU2() uint64 {
	f.t.Helper()

	used := f.allocate(2, 1)
	frame := f.page(used).PAddr
	f.free(used)

	return frame
}

// A kernel can write every allocation of its address space, whichever
// context it is submitted through and whichever context made the allocation.
func TestAKernelDirtiesTheAllocationsOfEveryContextOfItsAddressSpace(t *testing.T) {
	f := newAddressSpaceFixture(t)
	buffer := f.allocate(1, 1)
	f.mustNeedFlush(buffer, 1, false)

	f.r.MarkBuffersDirty(f.sibling)
	f.mustNeedFlush(buffer, 1, true)

	// A kernel of the other address space writes none of them.
	clean := f.allocate(1, 1)
	f.r.MarkBuffersDirty(f.stranger)

	if f.r.NeedsFlush(f.owner, clean, recyclePageSize) || f.r.NeedsFlush(f.sibling, clean, recyclePageSize) {
		t.Fatal("a kernel of another address space dirtied an allocation")
	}
}

// An allocation in a page that was used before is dirty for a copy through
// any context of its address space.
func TestARecycledAllocationNeedsAFlushThroughEveryContext(t *testing.T) {
	f := newAddressSpaceFixture(t)
	frame := f.recycledFrameOfGPU2()

	buffer := f.allocate(2, 1)
	if got := f.page(buffer).PAddr; got != frame {
		t.Fatalf("the buffer is in frame %#x, want the recycled frame %#x", got, frame)
	}

	f.mustNeedFlush(buffer, 1, true)
}

// Remap and Distribute through one context dirty the allocation that another
// context of the address space made, when a page of it moves into a page
// that was used before. The destination is GPU 2, or a unified GPU that
// takes its first frames from GPU 2: the page is on GPU 2 either way, and it
// is the history of the frames of GPU 2 that tells that the page was used.
func TestRemapAndDistributeThroughASiblingDirtyTheAllocationOfTheOwner(t *testing.T) {
	unified := func(f *addressSpaceFixture) int {
		return f.r.CreateUnifiedGPU(f.owner, []int{2, 1})
	}

	moves := map[string]func(f *addressSpaceFixture, buffer Ptr){
		"remap": func(f *addressSpaceFixture, buffer Ptr) {
			f.r.Remap(f.sibling, uint64(buffer)+recyclePageSize, recyclePageSize, 2)
		},
		"distribute": func(f *addressSpaceFixture, buffer Ptr) {
			f.r.Distribute(f.sibling, buffer, 2*recyclePageSize, []int{1, 2})
		},
		"remap to a unified GPU": func(f *addressSpaceFixture, buffer Ptr) {
			f.r.Remap(f.sibling, uint64(buffer)+recyclePageSize, recyclePageSize, unified(f))
		},
		"distribute over a unified GPU": func(f *addressSpaceFixture, buffer Ptr) {
			f.r.Distribute(f.sibling, buffer, 2*recyclePageSize, []int{1, unified(f)})
		},
	}

	for name, move := range moves {
		t.Run(name, func(t *testing.T) {
			f := newAddressSpaceFixture(t)
			frame := f.recycledFrameOfGPU2()

			buffer := f.allocate(1, 2)
			f.mustNeedFlush(buffer, 2, false)

			move(f, buffer)

			if second := f.page(buffer + recyclePageSize); second.DeviceID != 2 || second.PAddr != frame {
				t.Fatalf("the second page is in %#x of device %d, want the recycled frame %#x of GPU 2",
					second.PAddr, second.DeviceID, frame)
			}

			f.mustNeedFlush(buffer, 2, true)
		})
	}
}

// The contexts share what the caches hold of an allocation, not the
// allocation: only the context that made it frees it, and with it goes its
// dirty record.
func TestOnlyTheContextThatMadeAnAllocationFreesIt(t *testing.T) {
	f := newAddressSpaceFixture(t)
	buffer := f.allocate(1, 1)
	f.r.MarkBuffersDirty(f.owner)

	if err := f.r.FreeMemory(f.sibling, buffer); err == nil {
		t.Fatal("a sibling freed an allocation of the owner")
	}

	f.mustNeedFlush(buffer, 1, true)

	if len(f.r.context(f.owner).Buffers) != 1 || len(f.r.context(f.sibling).Buffers) != 0 {
		t.Fatalf("allocations: owner %+v, sibling %+v",
			f.r.context(f.owner).Buffers, f.r.context(f.sibling).Buffers)
	}

	f.free(buffer)
	f.mustNeedFlush(buffer, 1, false)

	if _, mapped := f.table.Find(f.r.context(f.owner).PID, uint64(buffer)); mapped {
		t.Fatal("the freed page is still mapped")
	}
}
