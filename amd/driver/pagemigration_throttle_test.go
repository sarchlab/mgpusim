package driver

import (
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// newThreeGPUFixture builds a driver with three GPUs and the default
// page-migration specification, changed by configure.
func newThreeGPUFixture(
	t *testing.T,
	configure func(*PageMigrationSpec),
) *migrationFixture {
	t.Helper()

	f := newRuntimeConfiguredFixture(t, false, func(s *Spec) {
		s.GPUs = append(s.GPUs, "GPU[3].CommandProcessor")
		s.MigrationAgents = append(s.MigrationAgents, "GPU[3].CommandProcessor.PageMigration")
		configure(&s.PageMigration)
	}, func(s *gpuruntime.RuntimeSpec) {
		s.Devices = append(s.Devices, gpuruntime.DeviceProperties{CUCount: 4, DRAMSize: 4 * mem.GB})
	})

	return &migrationFixture{fixture: f, t: t}
}

// noPin makes a page thrash with its second migration within the lapse, as
// thrashingSpec does, with a nap of 1000 ns, and never pins it.
func noPin(s *PageMigrationSpec) {
	thrashingSpec(s)
	s.PinThreshold = 100
}

// thrashingPage returns a unified page that went to GPU 2 and back to GPU 1
// at time 0, and so thrashes.
func (f *migrationFixture) thrashingPage() vm.Page {
	f.t.Helper()

	page := f.unified(1)[0]
	f.migrate(page, 2)
	f.migrate(page, 1)

	return f.page(page.VAddr)
}

// thrashing returns the state of the page that thrashingPage returned.
func (f *migrationFixture) thrashing() migrationPage {
	return f.c.State.PageMigration.Pages[0]
}

// naps are the GPUs that are throttled on a page with the ends of their
// naps, in nanoseconds.
func naps(gpuAndEnd ...uint64) []throttledGPU {
	var out []throttledGPU

	for i := 0; i < len(gpuAndEnd); i += 2 {
		out = append(out, throttledGPU{GPU: int(gpuAndEnd[i]), Until: nsToPS(gpuAndEnd[i+1])})
	}

	return out
}

// mustTakeThePageAt checks that the driver does nothing with the thrashing
// page before the given time and that it then, on a tick that it asked for
// itself, starts to move the page to the GPU.
func (f *migrationFixture) mustTakeThePageAt(ns uint64, gpu int) {
	f.t.Helper()

	f.runTo(ns - 1)
	mustEqual(f.t, "commands before the nap ends", describe(f.outstanding()), []string(nil))

	f.runTo(ns)
	mustEqual(f.t, "commands when the nap ends", describe(f.outstanding()),
		[]string{"Hold>1", "Hold>2", "Hold>3"})
	mustEqual(f.t, "destination", f.thrashing().Dst, gpu)
	mustEqual(f.t, "selected", f.thrashing().Selected, gpu)
}

// Three GPUs want a thrashing page, and each is throttled at another time.
// Each waits a nap of its own: a GPU that is throttled later does not
// postpone a GPU that waits already, and the driver wakes at the end of each
// nap by itself. The page goes to the GPU whose nap ends, also when the
// fault of another GPU arrived before its own.
//
//nolint:funlen // Keep the rotation of the page between the three GPUs together.
func TestStaggeredGPUsEachWaitForTheirOwnNap(t *testing.T) {
	f := newThreeGPUFixture(t, noPin)
	page := f.thrashingPage()

	// GPU 2 asks at 0 and GPU 3 at 500 ns.
	f.fault(page, 2)
	f.advanceTo(500)
	f.fault(page, 3)

	mustEqual(t, "naps", f.thrashing().Throttled, naps(2, 1000, 3, 1500))
	mustEqual(t, "waiters", f.thrashing().Waiters, []int{2, 3})

	// The nap of GPU 2 ends at 1000 ns, not 1000 ns after GPU 3 asked.
	f.mustTakeThePageAt(1000, 2)
	mustEqual(t, "naps", f.thrashing().Throttled, naps(3, 1500))
	f.finishAll()
	mustEqual(t, "owner", f.page(page.VAddr).DeviceID, uint64(2))

	// GPU 1, which lost the page, asks for it and starts a nap of its own.
	// GPU 3 asks again behind it and has 500 ns of its nap left.
	f.fault(f.page(page.VAddr), 1)
	f.fault(f.page(page.VAddr), 3)

	mustEqual(t, "naps", f.thrashing().Throttled, naps(3, 1500, 1, 2000))
	mustEqual(t, "waiters", f.thrashing().Waiters, []int{1, 3})
	mustEqual(t, "commands of throttled GPUs", describe(f.outstanding()), []string(nil))

	// The nap of GPU 3 ends first, so the page goes to GPU 3, although GPU 1
	// is the first that waits.
	f.mustTakeThePageAt(1500, 3)
	f.finishAll()
	mustEqual(t, "owner", f.page(page.VAddr).DeviceID, uint64(3))

	// GPU 1 asks again and waits for the end of the nap it started at
	// 1000 ns. GPU 2 starts another nap.
	f.fault(f.page(page.VAddr), 2)
	f.fault(f.page(page.VAddr), 1)

	mustEqual(t, "naps", f.thrashing().Throttled, naps(1, 2000, 2, 2500))
	f.mustTakeThePageAt(2000, 1)
	f.finishAll()
	mustEqual(t, "owner", f.page(page.VAddr).DeviceID, uint64(1))

	if s := f.stats(); s.Throttles != 4 || s.Migrations != 5 || s.Pins != 0 {
		t.Fatalf("stats %+v", s)
	}
}

// A nap belongs to one GPU on one page: a GPU that is throttled on two pages
// at different times takes each page when the nap on that page ends.
func TestAGPUHasANapOnEachPage(t *testing.T) {
	f := newThreeGPUFixture(t, noPin)
	first := f.thrashingPage()

	second := f.unified(1)[0]
	f.migrate(second, 2)
	f.migrate(second, 1)
	second = f.page(second.VAddr)

	f.fault(first, 2)
	f.advanceTo(300)
	f.fault(second, 2)

	pages := f.c.State.PageMigration.Pages
	mustEqual(t, "naps on the first page", pages[0].Throttled, naps(2, 1000))
	mustEqual(t, "naps on the second page", pages[1].Throttled, naps(2, 1300))

	holds := []string{"Hold>1", "Hold>2", "Hold>3"}

	f.runTo(999)
	mustEqual(t, "commands before a nap ends", describe(f.outstanding()), []string(nil))
	f.runTo(1000)
	mustEqual(t, "first page at 1000 ns", describe(f.of(first)), holds)
	f.runTo(1299)
	mustEqual(t, "second page before its nap ends", describe(f.of(second)), []string(nil))
	f.runTo(1300)
	mustEqual(t, "second page at 1300 ns", describe(f.of(second)), holds)
}

// A memory copy keeps the faults of two throttled GPUs waiting until both
// naps are over. The page then goes to the GPU whose nap ended first,
// although the fault of the other GPU is the first that waits.
func TestTheGPUWhoseNapEndedFirstTakesThePageFirst(t *testing.T) {
	f := newThreeGPUFixture(t, noPin)
	page := f.thrashingPage()

	// As above: GPU 2 takes the page at 1000 ns, and then GPU 1 waits until
	// 2000 ns in front of GPU 3, which waits until 1200 ns.
	f.fault(page, 2)
	f.advanceTo(200)
	f.fault(page, 3)
	f.mustTakeThePageAt(1000, 2)
	f.finishAll()

	moved := f.page(page.VAddr)
	f.fault(moved, 1)
	f.fault(moved, 3)

	mustEqual(t, "naps", f.thrashing().Throttled, naps(3, 1200, 1, 2000))
	mustEqual(t, "waiters", f.thrashing().Waiters, []int{1, 3})

	// A copy from the page starts and runs until 2500 ns: the faults wait
	// for it beyond the end of both naps.
	f.running(gpuruntime.Command{
		Kind: gpuruntime.CopyD2H, Src: gpuruntime.Ptr(moved.VAddr), ByteSize: 64,
	}, []messaging.Msg{f.request(protocol.MemCopyD2HReq{})})

	f.advanceTo(2500)
	mustEqual(t, "commands during the copy", describe(f.outstanding()), []string(nil))

	if s := f.c.State.PageMigration; len(s.Queue) != 1 || !s.Queue[0].Deferred ||
		s.Stats.DeferredFaults != 1 {
		t.Fatalf("the faults do not wait for the copy: queue %+v, stats %+v", s.Queue, s.Stats)
	}

	f.c.State.Executing = nil
	f.ticks()

	mustEqual(t, "commands after the copy", describe(f.outstanding()),
		[]string{"Hold>1", "Hold>2", "Hold>3"})
	mustEqual(t, "destination", f.thrashing().Dst, 3)
	mustEqual(t, "naps", f.thrashing().Throttled, naps(1, 2000))

	// GPU 1 has waited its nap as well: it takes the page with its next
	// fault.
	f.finishAll()
	f.fault(f.page(page.VAddr), 1)
	mustEqual(t, "destination", f.thrashing().Dst, 1)
	f.finishAll()
	mustEqual(t, "owner", f.page(page.VAddr).DeviceID, uint64(1))
}

// With access counters, no access waits for a nap, and the driver does not
// tick for one: the notifications of a GPU are skipped until its own nap is
// over, whenever the other GPUs were throttled.
//
//nolint:funlen // Keep the notifications of the three GPUs in the order of their times.
func TestStaggeredNotificationsEachHaveTheirOwnNap(t *testing.T) {
	f := newThreeGPUFixture(t, func(s *PageMigrationSpec) {
		noPin(s)
		counterPolicy(s)
	})
	page := f.unified(1)[0]
	frame := func() uint64 { return f.page(page.VAddr).PAddr }
	owner := func() uint64 { return f.page(page.VAddr).DeviceID }

	// The page goes to GPU 2 and back, and thrashes.
	f.notify(frame(), 2)
	f.finishAll()
	f.notify(frame(), 1)
	f.finishAll()

	// GPU 2 is throttled at 0 and GPU 3 at 500 ns.
	f.notify(frame(), 2)
	f.advanceTo(500)
	f.notify(frame(), 3)
	mustEqual(t, "naps", f.thrashing().Throttled, naps(2, 1000, 3, 1500))

	// Nothing waits, so no nap keeps the simulation running.
	engine := f.s.Engine().(*timing.SerialEngine)
	if err := engine.Run(); err != nil {
		t.Fatal(err)
	}

	if engine.CurrentTime() >= nsToPS(1000) {
		t.Fatalf("the run waited for a nap: now %d", engine.CurrentTime())
	}

	// At 1000 ns the nap of GPU 2 is over, and that of GPU 3 is not.
	f.advanceTo(1000)
	f.notify(frame(), 3)
	mustEqual(t, "commands for GPU 3 in its nap", describe(f.outstanding()), []string(nil))
	f.notify(frame(), 2)
	f.finishAll()
	mustEqual(t, "owner at 1000 ns", owner(), uint64(2))

	// GPU 3 follows at 1500 ns.
	f.advanceTo(1499)
	f.notify(frame(), 3)
	mustEqual(t, "commands for GPU 3 in its nap", describe(f.outstanding()), []string(nil))
	f.advanceTo(1500)
	f.notify(frame(), 3)
	f.finishAll()
	mustEqual(t, "owner at 1500 ns", owner(), uint64(3))

	s := f.c.State.PageMigration
	if s.Stats.Throttles != 2 || s.Stats.ThrottledNotifications != 4 || s.Stats.Migrations != 4 ||
		len(s.Pages[0].Waiters) != 0 || len(s.Throttling) != 0 {
		t.Fatalf("stats %+v, page %+v", s.Stats, s.Pages[0])
	}
}

// A nap that a notification started has no tick of the driver. When an
// access of the GPU then waits for the page, the driver wakes at the end of
// that nap all the same.
func TestAFaultInANapThatANotificationStartedIsWokenAtItsEnd(t *testing.T) {
	f := newThreeGPUFixture(t, noPin)
	page := f.thrashingPage()

	f.notify(page.PAddr, 2)
	mustEqual(t, "naps", f.thrashing().Throttled, naps(2, 1000))

	f.advanceTo(300)
	f.fault(page, 2)
	mustEqual(t, "naps", f.thrashing().Throttled, naps(2, 1000))
	mustEqual(t, "waiters", f.thrashing().Waiters, []int{2})

	f.mustTakeThePageAt(1000, 2)
}

// The fault of a throttled GPU leaves the queue and waits on its page. When
// the nap ends, the driver queues the page again by itself, also when the
// queue is full: the fault was taken in before.
func TestTheEndOfANapQueuesAPageAlsoWhenTheQueueIsFull(t *testing.T) {
	f := newThreeGPUFixture(t, func(s *PageMigrationSpec) {
		noPin(s)
		s.QueueSize = 1
	})
	page := f.thrashingPage()
	other := f.unified(1)[0]

	f.fault(page, 2)
	mustEqual(t, "requests that wait during the nap", len(f.c.State.PageMigration.Queue), 0)

	// The fault of another page waits for a memory copy and has the only
	// entry of the queue.
	f.running(gpuruntime.Command{
		Kind: gpuruntime.CopyD2H, Src: gpuruntime.Ptr(other.VAddr), ByteSize: 64,
	}, []messaging.Msg{f.request(protocol.MemCopyD2HReq{})})
	f.fault(other, 3)

	if s := f.c.State.PageMigration; len(s.Queue) != 1 || !s.Queue[0].Deferred {
		t.Fatalf("the queue is not full: %+v", s.Queue)
	}

	f.mustTakeThePageAt(1000, 2)

	if s := f.c.State.PageMigration; len(s.Queue) != 1 || !s.Queue[0].Deferred {
		t.Fatalf("the fault that waits for the copy lost its entry: %+v", s.Queue)
	}
}

// The throttling operations of all GPUs count for the pin. When the page is
// pinned, a GPU that still waits for the end of its nap is released with the
// others, and the end of its nap moves nothing.
func TestAPinReleasesAGPUThatWaitsForItsNap(t *testing.T) {
	f := newThreeGPUFixture(t, thrashingSpec)
	page := f.thrashingPage()

	// Two throttling operations: GPU 2 at 0 and GPU 3 at 500 ns.
	f.fault(page, 2)
	f.advanceTo(500)
	f.fault(page, 3)
	f.mustTakeThePageAt(1000, 2)
	f.finishAll()

	// GPU 3 waits for 1500 ns. GPU 1 would be the third: the page is pinned
	// on GPU 2, and both GPUs are released to access it there.
	moved := f.page(page.VAddr)
	f.fault(moved, 3)
	mustEqual(t, "commands for GPU 3 in its nap", describe(f.outstanding()), []string(nil))
	f.fault(moved, 1)
	mustEqual(t, "holds of the release", f.step(moved), []string{"Hold>3", "Hold>1"})
	mustEqual(t, "release", f.step(moved), []string{"Release>3", "Release>1"})

	pinned := f.page(page.VAddr)
	if !pinned.IsPinned || pinned.DeviceID != 2 || f.stats().Pins != 1 || f.stats().Throttles != 2 {
		t.Fatalf("page not pinned in place: %+v, stats %+v", pinned, f.stats())
	}

	// The nap of GPU 3 ends at 1500 ns: nothing waits and nothing moves.
	f.runTo(1500)
	mustEqual(t, "commands at the end of the nap", describe(f.outstanding()), []string(nil))

	// The pin times out 5000 ns after 1000 ns and takes the naps with it.
	f.advanceTo(6000)
	f.finishAll()

	if p := f.thrashing(); f.stats().Unpins != 1 || len(p.Throttled) != 0 || p.ThrottleCount != 0 {
		t.Fatalf("the pin did not end: %+v, stats %+v", p, f.stats())
	}

	f.migrate(f.page(page.VAddr), 3)
}

// The end of each nap is state of the driver, and the ticks it asked for
// are events of the engine: a simulation that is taken back to a checkpoint
// with two GPUs in their naps wakes each of them at its own time again.
func TestTheNapsOfThrottledGPUsSurviveACheckpoint(t *testing.T) {
	f := newThreeGPUFixture(t, noPin)
	page := f.thrashingPage()

	f.fault(page, 2)
	f.advanceTo(500)
	f.fault(page, 3)

	path := filepath.Join(t.TempDir(), "throttled.tar.gz")
	if err := f.s.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}

	stats := f.stats()
	f.mustTakeThePageAt(1000, 2)

	// The engine runs out of events, which a checkpoint needs to be loaded.
	// Then back to 500 ns: the Holds that were sent after the checkpoint are
	// gone, and the page table was not changed since.
	f.run()

	if err := f.s.LoadCheckpoint(path); err != nil {
		t.Fatal(err)
	}

	f.unsolved = nil

	restored := f.c.State.PageMigration
	mustEqual(t, "time", f.s.Engine().CurrentTime(), nsToPS(500))
	mustEqual(t, "stats", restored.Stats, stats)
	mustEqual(t, "phase", restored.Pages[0].Phase, pageIdle)
	mustEqual(t, "waiters", restored.Pages[0].Waiters, []int{2, 3})
	mustEqual(t, "naps", restored.Pages[0].Throttled, naps(2, 1000, 3, 1500))
	mustEqual(t, "pages that wait for a nap", restored.Throttling, []int{0})

	// Both naps end when they did before.
	f.mustTakeThePageAt(1000, 2)
	mustEqual(t, "naps", f.thrashing().Throttled, naps(3, 1500))
	f.finishAll()

	f.fault(f.page(page.VAddr), 3)
	f.mustTakeThePageAt(1500, 3)
}
