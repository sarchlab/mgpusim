package driver

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

func TestDefaultSpecMigratesOnFaultsWithTheUVMParameters(t *testing.T) {
	s := DefaultPageMigrationSpec()

	mustEqual(t, "default specification", s, PageMigrationSpec{
		Policy:                 PageMigrationFault,
		BatchSize:              256,
		QueueSize:              1024,
		AccessCounterThreshold: 256,
		PrefetchThreshold:      51,
		ThrashingThreshold:     3,
		ThrashingLapseNS:       500_000,
		ThrashingNapNS:         500_000,
		ThrashingEpochNS:       2_000_000_000,
		PinThreshold:           10,
		PinNS:                  150_000_000,
		DataMoverPages:         64,
	})

	if !s.Enabled() || s.validate(2, 2) != nil {
		t.Fatal("the default specification must be valid and migrate")
	}

	s.Policy = PageMigrationCounter
	if !s.Enabled() || s.validate(2, 2) != nil {
		t.Fatal("access-counter migration is a policy of the same specification")
	}

	if s.validate(2, 1) == nil {
		t.Fatal("a migrating driver needs one migration agent per GPU")
	}
}

//nolint:funlen // Keep the phases of one migration and what each waits for together.
func TestFaultMigratesOnePageThroughEveryPhase(t *testing.T) {
	f := newMigrationFixture(t, noChange)
	page := f.unified(1)[0]

	f.fault(page, 2)

	holds := f.of(page)
	mustEqual(t, "holds", describe(holds), []string{"Hold>1", "Hold>2"})
	mustEqual(t, "hold", holds[0].Payload.(protocol.PageHoldReq), protocol.PageHoldReq{
		PID: page.PID, VAddr: page.VAddr, PageSize: testPageSize,
	})

	// Nothing else happens until the last GPU holds the page.
	f.answer(holds[0])
	mustEqual(t, "after one hold answer", describe(f.of(page)), []string{"Hold>2"})
	f.answer(holds[1])

	// The source flushes the old frame; the destination drops the new one.
	prepare := f.of(page)
	mustEqual(t, "prepare", describe(prepare), []string{"Flush>1", "Invalidate>2"})

	newFrame := prepare[1].Payload.(protocol.PageInvalidateReq).PAddr
	mustEqual(t, "flush", prepare[0].Payload.(protocol.PageFlushReq),
		protocol.PageFlushReq{PAddr: page.PAddr, PageSize: testPageSize})
	mustEqual(t, "owner of the new frame", f.r.DeviceIDByPAddr(newFrame), 2)

	// The copy waits for both: the destination pulls the page only after it
	// has answered that the L2 lines of the new frame are gone.
	f.answer(prepare[0])
	mustEqual(t, "before the invalidate answer", describe(f.of(page)),
		[]string{"Invalidate>2"})
	f.answer(prepare[1])

	copies := f.of(page)
	mustEqual(t, "copy", describe(copies), []string{"Copy>2"})
	mustEqual(t, "copy request", copies[0].Payload.(protocol.PageCopyReq),
		protocol.PageCopyReq{SrcAddress: page.PAddr, DstAddress: newFrame, PageSize: testPageSize})
	mustEqual(t, "owner before the copy answer", f.page(page.VAddr).DeviceID, uint64(1))

	// With the answer of the copy the page table changes, and the old
	// translation is shot down: the destination is not asked to invalidate
	// the new frame again.
	f.answer(copies...)

	moved := f.page(page.VAddr)
	if moved.DeviceID != 2 || moved.PAddr != newFrame {
		t.Fatalf("page not remapped: %+v", moved)
	}

	mustEqual(t, "shootdown", f.step(page), []string{"Shootdown>1", "Shootdown>2"})
	mustEqual(t, "release", f.step(page), []string{"Release>1", "Release>2"})

	s := f.c.State.PageMigration
	if s.Pages[0].Phase != pageIdle || len(s.Pending) != 0 || len(s.Windows) != 0 {
		t.Fatalf("page %+v, pending %v, windows %v", s.Pages[0], s.Pending, s.Windows)
	}

	got := s.Stats
	if got.Faults != 1 || got.Batches != 1 || got.Windows != 1 || got.Migrations != 1 ||
		got.PagesMigrated != 1 || got.BytesMigrated != testPageSize || got.FramesRetired != 1 {
		t.Fatalf("stats %+v", got)
	}
}

func TestFaultOnALocalPageHoldsAndReleasesOnlyTheFaultingGPU(t *testing.T) {
	f := newMigrationFixture(t, noChange)
	page := f.unified(1)[0]

	f.fault(page, 1)

	// The faulting GPU holds the page first, so the Release cannot overtake
	// a fault response still on its way.
	mustEqual(t, "hold", f.step(page), []string{"Hold>1"})
	mustEqual(t, "release", f.step(page), []string{"Release>1"})
	mustEqual(t, "end", f.step(page), []string(nil))

	if s := f.stats(); s.Releases != 1 || s.Migrations != 0 {
		t.Fatalf("stats %+v", s)
	}
}

