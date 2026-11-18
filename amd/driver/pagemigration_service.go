package driver

import (
	"fmt"
	"slices"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// serviceFaults considers the demand faults that wait on a page against the
// mapping and the pin of the page as they are now. It returns false if the
// faults must wait for a memory copy that uses the page.
func (m *migrationMW) serviceFaults(pi int) bool {
	s := m.state()
	p := &s.Pages[pi]
	p.Queued = false

	// A page that is serviced keeps its faults until its last Release
	// answer; they are queued again then.
	if p.Phase != pageIdle || len(p.Waiters) == 0 {
		return true
	}

	if m.copyUses(p.PID, p.VAddr) {
		p.Queued = true
		return false
	}

	page, found := m.comp.Resources.PageTable.Find(p.PID, p.VAddr)
	owner := int(page.DeviceID)

	if m.spec().Policy != PageMigrationFault || !found || !page.Unified || page.IsPinned || !m.isGPU(owner) {
		// Nothing can move: on the replay, the walk does not fault.
		m.startRelease(pi, slices.Clone(p.Waiters))
		return true
	}

	explicit := m.comp.State.PageTables.Views[p.PID][p.VAddr].Explicit
	for _, gpu := range m.requesters(p, owner) {
		if slices.Contains(explicit, gpu) {
			continue
		}
		switch m.hint(pi, gpu, true) {
		case hintMigrate:
			m.startWindow(pi, gpu)
			return true
		case hintPin:
			if len(m.comp.Spec.PageTables) > 0 {
				m.startPin(pi, page)
			} else {
				m.pin(pi, page)
				m.startRelease(pi, slices.Clone(s.Pages[pi].Waiters))
			}

			return true
		}
	}

	m.serviceThrottled(pi, owner, explicit)

	return true
}

// serviceThrottled ends the consideration of a page that does not move
// now: waiters that can use the resident or an advised remote mapping are
// released to replay. The throttled GPUs keep waiting,
// each for the end of its own nap. The request of the page has left the
// queue; wakeThrottled queues the page again when the first of those naps
// ends.
func (m *migrationMW) serviceThrottled(pi, owner int, explicit []int) {
	s := m.state()
	p := &s.Pages[pi]

	var replay []int
	for _, gpu := range p.Waiters {
		if gpu == owner || slices.Contains(explicit, gpu) {
			replay = append(replay, gpu)
		}
	}
	if len(replay) != 0 {
		if len(replay) < len(p.Waiters) {
			// The remaining faults keep their task while this subset replays.
			p.RetainedTrace = p.Trace
			p.Trace = m.traceStart(p.RetainedTrace.ID, "partial release")
		}
		m.startRelease(pi, replay)
		return
	}

	if len(p.Waiters) > 0 && !slices.Contains(s.Throttling, pi) {
		s.Throttling = append(s.Throttling, pi)
	}
}

// serviceNotification migrates the page that now maps the frame of an
// access-counter notification to the notifier. A frame that no page maps is
// skipped, and a frame that was allocated again is judged by the page that
// has it now. No access waits for a notification, so nothing is released
// when the page does not move.
func (m *migrationMW) serviceNotification(r migrationRequest) {
	s := m.state()

	page, ok := m.comp.Resources.PageTable.ReverseLookup(r.PAddr)
	if !ok {
		m.comp.TaskTracker().EndTask(r.Trace.ID)
		s.Stats.FreeFrameNotifications++
		return
	}

	if !page.Unified || page.IsPinned || int(page.DeviceID) == r.GPU ||
		!m.isGPU(int(page.DeviceID)) || m.copyUses(page.PID, page.VAddr) {
		m.comp.TaskTracker().EndTask(r.Trace.ID)
		s.Stats.IgnoredNotifications++
		return
	}

	pi := m.pageIndex(page.PID, page.VAddr)
	if s.Pages[pi].Phase != pageIdle {
		m.comp.TaskTracker().EndTask(r.Trace.ID)
		s.Stats.IgnoredNotifications++
		return
	}

	switch m.hint(pi, r.GPU, false) {
	case hintMigrate:
		m.useCounterTrace(pi, r.Trace)
		m.startWindow(pi, r.GPU)
	case hintPin:
		if len(m.comp.Spec.PageTables) > 0 {
			m.useCounterTrace(pi, r.Trace)
			m.startPin(pi, page)
		} else {
			m.comp.TaskTracker().EndTask(r.Trace.ID)
			m.pin(pi, page)
		}
	default:
		// The GPU is throttled: its remote accesses go on.
		m.comp.TaskTracker().EndTask(r.Trace.ID)
		s.Stats.ThrottledNotifications++
	}
}

// Counter service satisfies all existing waiters by holding and releasing
// every GPU. Their queued fault service is subsumed by this notification.
func (m *migrationMW) useCounterTrace(pi int, trace pageTrace) {
	p := &m.state().Pages[pi]
	m.comp.TaskTracker().EndTask(p.Trace.ID)
	p.Trace = trace
}

// startWindow starts the migration of a page to GPU dst and of the pages
// that are prefetched with it. The pages are serviced one by one; the window
// only fixes the order in which they are submitted to the data mover.
func (m *migrationMW) startWindow(pi, dst int) {
	s := m.state()
	members := m.prefetch(pi, dst)

	s.Windows = append(s.Windows, copyWindow{
		Dst:   dst,
		Pages: append([]int{pi}, members...),
	})
	s.Stats.Windows++

	m.startMigration(pi, dst, false)

	for _, mi := range members {
		s.Pages[mi].Trace = m.traceStart(s.Pages[pi].Trace.ID, "prefetch")
		m.startMigration(mi, dst, true)
	}
}

// startMigration starts moving a page to GPU dst: every GPU holds it first.
// Its new frame comes from the ordinary allocator of dst.
func (m *migrationMW) startMigration(pi, dst int, prefetched bool) {
	s := m.state()
	p := &s.Pages[pi]

	page, ok := m.comp.Resources.PageTable.Find(p.PID, p.VAddr)
	if !ok {
		panic(fmt.Sprintf("driver: migrating page %#x is not mapped", p.VAddr))
	}

	// Every GPU is held and released, so the faults that wait on the page
	// are answered by this service; a GPU that still needs the page faults
	// again after the release.
	p.Waiters = p.Waiters[:0]
	p.Service = serviceMigrate
	p.Dst = dst
	p.OldPage = page
	p.NewPAddr = m.comp.Resources.Runtime.AllocateFrame(dst)
	p.Prefetched = prefetched
	p.Targets = m.allGPUs()
	p.ServiceStart = m.now()

	m.recordMigration(pi)
	m.startHold(pi)
}

// startRelease starts a service that moves nothing: the GPUs that wait hold
// and then release the page. The Hold is needed because a fault notification
// reaches the driver directly while the fault response still travels through
// the TLBs: a Release that overtook the response would not clear the fault.
// The Hold answers only after the response has arrived.
func (m *migrationMW) startRelease(pi int, gpus []int) {
	p := &m.state().Pages[pi]

	p.Waiters = slices.DeleteFunc(p.Waiters, func(gpu int) bool {
		return slices.Contains(gpus, gpu)
	})
	p.Service = serviceRelease
	p.Targets = gpus
	p.ServiceStart = m.now()

	m.startHold(pi)
}

func (m *migrationMW) startHold(pi int) {
	p := &m.state().Pages[pi]
	m.tracePage(p)
	m.mark(p, tasks.MilestoneKindQueue, "page service admitted")
	p.Phase = pageHold

	m.sendToTargets(pi, protocol.PageHoldReq{
		PID: p.PID, VAddr: p.VAddr, PageSize: m.pageSize(),
	})
}

// ack takes in the answer of a command processor and advances its page once
// every command of the phase is answered.
func (m *migrationMW) ack(reqID uint64) {
	s := m.state()

	pi, ok := s.Pending[reqID]
	if !ok {
		panic(fmt.Sprintf("driver: unknown page-migration answer to %d", reqID))
	}

	delete(s.Pending, reqID)

	p := &s.Pages[pi]
	p.PendingAcks--

	if p.PendingAcks > 0 {
		return
	}

	m.markAck(p)
	switch p.Phase {
	case pageHold:
		m.held(pi)
	case pagePrepare:
		p.Phase = pageCopyWait
	case pageCopy:
		// The page is in the new frame. No GPU has used the frame since the
		// destination dropped its L2 lines: the frame is not mapped yet, and
		// the copy went around the L2.
		s.Stats.CopyTimeSum += uint64(m.now() - p.CopyStart)
		if page, ok := m.comp.Resources.PageTable.Find(p.PID, p.VAddr); ok && page == p.OldPage {
			m.remap(pi)
		}
		m.commitMapping(pi)
	case pageShootdown:
		s.Stats.ShootdownTimeSum += uint64(m.now() - p.ShootdownStart)
		m.startReleasePhase(pi)
	case pageRelease:
		m.finish(pi)
	default:
		panic(fmt.Sprintf("driver: page-migration answer in phase %d", p.Phase))
	}
}

// held continues a service once every target holds the page: no access to
// it is outstanding and no translation of it is in flight.
func (m *migrationMW) held(pi int) {
	p := &m.state().Pages[pi]

	switch p.Service {
	case serviceMigrate:
		m.startPrepare(pi)
	case servicePin:
		if page, ok := m.comp.Resources.PageTable.Find(p.PID, p.VAddr); ok && page == p.OldPage {
			m.pin(pi, page)
		}
		m.commitMapping(pi)
	case serviceUnpin:
		m.clearPin(pi)
		m.commitMapping(pi)
	default:
		m.startReleasePhase(pi)
	}
}

// startPrepare makes the two frames ready for the copy. The source writes
// the dirty L2 lines of the old frame back and drops its lines, so the
// memory below its L2 holds the page. The destination drops the L2 lines of
// the new frame: the frame may have been used before, and a dirty line left
// from then must not be written back over the copy. Its answer also waits
// for the lines of the frame that the L2 is writing back already, so no
// writeback of the frame is on its way when the copy starts. Nothing fills
// the L2 with lines of the new frame after that: the frame is mapped only
// after the copy, which goes around the L2.
func (m *migrationMW) startPrepare(pi int) {
	p := &m.state().Pages[pi]
	p.Phase = pagePrepare

	m.sendTo(pi, int(p.OldPage.DeviceID), protocol.PageFlushReq{
		PAddr: p.OldPage.PAddr, PageSize: p.OldPage.PageSize,
	})
	m.sendTo(pi, p.Dst, protocol.PageInvalidateReq{
		PAddr: p.NewPAddr, PageSize: p.OldPage.PageSize,
	})
}

// submitCopies sends the copies of the pages that are ready to the command
// processors of their destinations, one request per page: a page is always
// pulled by the data mover of the GPU that it moves to. On the data mover of
// a GPU, the pages of a window are submitted in the order of the window, and
// a window is submitted completely before the next one starts; a page then
// completes on its own, so pages of several windows are in flight together.
func (m *migrationMW) submitCopies() bool {
	s := m.state()
	madeProgress := false
	blocked := make([]bool, m.numGPUs()+1)

	for wi := range s.Windows {
		w := &s.Windows[wi]
		if blocked[w.Dst] {
			continue
		}

		for w.Next < len(w.Pages) && s.Pages[w.Pages[w.Next]].Phase == pageCopyWait {
			m.submitCopy(w.Pages[w.Next])
			w.Next++

			madeProgress = true
		}

		blocked[w.Dst] = w.Next < len(w.Pages)
	}

	s.Windows = slices.DeleteFunc(s.Windows, func(w copyWindow) bool {
		return w.Next == len(w.Pages)
	})

	return madeProgress
}

func (m *migrationMW) submitCopy(pi int) {
	p := &m.state().Pages[pi]
	m.mark(p, tasks.MilestoneKindQueue, "copy window ready")
	p.Phase = pageCopy
	p.CopyStart = m.now()

	m.sendTo(pi, p.Dst, protocol.PageCopyReq{
		SrcAddress: p.OldPage.PAddr,
		DstAddress: p.NewPAddr,
		PageSize:   p.OldPage.PageSize,
	})
}

// remap points the page table to the new frame once the copy is done. No GPU
// translates the page while it is held, so no TLB can be filled with the old
// mapping after this point. The old frame is retired: it is not returned to
// the allocator, so nothing that still names it, such as an L1 line, can
// meet another page.
func (m *migrationMW) remap(pi int) {
	s := m.state()
	p := &s.Pages[pi]

	page := p.OldPage
	page.PAddr = p.NewPAddr
	page.DeviceID = uint64(p.Dst)
	m.comp.Resources.PageTable.Update(page)

	s.Stats.PagesMigrated++
	s.Stats.BytesMigrated += page.PageSize
	s.Stats.FramesRetired++

	if p.Prefetched {
		s.Stats.PagesPrefetched++
	} else {
		s.Stats.Migrations++
	}
}

func (m *migrationMW) startPin(pi int, page vm.Page) {
	p := &m.state().Pages[pi]
	p.Service = servicePin
	p.OldPage = page
	p.Waiters = nil
	p.ServiceStart = m.now()
	p.Targets = m.allGPUs()
	m.startHold(pi)
}

// commitMapping runs under the all-GPU Hold after copy or pin maintenance.
// The new physical page can become visible only after every MMU responds.
func (m *migrationMW) commitMapping(pi int) {
	if len(m.comp.Spec.PageTables) == 0 {
		m.startShootdown(pi)
		return
	}
	p := &m.state().Pages[pi]
	p.Phase = pageMapping
	table := m.comp.Middlewares.PageTable
	view := m.comp.State.PageTables.Views[p.PID][p.VAddr]
	after, _ := m.comp.Resources.PageTable.Find(p.PID, p.VAddr)
	table.publish(mappingGroup{PID: p.PID, VAddr: p.VAddr}, view.withPage(after), p.Trace.ID)
	m.mappingsReady(pi)
}

// Advice and host remaps can arrive while the page is held. Join the host
// streams before issuing shootdown, and again before releasing any GPU.
func (m *migrationMW) mappingsReady(pi int) {
	p := &m.state().Pages[pi]
	if m.comp.State.PageTables.PendingPages[p.PID][p.VAddr] != 0 {
		return
	}
	switch p.Phase {
	case pageMapping:
		m.mark(p, tasks.MilestoneKindTranslation, "page tables committed")
		m.startShootdown(pi)
	case pageWaitReleaseMappings:
		m.mark(p, tasks.MilestoneKindTranslation, "outstanding mappings committed before release")
		m.startReleasePhase(pi)
	}
}

func (m *migrationMW) startShootdown(pi int) {
	p := &m.state().Pages[pi]
	p.Phase = pageShootdown
	p.ShootdownStart = m.now()

	m.sendToTargets(pi, protocol.PageShootdownReq{
		PID: p.PID, VAddr: p.VAddr, PageSize: m.pageSize(),
	})
}

func (m *migrationMW) startReleasePhase(pi int) {
	p := &m.state().Pages[pi]
	if m.comp.State.PageTables.PendingPages[p.PID][p.VAddr] != 0 {
		p.Phase = pageWaitReleaseMappings
		return
	}
	p.Phase = pageRelease

	m.sendToTargets(pi, protocol.PageReleaseReq{
		PID: p.PID, VAddr: p.VAddr, PageSize: m.pageSize(),
	})
}

// finish ends the service of a page with its last Release answer. Only now
// are the faults that arrived during the service considered again, against
// the mapping that the service left.
func (m *migrationMW) finish(pi int) {
	s := m.state()
	p := &s.Pages[pi]

	switch p.Service {
	case serviceMigrate:
		s.Stats.MigrationTimeSum += uint64(m.now() - p.ServiceStart)
	case serviceUnpin:
		s.Stats.Unpins++
	default:
		s.Stats.Releases++
	}

	m.comp.TaskTracker().EndTask(p.Trace.ID)
	p.Trace, p.RetainedTrace = p.RetainedTrace, pageTrace{}
	p.Phase = pageIdle
	p.Service = serviceNone
	p.Prefetched = false
	p.Targets = nil
	p.OldPage = vm.Page{}
	p.NewPAddr = 0

	if len(p.Waiters) > 0 {
		if p.Trace.ID == 0 {
			p.Trace = m.traceStart(0, "waiting fault")
		}
		m.enqueuePage(pi)
	}
}
