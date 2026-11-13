package internal

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
)

// newReuseAllocator registers a CPU and two GPUs of 64 pages each.
func newReuseAllocator(
	t *testing.T,
	kind AllocatorType,
	log2PageSize uint64,
) (*memoryAllocatorImpl, *vm.PageTable) {
	t.Helper()

	table := vm.NewPageTable(log2PageSize)
	a := NewMemoryAllocator(table, log2PageSize).(*memoryAllocatorImpl)

	for id, deviceType := range []DeviceType{DeviceTypeCPU, DeviceTypeGPU, DeviceTypeGPU} {
		d := &Device{ID: id, Type: deviceType, MemState: NewDeviceMemoryState(log2PageSize, kind)}
		d.SetTotalMemSize(64 << log2PageSize)
		a.RegisterDevice(d)
	}

	return a, table
}

var allocatorKinds = []struct {
	name string
	kind AllocatorType
}{
	{"free list", AllocatorTypeDefault},
	{"buddy", AllocatorTypeBuddy},
}

// Both allocators hand a freed page out again: the free list after its
// unused pages, the buddy allocator at once.
func TestFreedPagesAreAllocatedAgain(t *testing.T) {
	for _, k := range allocatorKinds {
		t.Run(k.name, func(t *testing.T) {
			a, table := newReuseAllocator(t, k.kind, 12)

			first := a.Allocate(1, 4096, 1)
			freed, _ := table.Find(1, first)
			a.Free(1, first, 4096)

			seen := false
			for range 64 {
				seen = seen || a.AllocateFrame(1) == freed.PAddr
			}

			if !seen {
				t.Fatalf("page %#x was never allocated again", freed.PAddr)
			}
		})
	}
}

// A device never hands out a page that is in use, whatever the order of
// allocations and frees. The buddy allocator used to merge a freed block
// with a buddy that was still allocated when the block had been split from a
// larger one, which gave every page of the device out again.
func TestNoPageIsAllocatedTwice(t *testing.T) {
	for _, k := range allocatorKinds {
		for _, log2PageSize := range []uint64{12, 21} {
			t.Run(fmt.Sprintf("%s, 2^%d-byte pages", k.name, log2PageSize), func(t *testing.T) {
				a, table := newReuseAllocator(t, k.kind, log2PageSize)
				churn(t, a, table, uint64(1)<<log2PageSize)
			})
		}
	}
}

// churn allocates and frees single pages of GPU 1 in a random order, with up
// to 60 of its 64 pages in use, and checks every page that is handed out.
func churn(t *testing.T, a *memoryAllocatorImpl, table *vm.PageTable, pageSize uint64) {
	t.Helper()

	rng := rand.New(rand.NewSource(338))
	inUse := map[uint64]bool{}

	var live []uint64

	for step := range 2000 {
		if len(live) > 0 && (len(live) == 60 || rng.Intn(2) == 0) {
			i := rng.Intn(len(live))
			page, _ := table.Find(1, live[i])
			delete(inUse, page.PAddr)
			a.Free(1, live[i], pageSize)
			live = append(live[:i], live[i+1:]...)

			continue
		}

		vAddr := a.Allocate(1, pageSize, 1)
		page, _ := table.Find(1, vAddr)

		if inUse[page.PAddr] {
			t.Fatalf("step %d: page %#x is allocated twice", step, page.PAddr)
		}

		if a.deviceIDByPAddr(page.PAddr) != 1 || page.PAddr%pageSize != 0 {
			t.Fatalf("step %d: page %#x is not a page of the device", step, page.PAddr)
		}

		inUse[page.PAddr] = true
		live = append(live, vAddr)
	}
}

// AllocateFrame takes a page of the device without mapping it; Free returns
// the page that a virtual page maps when it is freed, so a frame that a
// migration left behind stays out of use.
func TestAFrameThatAPageLeftIsNotAllocatedAgain(t *testing.T) {
	for _, k := range allocatorKinds {
		t.Run(k.name, func(t *testing.T) {
			a, table := newReuseAllocator(t, k.kind, 12)

			vAddr := a.Allocate(1, 4096, 1)
			page, _ := table.Find(1, vAddr)
			left := page.PAddr

			// The page migrates to GPU 2: a new frame, mapped by the caller.
			page.PAddr = a.AllocateFrame(2)
			page.DeviceID = 2
			table.Update(page)

			if a.deviceIDByPAddr(page.PAddr) != 2 {
				t.Fatalf("frame %#x is not on GPU 2", page.PAddr)
			}

			a.Free(1, vAddr, 4096)

			// Every page of GPU 1 that is left.
			for range 63 {
				if a.AllocateFrame(1) == left {
					t.Fatalf("retired frame %#x was allocated again", left)
				}
			}
		})
	}
}
