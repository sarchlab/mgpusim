package driver

import (
	"fmt"
	"log"
	"reflect"
	"slices"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// Page-migration policies. With PageMigrationFault, the first access of a GPU
// to a unified page on another GPU faults (sarchlab/akita#589). With
// PageMigrationCounter, the page stays mapped remotely and the RDMA engines'
// access counters request the migrations (sarchlab/akita#590).
const (
	PageMigrationNone    = "none"
	PageMigrationFault   = "fault"
	PageMigrationCounter = "counter"
)

// PageMigrationSpec holds the policy and the tuning parameters of page
// migration between GPUs. A page is always a page of the system page size:
// faults, access counters, prefetching, thrashing, and pins all work on it.
// Times are in nanoseconds.
type PageMigrationSpec struct {
	// Policy is PageMigrationFault (the default), PageMigrationCounter, or
	// PageMigrationNone.
	Policy string `json:"policy"`

	// The driver starts a batch as soon as requests wait: it services the
	// requests that wait at that moment, at most BatchSize
	// (uvm_perf_fault_batch_count), and never waits to fill a batch.
	//
	// QueueSize bounds what the driver takes in. A fault or a notification
	// that needs an entry of the queue is taken from the port only while the
	// queue has fewer than QueueSize entries. Otherwise it stays in the
	// port, with everything that arrived behind it, and so at the senders:
	// nothing is dropped. A fault that waits for a memory copy keeps its
	// entry, so it counts; it takes no place in a batch, so the requests
	// that are in the queue behind it are serviced. The only entries beyond
	// QueueSize are faults that the driver has taken in before and that it
	// queues again by itself: the faults that arrived while their page was
	// serviced, when that service ends, and the faults of throttled GPUs,
	// which left the queue when the GPUs were throttled, when the nap of one
	// of them ends. That is at most one entry per page.
	BatchSize int `json:"batch_size"`
	QueueSize int `json:"queue_size"`

	// AccessCounterThreshold is the number of remote accesses of a GPU to a
	// physical page frame that make its RDMA engine notify the driver
	// (uvm_perf_access_counter_threshold); 0 turns the counters off. The
	// platform sets it in the RDMA engines.
	AccessCounterThreshold uint64 `json:"access_counter_threshold"`

	// A migration also moves, or prefetches, the eligible pages of the
	// largest aligned region of its 2 MiB block in which more than
	// PrefetchThreshold percent of the page positions are on the destination
	// or migrate to it (uvm_perf_prefetch_threshold, sarchlab/akita#591); 0
	// turns prefetching off.
	PrefetchThreshold uint64 `json:"prefetch_threshold"`

	// A migration of a page within ThrashingLapseNS of its previous
	// migration is a thrashing event (uvm_perf_thrashing_lapse_usec); a page
	// with ThrashingThreshold events is thrashing
	// (uvm_perf_thrashing_threshold). A GPU that asks for a thrashing page
	// is throttled for ThrashingNapNS (uvm_perf_thrashing_nap) unless it is
	// the GPU that may make progress; the nap of each GPU starts when that
	// GPU is throttled. After PinThreshold throttling
	// operations in a row (uvm_perf_thrashing_pin_threshold), the page is
	// pinned where it is for PinNS (uvm_perf_thrashing_pin). A page forgets
	// its thrashing when nothing happens to it for ThrashingEpochNS
	// (uvm_perf_thrashing_epoch).
	ThrashingThreshold int    `json:"thrashing_threshold"`
	ThrashingLapseNS   uint64 `json:"thrashing_lapse_ns"`
	ThrashingNapNS     uint64 `json:"thrashing_nap_ns"`
	ThrashingEpochNS   uint64 `json:"thrashing_epoch_ns"`
	PinThreshold       int    `json:"pin_threshold"`
	PinNS              uint64 `json:"pin_ns"`

	// DataMoverPages is the number of page copies of one direction that a
	// data mover of a GPU keeps in flight. The platform sets it in the data
	// movers.
	DataMoverPages int `json:"data_mover_pages"`
}

// DefaultPageMigrationSpec returns the default parameters, which follow the
// NVIDIA UVM driver: pages migrate on faults.
func DefaultPageMigrationSpec() PageMigrationSpec {
	return DefaultSpec().PageMigration
}

// Enabled reports whether a policy migrates pages.
func (s PageMigrationSpec) Enabled() bool {
	return s.Policy == PageMigrationFault || s.Policy == PageMigrationCounter
}

func (s PageMigrationSpec) validate(numGPUs, numAgents int) error {
	switch s.Policy {
	case "", PageMigrationNone:
		return nil
	case PageMigrationFault, PageMigrationCounter:
	default:
		return fmt.Errorf("driver: unknown page-migration policy %q", s.Policy)
	}

	if s.PrefetchThreshold > 100 {
		return fmt.Errorf("driver: prefetch threshold %d%% is above 100%%",
			s.PrefetchThreshold)
	}

	if s.BatchSize <= 0 || s.QueueSize <= 0 || s.DataMoverPages <= 0 {
		return fmt.Errorf(
			"driver: batch size, queue size, and data-mover pages must be positive")
	}

	if s.ThrashingThreshold <= 0 || s.PinThreshold <= 0 {
		return fmt.Errorf("driver: thrashing and pin thresholds must be positive")
	}

	if numAgents != numGPUs {
		return fmt.Errorf("driver: %d migration agents for %d GPUs",
			numAgents, numGPUs)
	}

	return nil
}

func nsToPS(ns uint64) timing.VTimeInPicoSec {
	return timing.VTimeInPicoSec(ns * 1000)
}

// Phases of the service of a page.
const (
	pageIdle       = iota // nothing to do
	pageHold              // the GPUs hold the page
	pagePrepare           // the source flushes the old frame, the destination drops the new one
	pageCopyWait          // waits for its turn at the destination's data mover
	pageCopy              // the destination's data mover copies
	pageInvalidate        // the destination drops the L2 lines of the new frame
	pageShootdown         // the GPUs drop the old translation
	pageRelease           // the GPUs release the page
)

// Services of a page.
const (
	serviceNone    = iota
	serviceMigrate // hold, flush, copy, invalidate, remap, shoot down, release
	serviceRelease // hold and release: nothing moves
	serviceUnpin   // hold, clear the pin, shoot down, release
)

// migrationPage is a page and where it is in its service. A page is serviced
// on its own: it never waits for another page, except for its turn at a data
// mover.
type migrationPage struct {
	PID   vm.PID `json:"pid"`
	VAddr uint64 `json:"vaddr"`

	// Phase and Service tell what is done with the page. Targets are the
	// GPUs that hold and release it. A migration moves OldPage to the frame
	// NewPAddr of GPU Dst; Prefetched is set when the page moves with the
	// window of another page. PendingAcks counts the answers of the phase
	// that have not arrived.
	Phase       int     `json:"phase"`
	Service     int     `json:"service"`
	Dst         int     `json:"dst"`
	Targets     []int   `json:"targets"`
	OldPage     vm.Page `json:"old_page"`
	NewPAddr    uint64  `json:"new_paddr"`
	Prefetched  bool    `json:"prefetched"`
	PendingAcks int     `json:"pending_acks"`

	// Waiters are the GPUs that have a demand fault on the page that is not
	// serviced yet, each once, in arrival order. A fault that arrives while
	// the page is serviced waits here until the last Release answer of the
	// service. Queued is set while the page waits in the queue.
	Waiters []int `json:"waiters"`
	Queued  bool  `json:"queued"`

	// Thrashing. Events counts the thrashing events, up to the threshold.
	// Throttled are the GPUs that are throttled, each with the end of its
	// own nap, in the order they were throttled. ThrottleCount counts the
	// throttling operations in a row, and Selected is the GPU that may make
	// progress while the others are throttled (0: none).
	HasEvent      bool                  `json:"has_event"`
	LastEvent     timing.VTimeInPicoSec `json:"last_event"`
	LastActive    timing.VTimeInPicoSec `json:"last_active"`
	Events        int                   `json:"events"`
	Throttled     []throttledGPU        `json:"throttled"`
	ThrottleCount int                   `json:"throttle_count"`
	Selected      int                   `json:"selected"`
	Pinned        bool                  `json:"pinned"`
	PinnedUntil   timing.VTimeInPicoSec `json:"pinned_until"`

	ServiceStart   timing.VTimeInPicoSec `json:"service_start"`
	CopyStart      timing.VTimeInPicoSec `json:"copy_start"`
	ShootdownStart timing.VTimeInPicoSec `json:"shootdown_start"`
}

// throttledGPU is a GPU that asked for a thrashing page and must wait until
// Until, the end of its nap, before it may take the page. The nap is its
// own: it starts when the GPU is throttled, and a GPU that is throttled on
// the page earlier or later neither shortens nor extends it.
type throttledGPU struct {
	GPU   int                   `json:"gpu"`
	Until timing.VTimeInPicoSec `json:"until"`
}

// napEnd returns the end of the nap of a GPU that is throttled on the page.
func (p *migrationPage) napEnd(gpu int) (until timing.VTimeInPicoSec, throttled bool) {
	for _, t := range p.Throttled {
		if t.GPU == gpu {
			return t.Until, true
		}
	}

	return 0, false
}

// migrationRequest is a request that waits in the queue: the demand faults
// of a page (its Waiters), or, if Counter is set, the access-counter
// notification of GPU for the physical page frame at PAddr. Deferred is set
// on faults that a batch passed over because a memory copy uses their page.
type migrationRequest struct {
	Counter  bool   `json:"counter"`
	Page     int    `json:"page"`
	PAddr    uint64 `json:"paddr"`
	GPU      int    `json:"gpu"`
	Deferred bool   `json:"deferred"`
}

// copyWindow is the pages that one fault or notification moves to GPU Dst, in
// the order they are submitted to its inbound data mover: the triggering
// page, then the prefetched pages by address. Next is the number of pages
// submitted so far.
type copyWindow struct {
	Dst   int   `json:"dst"`
	Pages []int `json:"pages"`
	Next  int   `json:"next"`
}

// PageMigrationStats are the migration statistics. Times are in picoseconds.
type PageMigrationStats struct {
	// Faults are the fault notifications. A coalesced fault repeats a fault
	// that already waits; a retained fault arrived while its page was
	// serviced; a deferred fault waited for a memory copy.
	Faults          uint64 `json:"faults"`
	CoalescedFaults uint64 `json:"coalesced_faults"`
	RetainedFaults  uint64 `json:"retained_faults"`
	DeferredFaults  uint64 `json:"deferred_faults"`

	// Notifications are the access-counter notifications. A notification is
	// ignored when its page does not move: it is on the notifier, serviced,
	// pinned, or not unified. FreeFrameNotifications name a frame that no
	// page maps, and ThrottledNotifications a throttled GPU.
	Notifications          uint64 `json:"notifications"`
	CoalescedNotifications uint64 `json:"coalesced_notifications"`
	IgnoredNotifications   uint64 `json:"ignored_notifications"`
	FreeFrameNotifications uint64 `json:"free_frame_notifications"`
	ThrottledNotifications uint64 `json:"throttled_notifications"`

	// QueueFullWaits counts the times a fault or a notification stayed in
	// the port because the queue was full.
	QueueFullWaits uint64 `json:"queue_full_waits"`

	// Migrations are the pages moved for a fault or a notification and
	// PagesPrefetched the pages moved with them; PagesMigrated is both.
	// Releases are the services that moved nothing.
	Batches         uint64 `json:"batches"`
	Windows         uint64 `json:"windows"`
	Migrations      uint64 `json:"migrations"`
	PagesPrefetched uint64 `json:"pages_prefetched"`
	PagesMigrated   uint64 `json:"pages_migrated"`
	BytesMigrated   uint64 `json:"bytes_migrated"`
	FramesRetired   uint64 `json:"frames_retired"`
	Releases        uint64 `json:"releases"`

	ThrashingEvents uint64 `json:"thrashing_events"`
	ThrashingPages  uint64 `json:"thrashing_pages"`
	Throttles       uint64 `json:"throttles"`
	Pins            uint64 `json:"pins"`
	Unpins          uint64 `json:"unpins"`

	MigrationTimeSum uint64 `json:"migration_time_sum"`
	CopyTimeSum      uint64 `json:"copy_time_sum"`
	ShootdownTimeSum uint64 `json:"shootdown_time_sum"`
}

// PageMigrationState is the state of the page-migration middleware.
type PageMigrationState struct {
	// Pages are the pages that were ever faulted on, notified, or moved;
	// PageIndex finds a page by its process and its virtual page number.
	Pages     []migrationPage           `json:"pages"`
	PageIndex map[vm.PID]map[uint64]int `json:"page_index"`

	// Queue holds the requests that wait for a batch, in arrival order.
	Queue []migrationRequest `json:"queue"`

	// Windows are the windows whose pages are not all submitted to a data
	// mover yet, oldest first.
	Windows []copyWindow `json:"windows"`

	// Throttling are the idle pages whose waiting GPUs are all throttled,
	// and Pins the pinned pages.
	Throttling []int `json:"throttling"`
	Pins       []int `json:"pins"`

	// Pending maps the ID of each command in flight to its page. Out holds
	// the commands waiting to be sent.
	Pending map[uint64]int  `json:"pending"`
	Out     []messaging.Msg `json:"out"`

	Stats PageMigrationStats `json:"stats"`
}

func newPageMigrationState() PageMigrationState {
	return PageMigrationState{
		PageIndex: make(map[vm.PID]map[uint64]int),
		Pending:   make(map[uint64]int),
	}
}

// migrationMW is the page-migration service of the driver. It receives the
// MMU's fault notifications and the RDMA engines' access-counter
// notifications, decides in batches what each page does, and then runs every
// page on its own with commands to the GPUs' command processors: hold the
// page on every GPU; flush the old frame out of the source's L2 and drop the
// new frame from the destination's; copy with the destination's inbound data
// mover, around both L2s; drop the new frame from the destination's L2
// again; update the page table; shoot down the old translation; and release
// the page.
type migrationMW struct{ comp *Comp }

func (m *migrationMW) Handle(timing.Event) bool { return m.Tick() }

func (m *migrationMW) state() *PageMigrationState {
	return &m.comp.State.PageMigration
}

func (m *migrationMW) spec() PageMigrationSpec {
	return m.comp.Spec.PageMigration
}

func (m *migrationMW) now() timing.VTimeInPicoSec {
	return m.comp.CurrentTime()
}

func (m *migrationMW) log2PageSize() uint64 {
	return m.comp.Resources.PageTable.Log2PageSize()
}

func (m *migrationMW) pageSize() uint64 {
	return uint64(1) << m.log2PageSize()
}

func (m *migrationMW) numGPUs() int {
	return len(m.comp.Spec.MigrationAgents)
}

func (m *migrationMW) isGPU(dev int) bool {
	return dev >= 1 && dev <= m.numGPUs()
}

// Tick takes in the answers and the notifications, wakes the pages whose
// throttling or pin has ended, services a batch, and sends the commands.
func (m *migrationMW) Tick() bool {
	if !m.spec().Enabled() {
		return false
	}

	madeProgress := m.receiveAnswers()
	madeProgress = m.receiveNotifications() || madeProgress
	madeProgress = m.wakeThrottled() || madeProgress
	madeProgress = m.expirePins() || madeProgress
	madeProgress = m.serviceBatch() || madeProgress
	madeProgress = m.submitCopies() || madeProgress
	madeProgress = m.send() || madeProgress

	return madeProgress
}

func (m *migrationMW) send() bool {
	s := m.state()
	port := m.comp.Ports.PageMigration
	madeProgress := false

	for len(s.Out) > 0 && port.CanSend() {
		port.Send(s.Out[0])
		s.Out[0] = messaging.Msg{}
		s.Out = s.Out[1:]
		madeProgress = true
	}

	return madeProgress
}

// receiveAnswers takes in every answer of the command processors.
func (m *migrationMW) receiveAnswers() bool {
	port := m.comp.Ports.PageMigration
	madeProgress := false

	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			return madeProgress
		}

		if _, ok := msg.Payload.(protocol.GeneralRsp); !ok {
			log.Panicf("driver: cannot handle %s on PageMigration",
				reflect.TypeOf(msg.Payload))
		}

		m.ack(msg.RspTo)

		madeProgress = true
	}
}

