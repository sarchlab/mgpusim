package driver

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// block allocates unified pages on GPU 1 and returns the 32 of them that
// start an aligned group of 32 page positions, all in one 2 MiB block.
func (f *migrationFixture) block() []vm.Page {
	f.t.Helper()

	pages := f.unified(96)
	start := 0

	for pages[start].VAddr/testPageSize%32 != 0 {
		start++
	}

	return pages[start : start+32]
}

// copySources returns the source addresses of the copies that are not
// answered yet, in the order the driver submitted them.
func (f *migrationFixture) copySources() []uint64 {
	var srcs []uint64

	for _, msg := range f.outstanding() {
		if c, ok := msg.Payload.(protocol.PageCopyReq); ok {
			srcs = append(srcs, c.SrcAddress)
		}
	}

	return srcs
}

// ready answers the commands of a page until it waits for or is in its copy.
func (f *migrationFixture) ready(page vm.Page) {
	f.t.Helper()

	mustEqual(f.t, "hold", f.step(page), []string{"Hold>1", "Hold>2"})
	mustEqual(f.t, "prepare", f.step(page), []string{"Flush>1", "Invalidate>2"})
}

// windowABCX starts a window of three pages, A, B, and C, and then a window
// of one page, X, all to GPU 2.
func windowABCX(t *testing.T) (f *migrationFixture, a, b, c, x vm.Page) {
	t.Helper()

	f = newMigrationFixture(t, noChange)
	pages := f.block()

	// Five of the eight pages around A are on GPU 2 already: with A, more
	// than 51% of the group, so A takes the rest of the group, B and C,
	// with it. Nothing is near X.
	for _, p := range pages[8:13] {
		f.place(p, 2)
	}

	a, b, c, x = pages[13], pages[14], pages[15], pages[24]
	f.deliverFault(a, 2)
	f.deliverFault(x, 2)
	f.ticks()

	s := f.c.State.PageMigration
	mustEqual(t, "windows", len(s.Windows), 2)
	mustEqual(t, "first window", len(s.Windows[0].Pages), 3)
	mustEqual(t, "second window", len(s.Windows[1].Pages), 1)

	return f, a, b, c, x
}

func TestAWindowSubmitsTheTriggerFirstThenThePrefetchedPagesByAddress(t *testing.T) {
	f, a, b, c, _ := windowABCX(t)

	// The pages become ready in the opposite order.
	f.ready(c)
	f.ready(b)
	mustEqual(t, "copies before the trigger is ready", f.copySources(), []uint64(nil))

	f.ready(a)
	mustEqual(t, "copies", f.copySources(), []uint64{a.PAddr, b.PAddr, c.PAddr})
}

//nolint:funlen // Keep the whole scheduling scenario of the two windows together.
func TestTheNextWindowIsSubmittedAfterTheEarlierOneAndBeforeItCompletes(t *testing.T) {
	f, a, b, c, x := windowABCX(t)

	// X is ready first, but the window before it on the data mover of GPU 2
	// is not submitted yet.
	f.ready(x)
	f.ready(a)
	f.ready(b)
	mustEqual(t, "copies while C is not ready", f.copySources(), []uint64{a.PAddr, b.PAddr})

	// C is submitted, which completes the first window: X follows at once,
	// while A, B, and C are still being copied.
	f.ready(c)
	mustEqual(t, "copies", f.copySources(), []uint64{a.PAddr, b.PAddr, c.PAddr, x.PAddr})
	mustEqual(t, "windows left", len(f.c.State.PageMigration.Windows), 0)

	// Each page goes on with its own completion: X ends while the first
	// window is still in flight.
	f.finish(x)
	mustEqual(t, "owner of X", f.page(x.VAddr).DeviceID, uint64(2))
	mustEqual(t, "owner of C", f.page(c.VAddr).DeviceID, uint64(1))
	mustEqual(t, "copies in flight", f.copySources(), []uint64{a.PAddr, b.PAddr, c.PAddr})

	// A second fault for C waits until the last Release answer of C.
	f.fault(c, 2)
	mustEqual(t, "copy", f.step(c), []string{"Copy>2"})
	mustEqual(t, "invalidate", f.step(c), []string{"Invalidate>2"})
	mustEqual(t, "shootdown", f.step(c), []string{"Shootdown>1", "Shootdown>2"})

	releases := f.of(c)
	f.answer(releases[0])
	mustEqual(t, "before the last release answer", describe(f.of(c)), []string{"Release>2"})

	f.answer(releases[1])
	mustEqual(t, "after the last release answer", describe(f.of(c)), []string{"Hold>2"})

	f.finishAll()

	s := f.stats()
	if s.Windows != 2 || s.Migrations != 2 || s.PagesPrefetched != 2 || s.Releases != 1 {
		t.Fatalf("stats %+v", s)
	}
}

