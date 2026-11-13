package driver

import (
	"encoding/binary"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// newCopyFixture builds the driver of a timing platform, where the GPUs keep
// device memory in caches, with magic or with timed memory copies.
func newCopyFixture(
	t *testing.T,
	magic bool,
	configureRuntime func(*gpuruntime.RuntimeSpec),
) *migrationFixture {
	t.Helper()

	f := newRuntimeConfiguredFixture(t, false, func(s *Spec) {
		s.UseMagicMemoryCopy = magic
		noPrefetch(&s.PageMigration)
	}, configureRuntime)

	return &migrationFixture{fixture: f, t: t}
}

func defaultRuntime(*gpuruntime.RuntimeSpec) {}

func buddyRuntime(s *gpuruntime.RuntimeSpec) { s.Allocator = gpuruntime.AllocatorBuddy }

// h2d queues a copy of a value from the host to the start of a page.
func (f *migrationFixture) h2d(q gpuruntime.CommandQueue, page vm.Page, value uint32) {
	f.t.Helper()

	host := f.r.AllocateHostBuffer(q, 4)
	if err := f.r.WriteHost(host, value); err != nil {
		f.t.Fatal(err)
	}

	f.r.EnqueueMemCopyH2D(q, gpuruntime.Ptr(page.VAddr), host.Address, 4)
}

// d2h queues a copy of the value at the start of a page to the host and
// returns where it arrives.
func (f *migrationFixture) d2h(q gpuruntime.CommandQueue, page vm.Page) gpuruntime.HostBuffer {
	host := f.r.AllocateHostBuffer(q, 4)
	f.r.EnqueueMemCopyD2H(q, host.Address, gpuruntime.Ptr(page.VAddr), 4)

	return host
}

// stored returns the value at the start of a page frame in the global
// storage.
func (f *migrationFixture) stored(pAddr uint64) uint32 {
	return binary.LittleEndian.Uint32(f.c.Resources.GlobalStorage.Read(pAddr, 4))
}

// requests prints the requests of a command that are not answered yet as
// "<what>><GPU>", for example "MemCopyH2D>2".
func (f *migrationFixture) requests(e Execution) []string {
	return describe(e.Requests)
}

// A magic copy completes in the tick that starts it and leaves the
// execution list. The copies that waited behind it still get their tick:
// nothing else wakes the driver once the pages are idle.
func TestWaitingMagicCopiesAllCompleteOnceTheirPagesAreIdle(t *testing.T) {
	f := newCopyFixture(t, true, defaultRuntime)
	pages := f.unified(2)
	second := f.r.CreateCommandQueue(f.ctx)

	// GPU 1 faults on two pages that it has: each is held and released where
	// it is, and no copy may start meanwhile.
	f.fault(pages[0], 1)
	f.fault(pages[1], 1)
	f.h2d(f.q, pages[0], 0x11111111)
	f.h2d(second, pages[1], 0x22222222)
	f.run()

	if e := f.c.State.Executing; len(e) != 2 || !e[0].WaitsForMigration || !e[1].WaitsForMigration {
		t.Fatalf("the copies do not wait for their pages: %+v", e)
	}

	// Both pages are released.
	f.finishAll()
	f.run()

	mustEqual(t, "unfinished commands", f.r.Unfinished(), []uint64{0, 0})
	mustEqual(t, "first page", f.stored(pages[0].PAddr), uint32(0x11111111))
	mustEqual(t, "second page", f.stored(pages[1].PAddr), uint32(0x22222222))
}

// A copy that still waits for its page does not hold back the copy of
// another queue whose page has become idle.
func TestAWaitingCopyDoesNotHoldBackTheCopyOfAnotherQueue(t *testing.T) {
	f := newMigrationFixture(t, noPrefetch)
	pages := f.unified(2)
	second := f.r.CreateCommandQueue(f.ctx)

	// The second page faults first, so its window is the first at the data
	// mover of GPU 2. The first queue copies into the first page.
	f.fault(pages[1], 2)
	f.fault(pages[0], 2)
	f.h2d(f.q, pages[0], 1)
	f.h2d(second, pages[1], 2)
	f.run()

	if e := f.c.State.Executing; len(e) != 2 || !e[0].WaitsForMigration || !e[1].WaitsForMigration {
		t.Fatalf("the copies do not wait for their pages: %+v", e)
	}

	// The second page arrives on GPU 2: its copy starts there, through the
	// GPU that has the page now, while the copy of the first queue, which is
	// ahead of it in the driver, keeps waiting.
	f.finish(pages[1])
	f.run()

	mustEqual(t, "owners", []uint64{f.page(pages[0].VAddr).DeviceID, f.page(pages[1].VAddr).DeviceID},
		[]uint64{1, 2})

	e := f.c.State.Executing
	if len(e) != 2 || !e[0].WaitsForMigration || e[1].WaitsForMigration {
		t.Fatalf("after the second page: %+v", e)
	}

	mustEqual(t, "requests of the second copy", f.requests(e[1]), []string{"MemCopyH2D>2"})

	// The first copy starts when its page is idle.
	f.finish(pages[0])
	f.run()

	e = f.c.State.Executing
	if len(e) != 2 || e[0].WaitsForMigration {
		t.Fatalf("after the first page: %+v", e)
	}

	mustEqual(t, "requests of the first copy", f.requests(e[0]), []string{"MemCopyH2D>2"})
}

// The faults of a page that a memory copy uses wait for the copy. They do
// not hold back the faults behind them, and they are serviced when the copy
// ends.
func TestFaultsBehindAFaultThatWaitsForAMemoryCopyAreServiced(t *testing.T) {
	f := newMigrationFixture(t, func(s *PageMigrationSpec) {
		noPrefetch(s)
		s.BatchSize = 1
	})
	pages := f.unified(3)

	// A copy from the first page is in flight.
	f.running(gpuruntime.Command{
		Kind: gpuruntime.CopyD2H, Src: gpuruntime.Ptr(pages[0].VAddr), ByteSize: 64,
	}, []messaging.Msg{f.request(protocol.MemCopyD2HReq{})})

	for _, p := range pages {
		f.deliverFault(p, 2)
	}

	// The driver comes to rest with the other two pages held, one batch
	// after the other. The first page keeps its fault and its place.
	f.ticks()

	mustEqual(t, "first page", describe(f.of(pages[0])), []string(nil))
	mustEqual(t, "second page", describe(f.of(pages[1])), []string{"Hold>1", "Hold>2"})
	mustEqual(t, "third page", describe(f.of(pages[2])), []string{"Hold>1", "Hold>2"})

	s := f.c.State.PageMigration
	if len(s.Queue) != 1 || s.Queue[0].Page != 0 || !s.Pages[0].Queued ||
		len(s.Pages[0].Waiters) != 1 || s.Stats.DeferredFaults != 1 || s.Stats.Batches != 2 {
		t.Fatalf("queue %+v, first page %+v, stats %+v", s.Queue, s.Pages[0], s.Stats)
	}

	// The copy ends: the fault that waited is serviced.
	f.c.State.Executing = nil
	f.ticks()

	mustEqual(t, "first page", describe(f.of(pages[0])), []string{"Hold>1", "Hold>2"})
	mustEqual(t, "requests that wait", len(f.c.State.PageMigration.Queue), 0)
}

// The queue is finite, and a fault that waits for a memory copy keeps its
// entry. With a queue of one entry, the fault of another page is not taken
// in while that entry is not free: it stays in the port, where it is not
// lost, and is serviced once the copy has ended.
func TestAFaultStaysInThePortWhileAFaultThatWaitsForACopyFillsTheQueue(t *testing.T) {
	f := newMigrationFixture(t, func(s *PageMigrationSpec) {
		noPrefetch(s)
		s.QueueSize = 1
	})
	pages := f.unified(2)
	a, b := pages[0], pages[1]

	// A copy from page A is in flight.
	f.running(gpuruntime.Command{
		Kind: gpuruntime.CopyD2H, Src: gpuruntime.Ptr(a.VAddr), ByteSize: 64,
	}, []messaging.Msg{f.request(protocol.MemCopyD2HReq{})})

	f.deliverFault(a, 2)
	f.deliverFault(b, 2)

	// The driver comes to rest: the fault on A has the entry and waits for
	// the copy, and the fault on B is still in the port.
	f.ticks()

	s := f.c.State.PageMigration
	if len(s.Queue) != 1 || !s.Queue[0].Deferred || s.Pages[s.Queue[0].Page].VAddr != a.VAddr ||
		s.Stats.Faults != 1 || s.Stats.DeferredFaults != 1 || s.Stats.QueueFullWaits == 0 {
		t.Fatalf("queue %+v, stats %+v", s.Queue, s.Stats)
	}

	mustEqual(t, "faults in the port", f.pn.IncomingBuf().Size(), 1)
	mustEqual(t, "commands", describe(f.outstanding()), []string(nil))

	// The copy ends: the fault on A is serviced, which frees the entry for
	// the fault on B.
	f.c.State.Executing = nil
	f.ticks()

	mustEqual(t, "faults in the port", f.pn.IncomingBuf().Size(), 0)
	mustEqual(t, "page A", describe(f.of(a)), []string{"Hold>1", "Hold>2"})
	mustEqual(t, "page B", describe(f.of(b)), []string{"Hold>1", "Hold>2"})

	// Both pages arrive on GPU 2.
	f.finishAll()

	mustEqual(t, "owner of A", f.page(a.VAddr).DeviceID, uint64(2))
	mustEqual(t, "owner of B", f.page(b.VAddr).DeviceID, uint64(2))

	if st := f.stats(); st.Faults != 2 || st.Migrations != 2 || st.DeferredFaults != 1 {
		t.Fatalf("stats %+v", st)
	}
}

// Two processes can have a page at the same virtual address. A process ID
// has 32 bits: the two processes here differ only in bit 20.
func TestPagesOfTwoProcessesAtTheSameAddressAreServicedOnTheirOwn(t *testing.T) {
	f := newMigrationFixture(t, noPrefetch)
	first := f.unified(1)[0]

	second := first
	second.PID = first.PID + 1<<20
	second.PAddr = f.r.AllocateFrame(1)
	f.p.Insert(second)

	f.deliverFault(second, 2)
	f.deliverFault(first, 2)
	f.ticks()

	mustEqual(t, "first process", describe(f.of(first)), []string{"Hold>1", "Hold>2"})
	mustEqual(t, "second process", describe(f.of(second)), []string{"Hold>1", "Hold>2"})

	for _, msg := range f.of(second) {
		mustEqual(t, "process that holds", msg.Payload.(protocol.PageHoldReq).PID, second.PID)
	}

	if s := f.stats(); s.Faults != 2 || s.CoalescedFaults != 0 || s.Windows != 2 {
		t.Fatalf("stats %+v", s)
	}

	// The page of the second process, whose window is the first, migrates
	// and is released while the page of the first process is still held.
	f.finish(second)

	moved, _ := f.p.Find(second.PID, second.VAddr)
	mustEqual(t, "owner in the second process", moved.DeviceID, uint64(2))
	mustEqual(t, "owner in the first process", f.page(first.VAddr).DeviceID, uint64(1))
	mustEqual(t, "first process", describe(f.of(first)), []string{"Hold>1", "Hold>2"})

	f.finish(first)

	mustEqual(t, "owner in the first process", f.page(first.VAddr).DeviceID, uint64(2))

	if s := f.stats(); s.Migrations != 2 || len(f.outstanding()) != 0 {
		t.Fatalf("stats %+v, outstanding %v", s, describe(f.outstanding()))
	}
}

// dirtyPages are the ways a page comes to need a flush of the caches before a
// host copy that goes through the GPUs.
var dirtyPages = []struct {
	name string
	make func(f *migrationFixture) vm.Page
}{
	{"memory that a kernel may have written", func(f *migrationFixture) vm.Page {
		page := f.unified(1)[0]
		f.r.MarkBuffersDirty(f.ctx)

		return page
	}},
	{"a page that was used before", func(f *migrationFixture) vm.Page {
		used := f.r.AllocateMemory(f.ctx, testPageSize)
		frame := f.page(uint64(used)).PAddr

		if err := f.r.FreeMemory(f.ctx, used); err != nil {
			f.t.Fatal(err)
		}

		page := f.unified(1)[0]
		mustEqual(f.t, "frame of the page", page.PAddr, frame)

		return page
	}},
}

// A timed copy goes through the GPUs. When the caches may hold lines of the
// memory, they are flushed first, and the GPU of the page then writes the
// memory below them.
func TestATimedCopyOfDirtyMemoryFlushesTheCaches(t *testing.T) {
	for _, dirty := range dirtyPages {
		t.Run(dirty.name, func(t *testing.T) {
			f := newCopyFixture(t, false, buddyRuntime)
			page := dirty.make(f)

			f.h2d(f.q, page, 0x12345678)
			f.run()

			e := f.c.State.Executing
			if len(e) != 1 {
				t.Fatalf("the copy is not in flight: %+v", e)
			}

			mustEqual(t, "requests", f.requests(e[0]), []string{"Flush>1", "Flush>2", "MemCopyH2D>1"})
			mustEqual(t, "page in the global storage", f.stored(page.PAddr), uint32(0))
		})
	}
}

// A magic copy reads and writes the global storage itself, in no time and
// around the GPUs, on a timing platform as in emulation. It does so also for
// the memory that a timed copy flushes the caches for: it asks no GPU and
// flushes nothing, and what it copies is what the global storage has,
// whatever a cache holds.
func TestAMagicCopyOfDirtyMemoryIsDirect(t *testing.T) {
	for _, dirty := range dirtyPages {
		t.Run(dirty.name, func(t *testing.T) {
			f := newCopyFixture(t, true, buddyRuntime)
			page := dirty.make(f)

			if !f.r.NeedsFlush(f.ctx, gpuruntime.Ptr(page.VAddr), testPageSize) {
				t.Fatal("the memory is not dirty")
			}

			f.h2d(f.q, page, 0x12345678)
			f.run()

			mustEqual(t, "unfinished commands after the upload", f.r.Unfinished(), []uint64{0})
			mustEqual(t, "page in the global storage", f.stored(page.PAddr), uint32(0x12345678))

			host := f.d2h(f.q, page)
			f.run()

			var got uint32
			if err := f.r.ReadHost(host, &got); err != nil {
				t.Fatal(err)
			}

			mustEqual(t, "unfinished commands after the download", f.r.Unfinished(), []uint64{0})
			mustEqual(t, "download", got, uint32(0x12345678))

			if _, sent := f.port.RetrieveOutgoing(); sent || len(f.c.State.RequestsToSend) != 0 {
				t.Fatal("a magic copy asked a GPU")
			}
		})
	}
}

// Contexts made with InitWithExistingPID share an address space. A timed copy
// that one of them submits into an allocation of another, in a page that was
// used before, flushes the caches and goes through the GPU of the page.
func TestACopyThroughAnotherContextOfTheAddressSpaceIsFlushed(t *testing.T) {
	f := newCopyFixture(t, false, buddyRuntime)
	page := dirtyPages[1].make(f)

	sibling := f.r.InitWithExistingPID(f.ctx)
	f.h2d(f.r.CreateCommandQueue(sibling), page, 0x12345678)
	f.run()

	e := f.c.State.Executing
	if len(e) != 1 {
		t.Fatalf("the copy is not in flight: %+v", e)
	}

	mustEqual(t, "requests", f.requests(e[0]), []string{"Flush>1", "Flush>2", "MemCopyH2D>1"})
	mustEqual(t, "page in the global storage", f.stored(page.PAddr), uint32(0))
}