// receiveNotifications takes in the fault and access-counter notifications,
// in the order of the port. None is ever dropped: a notification that needs
// an entry of a full queue stays at the head of the port, the ones behind it
// stay behind it, whatever page they are for, and their senders keep the
// rest. The queue is full also when its entries wait for a memory copy.
func (m *migrationMW) receiveNotifications() bool {
	s := m.state()
	port := m.comp.Ports.MigrationNotification
	madeProgress := false

	for {
		msg, ok := port.PeekIncoming()
		if !ok {
			return madeProgress
		}

		accepted := false

		switch payload := msg.Payload.(type) {
		case addrtransprotocol.PageFault:
			accepted = m.acceptFault(payload)
		case protocol.AccessCounterNotification:
			accepted = m.acceptNotification(payload)
		default:
			log.Panicf("driver: cannot handle %s on MigrationNotification",
				reflect.TypeOf(msg.Payload))
		}

		if !accepted {
			s.Stats.QueueFullWaits++
			return madeProgress
		}

		port.RetrieveIncoming()

		madeProgress = true
	}
}

func (m *migrationMW) queueFull() bool {
	return len(m.state().Queue) >= m.spec().QueueSize
}

// acceptFault records the demand fault of a GPU on a page. A fault of a GPU
// that already waits on the page is the same fault. While the page is
// serviced, in any phase, the fault waits on the page for the last Release
// answer. It returns false if the fault needs a place in a full queue.
func (m *migrationMW) acceptFault(f addrtransprotocol.PageFault) bool {
	s := m.state()
	gpu := int(f.DeviceID)
	pi := m.pageIndex(f.PID, f.VAddr)
	p := &s.Pages[pi]

	if slices.Contains(p.Waiters, gpu) {
		s.Stats.Faults++
		s.Stats.CoalescedFaults++

		return true
	}

	if p.Phase == pageIdle && !p.Queued {
		if m.queueFull() {
			return false
		}

		m.enqueuePage(pi)
	}

	p.Waiters = append(p.Waiters, gpu)
	s.Stats.Faults++

	if p.Phase != pageIdle {
		s.Stats.RetainedFaults++
	}

	return true
}