func TestWindowsOfDifferentDataMoversDoNotWaitForEachOther(t *testing.T) {
	f := newMigrationFixture(t, noPrefetch)
	pages := f.unified(2)
	f.place(pages[1], 2)

	// Page 0 moves to GPU 2 and page 1 to GPU 1: two inbound data movers.
	f.fault(pages[0], 2)
	f.fault(f.page(pages[1].VAddr), 1)

	mustEqual(t, "hold", f.step(pages[1]), []string{"Hold>1", "Hold>2"})
	mustEqual(t, "prepare", f.step(pages[1]), []string{"Flush>2", "Invalidate>1"})
	mustEqual(t, "copy", describe(f.of(pages[1])), []string{"Copy>1"})
}

func TestASweepPrefetchesTheRestOfTheRegion(t *testing.T) {
	f := newMigrationFixture(t, noChange)
	pages := f.block()

	// One of two page positions is not more than 51%: nothing is prefetched
	// until three of the four pages of a group are on GPU 2 or migrate to
	// it.
	f.migrate(pages[0], 2)
	f.migrate(pages[1], 2)
	mustEqual(t, "prefetched after two faults", f.stats().PagesPrefetched, uint64(0))

	f.migrate(pages[2], 2)
	mustEqual(t, "owner of the fourth page", f.page(pages[3].VAddr).DeviceID, uint64(2))

	// The fault on the fifth page has four of eight behind it: five of
	// eight with itself, so the group of eight is complete.
	f.migrate(pages[4], 2)

	for _, p := range pages[:8] {
		mustEqual(t, "owner", f.page(p.VAddr).DeviceID, uint64(2))
	}

	// And so on: nine of sixteen.
	f.migrate(pages[8], 2)

	for _, p := range pages[:16] {
		mustEqual(t, "owner", f.page(p.VAddr).DeviceID, uint64(2))
	}

	mustEqual(t, "owner of the next group", f.page(pages[16].VAddr).DeviceID, uint64(1))

	s := f.stats()
	if s.PagesPrefetched != 11 || s.Migrations != 5 || s.PagesMigrated != 16 || s.Windows != 5 {
		t.Fatalf("stats %+v", s)
	}
}

func TestPrefetchCountsEveryPagePositionOfARegion(t *testing.T) {
	f := newMigrationFixture(t, noChange)
	pages := f.block()

	// A sparse group of four: one page on GPU 2, the page that faults, a
	// position without a page, and a page that could move. Two of four
	// positions are not more than 51%, although two of the three pages that
	// exist are on GPU 2 or move there.
	f.place(pages[4], 2)
	f.p.Remove(pages[6].PID, pages[6].VAddr)

	f.fault(pages[5], 2)
	mustEqual(t, "window", len(f.c.State.PageMigration.Windows[0].Pages), 1)

	f.finishAll()
	mustEqual(t, "owner of the page that could move", f.page(pages[7].VAddr).DeviceID, uint64(1))
}

// fullGroup puts five pages of an aligned group of eight on GPU 2 and
// returns the group. A fault of GPU 2 on its sixth page makes six of eight,
// so the seventh and the eighth are prefetched if they may move.
func (f *migrationFixture) fullGroup() []vm.Page {
	f.t.Helper()

	group := f.block()[8:16]
	for _, p := range group[:5] {
		f.place(p, 2)
	}

	return group
}

func TestPrefetchMovesOnlyThePagesThatExistAndMayMove(t *testing.T) {
	f := newMigrationFixture(t, noChange)
	m := f.c.Middlewares.Migration

	// A sparse window: one position has no page and one page is pinned.
	group := f.fullGroup()
	f.p.Remove(group[6].PID, group[6].VAddr)
	m.pin(m.pageIndex(group[7].PID, group[7].VAddr), group[7])

	f.fault(group[5], 2)
	mustEqual(t, "window", len(f.c.State.PageMigration.Windows[0].Pages), 1)
	f.finishAll()

	// A page that thrashes is left, and the page behind it moves.
	group = f.fullGroup()
	thrashing := m.pageIndex(group[6].PID, group[6].VAddr)
	f.c.State.PageMigration.Pages[thrashing].Events = f.c.Spec.PageMigration.ThrashingThreshold

	f.fault(group[5], 2)

	s := f.c.State.PageMigration
	mustEqual(t, "window", len(s.Windows[0].Pages), 2)
	mustEqual(t, "prefetched", s.Pages[s.Windows[0].Pages[1]].VAddr, group[7].VAddr)
}

