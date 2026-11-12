package internal

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
)

// The platforms put device i of 64 pages at [i*64, (i+1)*64) pages: that is
// where its hardware finds its memory, and where the other devices route to
// it. A device hands out exactly the pages of that range, the first and the
// last included, except the page at address 0, which is never handed out.
func TestADeviceHandsOutThePagesOfItsRange(t *testing.T) {
	for _, k := range allocatorKinds {
		for _, log2PageSize := range []uint64{12, 21} {
			t.Run(fmt.Sprintf("%s, 2^%d-byte pages", k.name, log2PageSize), func(t *testing.T) {
				a, _ := newReuseAllocator(t, k.kind, log2PageSize)

				for id := range 3 {
					mustHandOutItsRange(t, a, id, uint64(1)<<log2PageSize)
				}
			})
		}
	}
}

// mustHandOutItsRange takes every page of a device of 64 pages.
func mustHandOutItsRange(t *testing.T, a *memoryAllocatorImpl, id int, pageSize uint64) {
	t.Helper()

	start := uint64(id) * 64 * pageSize
	end := start + 64*pageSize
	first := start

	if id == 0 {
		first = pageSize
	}

	seen := map[uint64]bool{}

	for !a.devices[id].MemState.noAvailablePAddrs() {
		frame := a.AllocateFrame(id)

		if frame < first || frame >= end || frame%pageSize != 0 || seen[frame] {
			t.Fatalf("device %d hands out %#x; its pages are [%#x, %#x)", id, frame, first, end)
		}

		if owner := a.deviceIDByPAddr(frame); owner != id {
			t.Fatalf("page %#x of device %d is found on device %d", frame, id, owner)
		}

		seen[frame] = true
	}

	if want := int((end - first) / pageSize); len(seen) != want {
		t.Fatalf("device %d handed out %d pages, want %d", id, len(seen), want)
	}

	if !seen[first] || !seen[end-pageSize] {
		t.Fatalf("device %d did not hand out its first page %#x and its last page %#x",
			id, first, end-pageSize)
	}
}

// A unified GPU has no memory of its own: it takes frames from its members.
// A page that is mapped through it is on the member that has its frame,
// however the page is mapped.
func TestAPageOfAUnifiedGPUIsOnTheGPUThatHasItsFrame(t *testing.T) {
	const pageSize = 4096

	for _, k := range allocatorKinds {
		t.Run(k.name, func(t *testing.T) {
			a, table := newReuseAllocator(t, k.kind, 12)
			a.RegisterDevice(&Device{
				ID:            3,
				Type:          DeviceTypeUnifiedGPU,
				UnifiedGPUIDs: []int{1, 2},
				ActualGPUs:    []*Device{a.devices[1], a.devices[2]},
				MemState:      NewDeviceMemoryState(12, k.kind),
			})

			// Four pages of GPU 1: the first stays, the second is mapped
			// again alone, the last two together, all through the unified
			// GPU.
			first := a.Allocate(1, 4*pageSize, 1)
			a.AllocatePageWithGivenVAddr(1, 3, first+pageSize, false)
			a.Remap(1, first+2*pageSize, 2*pageSize, 3)

			for i := range uint64(4) {
				page, _ := table.Find(1, first+i*pageSize)
				owner := a.deviceIDByPAddr(page.PAddr)

				if owner != 1 && owner != 2 || page.DeviceID != uint64(owner) {
					t.Fatalf("page %d in frame %#x of device %d is recorded on device %d",
						i, page.PAddr, owner, page.DeviceID)
				}
			}
		})
	}
}

// The page at address 0 stays out of use after a checkpoint is restored.
func TestTheZeroPageStaysReservedAfterRestore(t *testing.T) {
	for _, k := range allocatorKinds {
		t.Run(k.name, func(t *testing.T) {
			a, _ := newReuseAllocator(t, k.kind, 12)
			devices := []*Device{a.devices[0], a.devices[1], a.devices[2]}

			restored, _, err := RestoreAllocator(SnapshotAllocator(a, devices), vm.NewPageTable(12))
			if err != nil {
				t.Fatal(err)
			}

			mustHandOutItsRange(t, restored.(*memoryAllocatorImpl), 0, 4096)
		})
	}
}