// acceptNotification queues an access-counter notification. The frame is
// resolved when the notification is serviced. It returns false if the
// notification needs a place in a full queue.
func (m *migrationMW) acceptNotification(
	n protocol.AccessCounterNotification,
) bool {
	s := m.state()
	r := migrationRequest{
		Counter: true,
		PAddr:   n.PAddr &^ (m.pageSize() - 1),
		GPU:     int(n.DeviceID),
	}

	if slices.Contains(s.Queue, r) {
		s.Stats.Notifications++
		s.Stats.CoalescedNotifications++

		return true
	}

	if m.queueFull() {
		return false
	}

	s.Queue = append(s.Queue, r)
	s.Stats.Notifications++

	return true
}

// enqueuePage queues the demand faults of a page. QueueSize bounds what is
// taken in from the port, and acceptFault checks it. The driver queues a
// page again by itself also when the queue is full, because its faults were
// taken in before and are never lost: a page whose service has ended with
// faults that arrived meanwhile, and a page on which the nap of a throttled
// GPU has ended. A page has at most one entry.
func (m *migrationMW) enqueuePage(pi int) {
	s := m.state()
	s.Pages[pi].Queued = true
	s.Queue = append(s.Queue, migrationRequest{Page: pi})
}

// pageIndex returns the index of the page of vAddr in a process, creating
// it. The pages may move in memory when one is created.
func (m *migrationMW) pageIndex(pid vm.PID, vAddr uint64) int {
	s := m.state()

	if i, ok := m.findPage(pid, vAddr); ok {
		return i
	}

	if s.PageIndex[pid] == nil {
		s.PageIndex[pid] = make(map[uint64]int)
	}

	s.Pages = append(s.Pages, migrationPage{
		PID: pid, VAddr: vAddr &^ (m.pageSize() - 1),
	})
	s.PageIndex[pid][vAddr>>m.log2PageSize()] = len(s.Pages) - 1

	return len(s.Pages) - 1
}