func TestAThrashingPageDoesNotWidenItsWindow(t *testing.T) {
	f := newMigrationFixture(t, noChange)
	m := f.c.Middlewares.Migration
	group := f.fullGroup()

	// The page that faults thrashes, and GPU 2 is the GPU that may migrate
	// it: it moves alone, although its group is full enough to take the two
	// pages behind it along.
	pi := m.pageIndex(group[5].PID, group[5].VAddr)
	f.c.State.PageMigration.Pages[pi].Events = f.c.Spec.PageMigration.ThrashingThreshold
	f.c.State.PageMigration.Pages[pi].Selected = 2

	f.fault(group[5], 2)

	s := f.c.State.PageMigration
	mustEqual(t, "windows", len(s.Windows), 1)
	mustEqual(t, "window", len(s.Windows[0].Pages), 1)
}

func TestPrefetchThresholdZeroTurnsPrefetchingOff(t *testing.T) {
	f := newMigrationFixture(t, noPrefetch)
	pages := f.block()

	for _, p := range pages[:8] {
		f.migrate(p, 2)
	}

	if s := f.stats(); s.PagesPrefetched != 0 || s.Migrations != 8 {
		t.Fatalf("stats %+v", s)
	}
}

func counterPolicy(s *PageMigrationSpec) { s.Policy = PageMigrationCounter }

func TestANotificationMigratesThePageOfTheFrameToTheNotifier(t *testing.T) {
	f := newMigrationFixture(t, counterPolicy)
	page := f.unified(1)[0]

	// Twice before the driver ticks: one request.
	f.deliverNotification(page.PAddr, 2)
	f.notify(page.PAddr+0x40, 2)

	mustEqual(t, "holds", describe(f.of(page)), []string{"Hold>1", "Hold>2"})
	f.finishAll()
	mustEqual(t, "owner", f.page(page.VAddr).DeviceID, uint64(2))

	s := f.stats()
	if s.Notifications != 2 || s.CoalescedNotifications != 1 || s.Migrations != 1 ||
		s.Faults != 0 || s.Releases != 0 {
		t.Fatalf("stats %+v", s)
	}
}

func TestACounterTriggeredMigrationPrefetches(t *testing.T) {
	f := newMigrationFixture(t, counterPolicy)
	pages := f.block()

	for _, p := range pages[8:13] {
		f.place(p, 2)
	}

	f.notify(pages[13].PAddr, 2)
	f.finishAll()

	for _, p := range pages[13:16] {
		mustEqual(t, "owner", f.page(p.VAddr).DeviceID, uint64(2))
	}

	if s := f.stats(); s.PagesPrefetched != 2 || s.Migrations != 1 {
		t.Fatalf("stats %+v", s)
	}
}

func TestANotificationIsResolvedAgainstTheFrameAsItIsWhenServiced(t *testing.T) {
	f := newMigrationFixture(t, func(s *PageMigrationSpec) {
		counterPolicy(s)
		noPrefetch(s)
	})
	first := f.unified(1)[0]

	// The frame is free when its notification is serviced: skipped in
	// silence, with nothing sent.
	if err := f.r.FreeMemory(f.ctx, gpuruntime.Ptr(first.VAddr)); err != nil {
		t.Fatal(err)
	}

	f.notify(first.PAddr, 2)

	if s := f.stats(); s.FreeFrameNotifications != 1 || len(f.outstanding()) != 0 {
		t.Fatalf("stats %+v", s)
	}

	// The frame holds another page now: that page is judged, and moves.
	second := f.unified(1)[0]
	second.PAddr = first.PAddr
	f.p.Update(second)

	f.notify(first.PAddr, 2)
	mustEqual(t, "holds", describe(f.of(second)), []string{"Hold>1", "Hold>2"})
	f.finishAll()

	// On the notifier, serviced, or pinned: ignored.
	moved := f.page(second.VAddr)
	f.notify(moved.PAddr, 2)

	if s := f.stats(); s.IgnoredNotifications != 1 || s.Migrations != 1 {
		t.Fatalf("stats %+v", s)
	}
}

