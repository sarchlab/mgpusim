package driver

import (
	"slices"

	"github.com/sarchlab/akita/v5/mem/vm"
)

// The thrashing policy follows the stages of the NVIDIA UVM driver
// (uvm_perf_thrashing.c).
//
//  1. Detection. Every migration of a page, whether a fault, an
//     access-counter notification, or a prefetch moves it, within the lapse
//     of its previous migration is a thrashing event. A page with a
//     threshold of events is thrashing.
//  2. Throttling. A GPU that asks for a thrashing page waits for a nap,
//     unless it is the GPU selected to make progress. The GPU whose nap ends
//     becomes the selected one and may migrate the page, so the page stays
//     for at least a nap where it is used.
//  3. Pinning. After a threshold of throttling operations in a row, the page
//     is pinned where it is and the other GPUs access it remotely, until the
//     pin times out.

// Hints of the thrashing policy for a GPU that asks for a page.
const (
	hintMigrate  = iota // the page moves to the GPU
	hintThrottle        // the GPU waits for the end of its throttling
	hintPin             // the page is pinned where it is
)

// thrashing reports whether a page has been detected as thrashing.
func (m *migrationMW) thrashing(p *migrationPage) bool {
	return p.Events >= m.spec().ThrashingThreshold
}

// forgetThrashing ends the thrashing of a page that nothing happened to for
// an epoch.
func (m *migrationMW) forgetThrashing(p *migrationPage) {
	if !p.HasEvent || p.Pinned ||
		m.now()-p.LastActive <= nsToPS(m.spec().ThrashingEpochNS) {
		return
	}

	resetThrashing(p)
}

func resetThrashing(p *migrationPage) {
	p.Events = 0
	p.Throttled = nil
	p.ThrottleCount = 0
	p.Selected = 0
}

// recordMigration takes the migration of a page that starts now into the
// detection of thrashing.
func (m *migrationMW) recordMigration(pi int) {
	s := m.state()
	p := &s.Pages[pi]
	now := m.now()

	m.forgetThrashing(p)

	if p.HasEvent && now-p.LastEvent <= nsToPS(m.spec().ThrashingLapseNS) &&
		!m.thrashing(p) {
		p.Events++
		s.Stats.ThrashingEvents++

		if m.thrashing(p) {
			s.Stats.ThrashingPages++
		}
	}

	p.HasEvent = true
	p.LastEvent = now
	p.LastActive = now
}

// hint tells what to do with a page that GPU gpu, which does not have it,
// asks for. A GPU that is throttled keeps waiting until the nap ends; with
// wake set, an access of the GPU waits for the page, so the driver must tick
// then.
func (m *migrationMW) hint(pi, gpu int, wake bool) int {
	s := m.state()
	p := &s.Pages[pi]
	now := m.now()

	m.forgetThrashing(p)

	if !m.thrashing(p) {
		return hintMigrate
	}

	if slices.Contains(p.Throttled, gpu) {
		if now < p.ThrottleEnd {
			return hintThrottle
		}

		// The nap is over: the GPU is the one that makes progress now.
		p.Throttled = slices.DeleteFunc(p.Throttled, func(g int) bool {
			return g == gpu
		})
		p.Selected = gpu

		return hintMigrate
	}

	if p.Selected == gpu {
		return hintMigrate
	}

	if p.ThrottleCount >= m.spec().PinThreshold {
		return hintPin
	}

	p.Throttled = append(p.Throttled, gpu)
	p.ThrottleEnd = now + nsToPS(m.spec().ThrashingNapNS)
	p.ThrottleCount++
	p.LastActive = now
	s.Stats.Throttles++

	if wake {
		m.comp.TickAt(p.ThrottleEnd)
	}

	return hintThrottle
}

// wakeThrottled queues the faults of the pages whose throttling has ended.
func (m *migrationMW) wakeThrottled() bool {
	s := m.state()
	now := m.now()
	madeProgress := false

	s.Throttling = slices.DeleteFunc(s.Throttling, func(pi int) bool {
		p := &s.Pages[pi]

		// A page that is serviced or queued considers its faults by itself.
		if p.Phase != pageIdle || p.Queued || len(p.Waiters) == 0 {
			return true
		}

		if now < p.ThrottleEnd {
			return false
		}

		m.enqueuePage(pi)

		madeProgress = true

		return true
	})

	return madeProgress
}

// pin pins a page where it is: the page table marks it pinned, so its walks
// no longer fault and the other GPUs access it remotely until the pin times
// out. The timeout is background work: it does not keep a simulation running
// that has nothing else to do.
func (m *migrationMW) pin(pi int, page vm.Page) {
	s := m.state()
	p := &s.Pages[pi]

	page.IsPinned = true
	m.comp.Resources.PageTable.Update(page)

	p.Pinned = true
	p.PinnedUntil = m.now() + nsToPS(m.spec().PinNS)
	s.Pins = append(s.Pins, pi)
	s.Stats.Pins++

	m.comp.TickAtBackground(p.PinnedUntil)
}