func TestABatchStartsAtOnceWithTheRequestsThatWait(t *testing.T) {
	f := newMigrationFixture(t, func(s *PageMigrationSpec) {
		noPrefetch(s)
		s.BatchSize = 2
	})
	pages := f.unified(3)

	// Three faults wait when the driver ticks: it does not wait for more,
	// and takes a batch of two.
	for _, p := range pages {
		f.deliverFault(p, 2)
	}

	f.tick()

	mustEqual(t, "first batch", describe(f.outstanding()),
		[]string{"Hold>1", "Hold>2", "Hold>1", "Hold>2"})
	mustEqual(t, "waiting", len(f.c.State.PageMigration.Queue), 1)

	f.tick()

	mustEqual(t, "second batch", len(f.outstanding()), 6)

	if s := f.stats(); s.Batches != 2 || s.Faults != 3 {
		t.Fatalf("stats %+v", s)
	}
}

func TestRepeatedFaultsOfAGPUOnAPageAreOneFault(t *testing.T) {
	f := newMigrationFixture(t, noChange)
	page := f.unified(1)[0]

	// Every address translator of GPU 2 faults on its own; GPU 1 has the
	// page, and one of its translators faulted before the page arrived.
	for range 3 {
		f.deliverFault(page, 2)
	}

	f.deliverFault(page, 1)
	f.tick()

	s := f.c.State.PageMigration
	if s.Stats.Faults != 4 || s.Stats.CoalescedFaults != 2 || s.Stats.Batches != 1 {
		t.Fatalf("stats %+v", s.Stats)
	}

	// One migration answers them all: every GPU is held and released.
	mustEqual(t, "holds", describe(f.outstanding()), []string{"Hold>1", "Hold>2"})
	mustEqual(t, "waiters", len(s.Pages[0].Waiters), 0)
}

//nolint:funlen // Keep the faults of every phase and their reconsideration together.
func TestAFaultDuringAnyPhaseWaitsForTheLastReleaseAnswer(t *testing.T) {
	f := newMigrationFixture(t, noPrefetch)
	page := f.unified(1)[0]

	f.fault(page, 2)

	// A fault arrives in each phase of the migration: GPU 2 again while the
	// GPUs hold and while the page is copied, and GPU 1, which loses the
	// page, during the Release.
	f.fault(page, 2)
	mustEqual(t, "hold", f.step(page), []string{"Hold>1", "Hold>2"})
	mustEqual(t, "prepare", f.step(page), []string{"Flush>1", "Invalidate>2"})
	f.fault(page, 2)
	mustEqual(t, "copy", f.step(page), []string{"Copy>2"})
	mustEqual(t, "shootdown", f.step(page), []string{"Shootdown>1", "Shootdown>2"})

	releases := f.of(page)
	mustEqual(t, "release", describe(releases), []string{"Release>1", "Release>2"})

	f.answer(releases[0])
	f.fault(page, 1)

	// None of them is lost and none is serviced yet: one Release answer is
	// still out.
	s := f.c.State.PageMigration
	mustEqual(t, "waiters during the release", s.Pages[0].Waiters, []int{2, 1})
	mustEqual(t, "commands during the release", describe(f.outstanding()), []string{"Release>2"})

	if s.Stats.RetainedFaults != 2 || s.Stats.CoalescedFaults != 1 {
		t.Fatalf("stats %+v", s.Stats)
	}

	// The last Release answer: the faults are considered against the mapping
	// as it is now. The page is on GPU 2, and GPU 1 asks for it back.
	f.answer(releases[1])
	mustEqual(t, "second service", describe(f.of(page)), []string{"Hold>1", "Hold>2"})
	f.finishAll()

	mustEqual(t, "owner", f.page(page.VAddr).DeviceID, uint64(1))

	if s := f.stats(); s.Migrations != 2 || s.Releases != 0 {
		t.Fatalf("stats %+v", s)
	}
}

func TestAFaultOfTheNewOwnerDuringTheMigrationIsReleasedAfterIt(t *testing.T) {
	f := newMigrationFixture(t, noPrefetch)
	page := f.unified(1)[0]

	f.fault(page, 2)
	mustEqual(t, "hold", f.step(page), []string{"Hold>1", "Hold>2"})

	// Another translator of GPU 2 faults. Its response may reach it after
	// the Release of the migration, so the fault gets a service of its own.
	f.fault(page, 2)
	f.finish(page)

	if s := f.stats(); s.Migrations != 1 || s.Releases != 1 || s.RetainedFaults != 1 {
		t.Fatalf("stats %+v", s)
	}

	mustEqual(t, "owner", f.page(page.VAddr).DeviceID, uint64(2))
}