// findPage returns the page of vAddr in a process, if it was ever created.
func (m *migrationMW) findPage(pid vm.PID, vAddr uint64) (int, bool) {
	i, ok := m.state().PageIndex[pid][vAddr>>m.log2PageSize()]
	return i, ok
}

// serviceBatch services the requests that wait, in arrival order, at most a
// batch of them. It starts as soon as a request waits and takes what is
// there. The faults of a page that a memory copy uses are not serviced: they
// keep their entry of the queue until the copy ends, and neither hold back
// the requests that are in the queue behind them nor take a place in the
// batch. Their entries still count toward QueueSize, so they can keep new
// notifications in the port. It reports no progress when every request that
// is left waits for a memory copy, whose end ticks the driver again.
func (m *migrationMW) serviceBatch() bool {
	s := m.state()
	requests := s.Queue
	s.Queue = nil
	serviced := 0

	var left []migrationRequest

	for i, r := range requests {
		if serviced == m.spec().BatchSize {
			left = append(left, requests[i:]...)
			break
		}

		switch {
		case r.Counter:
			m.serviceNotification(r)
			serviced++
		case m.serviceFaults(r.Page):
			serviced++
		default:
			if !r.Deferred {
				r.Deferred = true
				s.Stats.DeferredFaults++
			}

			left = append(left, r)
		}
	}

	// A service may have queued requests; they are behind the ones that were
	// there before.
	s.Queue = append(left, s.Queue...)

	if serviced == 0 {
		return false
	}

	s.Stats.Batches++

	return true
}