// thrashingSpec makes a page thrash with its second migration within the
// lapse, and pins it with the third throttling.
func thrashingSpec(s *PageMigrationSpec) {
	noPrefetch(s)
	s.ThrashingThreshold = 1
	s.ThrashingLapseNS = 1000
	s.ThrashingNapNS = 1000
	s.PinThreshold = 2
	s.PinNS = 5000
}

//nolint:funlen // Keep the three stages of one thrashing page together.
func TestThrashingIsDetectedThrottledAndPinned(t *testing.T) {
	f := newMigrationFixture(t, thrashingSpec)
	page := f.unified(1)[0]
	state := func() migrationPage { return f.c.State.PageMigration.Pages[0] }

	// Detection: the second migration is within the lapse of the first.
	f.migrate(page, 2)
	mustEqual(t, "events after one migration", state().Events, 0)
	f.migrate(page, 1)

	if s := f.stats(); s.ThrashingEvents != 1 || s.ThrashingPages != 1 || s.Throttles != 0 {
		t.Fatalf("stats %+v", s)
	}

	// Throttling: GPU 2 asks for the thrashing page and must wait for its
	// nap. Its fault is kept; nothing is sent.
	f.fault(page, 2)
	mustEqual(t, "commands of a throttled GPU", len(f.outstanding()), 0)
	mustEqual(t, "throttled", state().Throttled, []int{2})
	mustEqual(t, "waiters", state().Waiters, []int{2})

	// The nap ends: GPU 2 is the one that makes progress, and migrates.
	f.advanceTo(1000)
	mustEqual(t, "selected", state().Selected, 2)
	f.finishAll()
	mustEqual(t, "owner after the nap", f.page(page.VAddr).DeviceID, uint64(2))

	// GPU 1 is throttled in turn, while GPU 2 keeps the page.
	f.fault(page, 1)
	mustEqual(t, "throttles", f.stats().Throttles, uint64(2))
	f.advanceTo(2000)
	f.finishAll()
	mustEqual(t, "owner after the second nap", f.page(page.VAddr).DeviceID, uint64(1))

	// Pinning: after two throttling operations in a row the page stays
	// where it is, and GPU 2 is released to access it remotely.
	f.fault(page, 2)
	mustEqual(t, "release of the waiting GPU", f.step(page), []string{"Hold>2"})
	mustEqual(t, "release of the waiting GPU", f.step(page), []string{"Release>2"})

	pinned := f.page(page.VAddr)
	if !pinned.IsPinned || pinned.DeviceID != 1 || f.stats().Pins != 1 {
		t.Fatalf("page not pinned in place: %+v, stats %+v", pinned, f.stats())
	}

	// A fault that was on its way when the page was pinned is released too.
	f.fault(page, 2)
	mustEqual(t, "late fault", f.step(page), []string{"Hold>2"})
	f.finishAll()

	// The pin times out: the pin is cleared under a hold of every GPU, and
	// the translations are shot down so that migration can resume.
	f.advanceTo(7000)
	mustEqual(t, "unpin hold", f.step(page), []string{"Hold>1", "Hold>2"})
	mustEqual(t, "pin after the hold", f.page(page.VAddr).IsPinned, false)
	mustEqual(t, "unpin shootdown", f.step(page), []string{"Shootdown>1", "Shootdown>2"})
	mustEqual(t, "unpin release", f.step(page), []string{"Release>1", "Release>2"})

	if s := f.stats(); s.Unpins != 1 || state().Events != 0 || state().ThrottleCount != 0 {
		t.Fatalf("stats %+v, page %+v", s, state())
	}

	f.migrate(page, 2)
}