func TestAFullQueueKeepsTheFaultsInThePort(t *testing.T) {
	f := newMigrationFixture(t, func(s *PageMigrationSpec) {
		noPrefetch(s)
		s.QueueSize = 1
		s.BatchSize = 1
	})
	pages := f.unified(4)

	for _, p := range pages {
		f.deliverFault(p, 2)
	}

	// The first fault fills the queue, so the faults behind it wait in the
	// port; the batch of the tick then services the one that was taken.
	f.tick()

	mustEqual(t, "faults in the port", f.pn.IncomingBuf().Size(), 3)
	mustEqual(t, "commands", describe(f.outstanding()), []string{"Hold>1", "Hold>2"})

	if s := f.stats(); s.QueueFullWaits != 1 || s.Faults != 1 {
		t.Fatalf("stats %+v", s)
	}

	// The answers of the command processors are not behind the faults.
	f.answer(f.of(pages[0])...)
	mustEqual(t, "first page", describe(f.of(pages[0])), []string{"Flush>1", "Invalidate>2"})

	// Every fault is serviced in the end.
	f.finishAll()

	for _, p := range pages {
		mustEqual(t, "owner", f.page(p.VAddr).DeviceID, uint64(2))
	}

	if s := f.stats(); s.Faults != 4 || s.Migrations != 4 {
		t.Fatalf("stats %+v", s)
	}
}

func TestAFaultWaitsForAMemoryCopyThatUsesThePage(t *testing.T) {
	f := newMigrationFixture(t, noPrefetch)
	page := f.unified(1)[0]

	f.running(gpuruntime.Command{
		Kind: gpuruntime.CopyD2H, Src: gpuruntime.Ptr(page.VAddr), ByteSize: 64,
	}, []messaging.Msg{f.request(protocol.MemCopyD2HReq{})})

	f.fault(page, 2)

	s := f.c.State.PageMigration
	if len(f.outstanding()) != 0 || len(s.Queue) != 1 || s.Stats.DeferredFaults == 0 {
		t.Fatalf("queue %v, stats %+v", s.Queue, s.Stats)
	}

	// The copy ends: the fault that waited is serviced.
	f.c.State.Executing = nil
	f.ticks()

	mustEqual(t, "holds", describe(f.outstanding()), []string{"Hold>1", "Hold>2"})
}

func TestAMemoryCopyWaitsForAPageThatMigrates(t *testing.T) {
	f := newMigrationFixture(t, noPrefetch)
	pages := f.unified(2)

	f.fault(pages[0], 2)

	m := f.c.Middlewares.Migration
	if !m.migrating(f.pid(), pages[0].VAddr+8, 16) {
		t.Fatal("a copy into a migrating page must wait")
	}

	if m.migrating(f.pid(), pages[1].VAddr, testPageSize) {
		t.Fatal("a copy into another page must not wait")
	}

	f.finishAll()

	if m.migrating(f.pid(), pages[0].VAddr, testPageSize) {
		t.Fatal("the page is idle again")
	}
}

func TestMigrationStateSurvivesACheckpoint(t *testing.T) {
	f := newMigrationFixture(t, noChange)
	pages := f.unified(2)

	// One page in the middle of its migration with a fault that waits on
	// it, one page whose migration has just started.
	f.fault(pages[0], 2)
	f.step(pages[0])
	f.fault(pages[0], 2)
	f.fault(pages[1], 2)

	var saved bytes.Buffer
	if err := f.c.SaveCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}

	restored := newMigrationFixture(t, noChange)
	if err := restored.c.LoadCheckpoint(bytes.NewReader(saved.Bytes())); err != nil {
		t.Fatal(err)
	}

	want, got := f.c.State.PageMigration, restored.c.State.PageMigration
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("restored state differs:\n%+v\n%+v", want, got)
	}

	if got.Pages[0].Phase != pagePrepare || len(got.Pages[0].Waiters) != 1 ||
		len(got.Windows) != 2 || len(got.Pending) != 4 {
		t.Fatalf("checkpoint did not hold a migration in flight: %+v", got)
	}
}

// The timeout of a pin is background work: it does not keep a simulation
// running that has nothing else to do.
func TestPinExpiryDoesNotKeepTheSimulationRunning(t *testing.T) {
	f := newMigrationFixture(t, func(s *PageMigrationSpec) {
		noPrefetch(s)
		s.PinNS = 1_000_000
	})
	page := f.unified(1)[0]

	f.c.Middlewares.Migration.pageIndex(page.PID, page.VAddr)
	f.c.Middlewares.Migration.pin(0, page)

	engine := f.s.Engine().(*timing.SerialEngine)
	if err := engine.Run(); err != nil {
		t.Fatal(err)
	}

	if engine.CurrentTime() >= nsToPS(1_000_000) || !f.page(page.VAddr).IsPinned {
		t.Fatalf("the run waited for the pin: now %d", engine.CurrentTime())
	}

	// With other work that reaches the timeout, the pin ends.
	f.advanceTo(1_000_000)
	f.finishAll()

	if f.page(page.VAddr).IsPinned || f.stats().Unpins != 1 {
		t.Fatalf("pin did not end: %+v", f.stats())
	}
}
