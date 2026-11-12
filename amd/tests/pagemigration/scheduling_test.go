package pagemigration

import (
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
)

// span is the time that something was seen in the samples of a watcher.
type span struct {
	first, last timing.VTimeInPicoSec
}

// watcher samples a running system every nanosecond: the windows of the
// driver, the page copies in the inbound data mover of a GPU, and the Hold
// and Release commands that the command processors work on.
type watcher struct {
	s   *system
	gpu int // the GPU whose inbound data mover is watched

	windows  [][]uint64      // the old frames of the pages of each window, in order
	admitted []uint64        // the source frames of the copies, in the order the mover took them
	inMover  map[uint64]span // by source frame
	held     map[uint64]span // Hold jobs, by virtual address
	released map[uint64]span // Release jobs, by virtual address

	// overlaps are the pages for which a command processor worked on a Hold
	// while a Release of an earlier service was still not answered.
	overlaps []uint64

	knownWindows map[uint64]bool // by the first frame of a window
	cps          []*cp.Comp
}

func watch(s *system, gpu int) *watcher {
	w := &watcher{
		s: s, gpu: gpu,
		inMover:      map[uint64]span{},
		held:         map[uint64]span{},
		released:     map[uint64]span{},
		knownWindows: map[uint64]bool{},
	}

	for _, c := range s.sim.Components() {
		if c, ok := c.(*cp.Comp); ok {
			w.cps = append(w.cps, c)
		}
	}

	return w
}

func see(spans map[uint64]span, key uint64, now timing.VTimeInPicoSec) {
	sp, ok := spans[key]
	if !ok {
		sp.first = now
	}

	sp.last = now
	spans[key] = sp
}

func (w *watcher) sample() {
	now := w.s.engine.CurrentTime()
	m := w.s.driver.State.PageMigration

	for _, win := range m.Windows {
		first := m.Pages[win.Pages[0]].OldPage.PAddr
		if w.knownWindows[first] {
			continue
		}

		w.knownWindows[first] = true

		var frames []uint64
		for _, pi := range win.Pages {
			frames = append(frames, m.Pages[pi].OldPage.PAddr)
		}

		w.windows = append(w.windows, frames)
	}

	for _, t := range w.s.inbound[w.gpu-1].State.Transactions {
		if _, ok := w.inMover[t.SrcAddress]; !ok {
			w.admitted = append(w.admitted, t.SrcAddress)
		}

		see(w.inMover, t.SrcAddress, now)
	}

	w.sampleJobs(now)
}

func (w *watcher) sampleJobs(now timing.VTimeInPicoSec) {
	holding, releasing := map[uint64]bool{}, map[uint64]bool{}

	for _, c := range w.cps {
		for _, job := range c.State.PageJobs {
			switch req := job.Req.Payload.(type) {
			case protocol.PageHoldReq:
				holding[req.VAddr] = true
				see(w.held, req.VAddr, now)
			case protocol.PageReleaseReq:
				releasing[req.VAddr] = true
				see(w.released, req.VAddr, now)
			}
		}
	}

	for vAddr := range holding {
		if releasing[vAddr] && !slices.Contains(w.overlaps, vAddr) {
			w.overlaps = append(w.overlaps, vAddr)
		}
	}
}

// A window is sparse when its region has positions without a page: the
// window takes the pages that exist, also those of another allocation, and
// the position without a page still counts as a position of the region.
func TestASparseWindowTakesThePagesThatExist(t *testing.T) {
	s := newSystem(t, "r9nano")

	const group = 8

	// Three allocations fill an aligned group of eight pages; the middle
	// one, the seventh page of the group, is freed again.
	first := s.alignedUnified(6, group)
	hole := s.unified(s.pageSize)
	last := s.unified(s.pageSize)

	if uint64(last-first) != 7*s.pageSize {
		t.Fatalf("the allocations are not one group: %#x, %#x, %#x", first, hole, last)
	}

	data := ramp(1, groupFloats)
	s.upload(last, data)
	s.free(hole)

	// GPU 2 touches the first three pages one by one; the third touch takes
	// the fourth page along. A touch of the fifth page then makes five of
	// the eight positions of the group, the freed one included: it takes
	// the sixth page and the eighth, which is another allocation, along.
	sink := s.local(2, groupFloats*floatBytes)
	for page := range uint64(3) {
		s.touch(2, sink, s.at(first, page, 0), groupFloats)
		s.run()
	}

	mustEqualOwners(t, "placement", s.owners(first, 6), 2, 2, 2, 2, 1, 1)

	before := s.stats()
	s.touch(2, sink, s.at(first, 4, 0), groupFloats)
	s.run()

	mustEqualOwners(t, "the sparse window", s.owners(first, 6), 2, 2, 2, 2, 2, 2)
	mustEqualOwners(t, "the page of the other allocation", s.owners(last, 1), 2)

	st := s.stats()
	if st.Windows != before.Windows+1 || st.PagesPrefetched != before.PagesPrefetched+2 {
		t.Fatalf("the fault did not take a window of three pages: before %+v, after %+v", before, st)
	}

	mustEqualFloats(t, "the prefetched page", s.download(last, groupFloats), data)
}