func TestAThrottledGPUKeepsWaitingWhileTheSelectedGPUMigrates(t *testing.T) {
	f := newMigrationFixture(t, func(s *PageMigrationSpec) {
		thrashingSpec(s)
		s.PinThreshold = 100
	})
	page := f.unified(1)[0]

	f.migrate(page, 2)
	f.migrate(page, 1)

	// GPU 2 is throttled until 1000 ns and then selected.
	f.fault(page, 2)
	f.advanceTo(1000)
	f.finishAll()

	// GPU 1 is throttled until 2000 ns. Meanwhile GPU 2 has the page; a
	// fault of GPU 2 that was on its way is released, and GPU 1 still waits.
	f.fault(page, 1)
	f.fault(page, 2)
	mustEqual(t, "release of the owner", f.step(page), []string{"Hold>2"})
	f.finishAll()

	p := f.c.State.PageMigration.Pages[0]
	mustEqual(t, "waiters", p.Waiters, []int{1})
	mustEqual(t, "owner", f.page(page.VAddr).DeviceID, uint64(2))

	f.advanceTo(2000)
	f.finishAll()
	mustEqual(t, "owner after the nap", f.page(page.VAddr).DeviceID, uint64(1))
}

func TestNotificationsOfAThrottledGPUAreSkipped(t *testing.T) {
	f := newMigrationFixture(t, func(s *PageMigrationSpec) {
		thrashingSpec(s)
		counterPolicy(s)
	})
	page := f.unified(1)[0]

	f.notify(page.PAddr, 2)
	f.finishAll()
	f.notify(f.page(page.VAddr).PAddr, 1)
	f.finishAll()

	// The page thrashes: GPU 2 is throttled, and its notifications are
	// skipped during the nap. Nothing waits for them.
	frame := f.page(page.VAddr).PAddr
	f.notify(frame, 2)
	f.notify(frame, 2)

	s := f.c.State.PageMigration
	if s.Stats.ThrottledNotifications != 2 || s.Stats.Throttles != 1 ||
		len(f.outstanding()) != 0 || len(s.Pages[0].Waiters) != 0 || len(s.Queue) != 0 {
		t.Fatalf("stats %+v, page %+v", s.Stats, s.Pages[0])
	}

	// After the nap, a notification of GPU 2 migrates the page.
	f.advanceTo(1000)
	f.notify(frame, 2)
	f.finishAll()
	mustEqual(t, "owner", f.page(page.VAddr).DeviceID, uint64(2))
}

func TestThrashingIsForgottenAfterAnEpoch(t *testing.T) {
	f := newMigrationFixture(t, func(s *PageMigrationSpec) {
		thrashingSpec(s)
		s.ThrashingEpochNS = 10_000
	})
	page := f.unified(1)[0]

	f.migrate(page, 2)
	f.migrate(page, 1)
	mustEqual(t, "events", f.c.State.PageMigration.Pages[0].Events, 1)

	f.advanceTo(20_000)
	f.migrate(page, 2)

	p := f.c.State.PageMigration.Pages[0]
	if p.Events != 0 || f.stats().Throttles != 0 {
		t.Fatalf("page %+v", p)
	}
}

// With the buddy allocator, a freed frame is the next one handed out, so the
// test sees which frames return to the allocator.
func TestTheFrameAPageLeavesIsRetiredAndTheFrameItHasIsFreedWithIt(t *testing.T) {
	base := newRuntimeConfiguredFixture(t, false,
		func(s *Spec) { noPrefetch(&s.PageMigration) },
		func(s *gpuruntime.RuntimeSpec) { s.Allocator = gpuruntime.AllocatorBuddy })
	f := &migrationFixture{fixture: base, t: t}
	page := f.unified(1)[0]

	f.migrate(page, 2)
	away := f.page(page.VAddr).PAddr
	f.migrate(page, 1)
	back := f.page(page.VAddr).PAddr

	// The page did not return to the frame it left: that frame is retired,
	// although the buddy allocator would hand a free frame out at once.
	if back == page.PAddr || f.r.DeviceIDByPAddr(back) != 1 {
		t.Fatalf("the page returned to frame %#x, first frame %#x", back, page.PAddr)
	}

	mustEqual(t, "frames retired", f.stats().FramesRetired, uint64(2))

	// Free returns the frame the page has now, and the allocator hands it
	// out again; the two retired frames stay out of use.
	if err := f.r.FreeMemory(f.ctx, gpuruntime.Ptr(page.VAddr)); err != nil {
		t.Fatal(err)
	}

	mustEqual(t, "frame after the free", f.r.AllocateFrame(1), back)

	for range 8 {
		if got := f.r.AllocateFrame(1); got == page.PAddr {
			t.Fatalf("retired frame %#x was allocated again", got)
		}

		if got := f.r.AllocateFrame(2); got == away {
			t.Fatalf("retired frame %#x was allocated again", got)
		}
	}
}