// copyUses reports whether a memory copy that has started uses the page. A
// copy uses physical addresses, so a page never migrates under it.
func (m *migrationMW) copyUses(pid vm.PID, vAddr uint64) bool {
	r := m.comp.Resources.Runtime
	pageSize := m.pageSize()

	for _, e := range m.comp.State.Executing {
		if e.WaitsForMigration || r.QueuePID(e.Queue) != pid {
			continue
		}

		start, size, ok := copyRange(e.Command)
		if ok && start < vAddr+pageSize && vAddr < start+size {
			return true
		}
	}

	return false
}

// copyRange returns the device range of a memory copy.
func copyRange(cmd gpuruntime.Command) (start, size uint64, ok bool) {
	switch cmd.Kind {
	case gpuruntime.CopyH2D:
		return uint64(cmd.Dst), uint64(cmd.ByteSize), cmd.ByteSize > 0
	case gpuruntime.CopyD2H:
		return uint64(cmd.Src), uint64(cmd.ByteSize), cmd.ByteSize > 0
	default:
		return 0, 0, false
	}
}

// migrating reports whether a page that a memory copy would use is being
// serviced, in which case the copy must not start.
func (m *migrationMW) migrating(pid vm.PID, start, size uint64) bool {
	pageSize := m.pageSize()

	for _, p := range m.state().Pages {
		if p.Phase != pageIdle && p.PID == pid &&
			start < p.VAddr+pageSize && p.VAddr < start+size {
			return true
		}
	}

	return false
}

// sendTo queues a command of a page for the command processor of a GPU.
func (m *migrationMW) sendTo(pi, gpu int, payload any) {
	s := m.state()
	msg := messaging.Msg{
		ID:      m.comp.NewID(),
		Src:     m.comp.Ports.PageMigration.AsRemote(),
		Dst:     m.comp.Spec.MigrationAgents[gpu-1],
		Payload: payload,
	}

	s.Out = append(s.Out, msg)
	s.Pending[msg.ID] = pi
	s.Pages[pi].PendingAcks++
}

// sendToTargets queues one command of a page per target GPU.
func (m *migrationMW) sendToTargets(pi int, payload any) {
	for _, gpu := range m.state().Pages[pi].Targets {
		m.sendTo(pi, gpu, payload)
	}
}

func (m *migrationMW) allGPUs() []int {
	gpus := make([]int, m.numGPUs())
	for i := range gpus {
		gpus[i] = i + 1
	}

	return gpus
}