// The scheduling of two windows on the inbound data mover of GPU 2, with
// real kernels on R9 Nano. Five pages of an aligned group of eight are on
// GPU 2, so the fault on the sixth takes the two that are left along: a
// window of three pages, A, B, and C. A fault on a page of the next group,
// X, is a window of its own. A is submitted first, then B and C by address,
// and X only after C; X enters the data mover while C is still being copied.
// GPU 2 also faults on B and C themselves while they migrate: those faults
// wait for the last Release answer of their page.
//
//nolint:funlen // Keep the placement of the pages and the scheduling checks together.
func TestTwoWindowsOnTheInboundDataMover(t *testing.T) {
	s := newSystem(t, "r9nano")

	const group = 8

	pageFloats := s.pageSize / floatBytes
	buffer := s.alignedUnified(group+1, group)
	data := ramp(1, int((group+1)*pageFloats))
	s.upload(buffer, data)

	// GPU 2 touches four pages one by one. The third touch prefetches the
	// fourth page of its group of four; five pages are on GPU 2 then.
	sink := s.local(2, groupFloats*floatBytes)
	for _, page := range []uint64{6, 0, 1, 2} {
		s.touch(2, sink, s.at(buffer, page, 0), groupFloats)
		s.run()
	}

	mustEqualOwners(t, "placement", s.owners(buffer, group+1), 2, 2, 2, 2, 1, 1, 2, 1, 1)

	// One kernel of GPU 2 reads from the end of page 4 to the start of page
	// 8. Pages 4, 5, and 7 are not on GPU 2: the one that faults first is A
	// and prefetches the other two, B and C by address. Page 8, which the
	// last work-group reads, is X.
	const floats = groupFloats + 3*1024 + groupFloats

	result := s.local(2, floats*floatBytes)
	start := s.at(buffer, 4, pageFloats-groupFloats)
	frames := make([]uint64, group+1)

	for p := range frames {
		frames[p] = s.page(s.at(buffer, uint64(p), 0)).PAddr
	}

	before := s.stats()
	w := watch(s, 2)

	s.touch(2, result, start, floats)
	s.runWatching(w.sample)

	// The windows: the trigger, then the prefetched pages by address; and X.
	x := frames[8]
	if len(w.windows) != 2 || len(w.windows[0]) != 3 || !slices.Equal(w.windows[1], []uint64{x}) {
		t.Fatalf("windows %#x, want three of the group and [%#x]", w.windows, x)
	}

	a, b, c := w.windows[0][0], w.windows[0][1], w.windows[0][2]
	if !slices.Contains([]uint64{frames[4], frames[5], frames[7]}, a) || b >= c ||
		!slices.Equal(slices.Sorted(slices.Values(w.windows[0])), []uint64{frames[4], frames[5], frames[7]}) {
		t.Fatalf("first window %#x is not a trigger and the rest of the group by address", w.windows[0])
	}

	// The data mover took the pages in exactly that order.
	if !slices.Equal(w.admitted, []uint64{a, b, c, x}) {
		t.Fatalf("the data mover took %#x, want %#x", w.admitted, []uint64{a, b, c, x})
	}

	// X entered the data mover after C and while C was still in it, and
	// pages of both windows were in flight together.
	if w.inMover[x].first < w.inMover[c].first || w.inMover[x].first > w.inMover[c].last {
		t.Fatalf("X entered at %d; C was in the data mover from %d to %d",
			w.inMover[x].first, w.inMover[c].first, w.inMover[c].last)
	}

	// The faults of GPU 2 on B and C arrived while the pages migrated. They
	// were kept, and their Hold was not sent before the last Release answer
	// of the migration.
	st := s.stats()
	if st.RetainedFaults == before.RetainedFaults || st.Releases == before.Releases {
		t.Fatalf("no fault waited for a page that migrated: %+v", st)
	}

	if len(w.overlaps) != 0 {
		t.Fatalf("pages %#x were held again before their last Release answer", w.overlaps)
	}

	vC := uint64(s.at(buffer, uint64(slices.Index(frames, c)), 0))
	if w.held[vC].last <= w.released[vC].first {
		t.Fatalf("the fault on C was not serviced after the Release: held until %d, released from %d",
			w.held[vC].last, w.released[vC].first)
	}

	mustEqualOwners(t, "after the kernel", s.owners(buffer, group+1), 2, 2, 2, 2, 2, 2, 2, 2, 2)

	first := 4*pageFloats + pageFloats - groupFloats
	mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data[first:first+floats])
	t.Logf("windows %#x; in the data mover: A %v, B %v, C %v, X %v; %+v",
		w.windows, w.inMover[a], w.inMover[b], w.inMover[c], w.inMover[x], st)
}