// expirePins starts the unpin service of the idle pages whose pin has timed
// out: every GPU holds the page, the pin is cleared, the translations of the
// page are shot down, and the page is released, so the next access of a GPU
// that does not have the page faults or is counted again.
func (m *migrationMW) expirePins() bool {
	s := m.state()
	now := m.now()
	madeProgress := false

	s.Pins = slices.DeleteFunc(s.Pins, func(pi int) bool {
		p := &s.Pages[pi]
		if now < p.PinnedUntil || p.Phase != pageIdle {
			return false
		}

		p.Service = serviceUnpin
		p.Targets = m.allGPUs()
		p.ServiceStart = now
		m.startHold(pi)

		madeProgress = true

		return true
	})

	return madeProgress
}

// clearPin ends the pin of a page and its thrashing, so that it can migrate
// again.
func (m *migrationMW) clearPin(pi int) {
	p := &m.state().Pages[pi]
	p.Pinned = false
	resetThrashing(p)

	page, ok := m.comp.Resources.PageTable.Find(p.PID, p.VAddr)
	if !ok {
		return
	}

	page.IsPinned = false
	m.comp.Resources.PageTable.Update(page)
}

// prefetchLog2Block is the size of the blocks in which pages are prefetched,
// the 2 MiB VA blocks of UVM (UVM_VA_BLOCK_BITS).
const prefetchLog2Block = 21

// prefetchLeaf is a page position of a block, as prefetching sees it:
// Counted if its page is on the destination or migrates to it, and Candidate
// if its page can move with the window.
type prefetchLeaf struct {
	Counted, Candidate bool
}

// prefetch returns the pages that the migration of a page to GPU dst also
// moves, by address. The page positions of the page's 2 MiB block form a
// binary tree; going up from the page, it selects the largest aligned region
// in which more than PrefetchThreshold percent of all the page positions are
// on dst or migrate to it, counting the page itself, as UVM does. The
// eligible pages of that region are prefetched. A thrashing page does not
// widen its window.
func (m *migrationMW) prefetch(pi, dst int) []int {
	spec := m.spec()
	log2PageSize := m.log2PageSize()
	trigger := m.state().Pages[pi]

	if spec.PrefetchThreshold == 0 || log2PageSize >= prefetchLog2Block ||
		m.thrashing(&trigger) {
		return nil
	}

	n := uint64(1) << (prefetchLog2Block - log2PageSize)
	first := trigger.VAddr &^ (uint64(1)<<prefetchLog2Block - 1)
	index := (trigger.VAddr - first) >> log2PageSize

	leaves := make([]prefetchLeaf, n)
	for i := range leaves {
		leaves[i] = m.prefetchLeaf(trigger.PID, first+uint64(i)<<log2PageSize, dst)
	}

	leaves[index] = prefetchLeaf{Counted: true}

	var start, end uint64

	for size := uint64(2); size <= n; size *= 2 {
		lo := index &^ (size - 1)
		counted := uint64(0)

		for _, l := range leaves[lo : lo+size] {
			if l.Counted {
				counted++
			}
		}

		if counted*100 > spec.PrefetchThreshold*size {
			start, end = lo, lo+size
		}
	}

	var members []int

	for i := start; i < end; i++ {
		if leaves[i].Candidate {
			members = append(members,
				m.pageIndex(trigger.PID, first+i<<log2PageSize))
		}
	}

	return members
}

// prefetchLeaf looks at the page position at vAddr. A position without a
// unified page counts for nothing but is a position of its region. A page
// counts if it is on the destination or migrates to it; a page whose service
// moves nothing, a release or an unpin, counts where it is. A page moves with
// a window if it is unified, on another GPU, not pinned, not thrashing, not
// serviced, not waited for by a fault of its own, and not used by a memory
// copy.
func (m *migrationMW) prefetchLeaf(pid vm.PID, vAddr uint64, dst int) prefetchLeaf {
	page, ok := m.comp.Resources.PageTable.Find(pid, vAddr)
	if !ok || !page.Unified {
		return prefetchLeaf{}
	}

	var p migrationPage
	if pi, ok := m.findPage(pid, vAddr); ok {
		p = m.state().Pages[pi]
	}

	// The GPU that has the page when its service, if any, has ended.
	owner := int(page.DeviceID)
	if p.Service == serviceMigrate {
		owner = p.Dst
	}

	if owner == dst {
		return prefetchLeaf{Counted: true}
	}

	if p.Phase != pageIdle {
		return prefetchLeaf{}
	}

	// As for a page that is asked for itself, thrashing that nothing has
	// added to for an epoch is over. Only this view of the page forgets it
	// here: the page itself does when it migrates.
	m.forgetThrashing(&p)

	return prefetchLeaf{
		Candidate: !page.IsPinned && m.isGPU(int(page.DeviceID)) &&
			len(p.Waiters) == 0 && !p.Queued && !m.thrashing(&p) &&
			!m.copyUses(pid, vAddr),
	}
}
