package driver

import (
	"fmt"
	"log"
	"reflect"

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

// PageMigrationSpec holds the parameters of page migration. Times are in
// nanoseconds.
type PageMigrationSpec struct {
	// Policy is PageMigrationNone, PageMigrationFault, or
	// PageMigrationCounter.
	Policy string `json:"policy"`

	// Log2Unit is the migration unit, from 12 (4 KB) to 21 (2 MB). A fault
	// or an access-counter notification migrates the unit that holds its
	// page.
	Log2Unit uint64 `json:"log2_unit"`

	// AccessCounterThreshold is the number of remote L1 misses of a GPU to a
	// unit that make its RDMA engine notify the driver
	// (uvm_perf_access_counter_threshold); 0 turns the counters off. The
	// platform sets it in the RDMA engines.
	AccessCounterThreshold uint64 `json:"access_counter_threshold"`

	// The first fault of a batch starts a timer of SoftwareLatencyNS; the
	// faults that arrive before it expires join the batch, up to BatchSize
	// (uvm_perf_fault_batch_count).
	SoftwareLatencyNS uint64 `json:"software_latency_ns"`
	BatchSize         int    `json:"batch_size"`

	// A migration of a unit within ThrashingLapseNS of its previous
	// migration is a thrashing event (uvm_perf_thrashing_lapse_usec). After
	// ThrashingThreshold consecutive events (uvm_perf_thrashing_threshold),
	// the unit is pinned for PinNS (uvm_perf_thrashing_pin).
	ThrashingLapseNS   uint64 `json:"thrashing_lapse_ns"`
	ThrashingThreshold int    `json:"thrashing_threshold"`
	PinNS              uint64 `json:"pin_ns"`
}

// DefaultPageMigrationSpec returns the default parameters, which follow the
// NVIDIA UVM driver. Page migration is off.
func DefaultPageMigrationSpec() PageMigrationSpec {
	return PageMigrationSpec{
		Policy:                 PageMigrationNone,
		Log2Unit:               12,
		AccessCounterThreshold: 256,
		SoftwareLatencyNS:      5000,
		BatchSize:              256,
		ThrashingLapseNS:       500000,
		ThrashingThreshold:     3,
		PinNS:                  150000000,
	}
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

	if s.Log2Unit < 12 || s.Log2Unit > 21 {
		return fmt.Errorf("driver: migration unit 2^%d is not in [2^12, 2^21]",
			s.Log2Unit)
	}

	if s.BatchSize <= 0 || s.ThrashingThreshold <= 0 {
		return fmt.Errorf("driver: batch size and thrashing threshold must be positive")
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

// Phases of a unit.
const (
	unitIdle      = iota // nothing to do
	unitHold             // waiting for the targets to hold the pages
	unitCopy             // the data mover of the destination copies
	unitShootdown        // the targets drop the old translations
	unitRelease          // waiting for the targets to release the pages
	unitUnpin            // a pin expired; the GPUs drop the remote translations
)

// migrationUnit is a migration unit and where it is in its service.
type migrationUnit struct {
	PID   vm.PID `json:"pid"`
	Index uint64 `json:"index"` // virtual address >> Log2Unit
	Phase int    `json:"phase"`
	Dst   int    `json:"dst"`

	// Migrate is set when the service moves pages; otherwise it only lets
	// the faulting accesses go. Targets are the GPUs that hold and release
	// VAddrs. OldPages are the pages that move and NewPAddrs their frames.
	Migrate     bool      `json:"migrate"`
	Targets     []int     `json:"targets"`
	VAddrs      []uint64  `json:"vaddrs"`
	OldPages    []vm.Page `json:"old_pages"`
	NewPAddrs   []uint64  `json:"new_paddrs"`
	PendingAcks int       `json:"pending_acks"`

	Migrations     int                   `json:"migrations"`
	LastMigration  timing.VTimeInPicoSec `json:"last_migration"`
	ThrashEvents   int                   `json:"thrash_events"`
	Pinned         bool                  `json:"pinned"`
	PinnedUntil    timing.VTimeInPicoSec `json:"pinned_until"`
	ServiceStart   timing.VTimeInPicoSec `json:"service_start"`
	CopyStart      timing.VTimeInPicoSec `json:"copy_start"`
	ShootdownStart timing.VTimeInPicoSec `json:"shootdown_start"`
}

// pageFault is a fault or, if Counter is set, an access-counter
// notification, waiting in a batch.
type pageFault struct {
	PID     vm.PID `json:"pid"`
	VAddr   uint64 `json:"vaddr"`
	GPU     int    `json:"gpu"`
	Counter bool   `json:"counter"`
}

// PageMigrationStats are the migration statistics. Times are in picoseconds.
type PageMigrationStats struct {
	Faults         uint64 `json:"faults"`
	DroppedFaults  uint64 `json:"dropped_faults"`
	IgnoredFaults  uint64 `json:"ignored_faults"`
	DeferredFaults uint64 `json:"deferred_faults"`

	// Notifications are the access-counter notifications. A notification is
	// dropped when the queue is full, and ignored when its unit does not
	// move: it is already on the notifier, migrating, pinned, or not unified.
	Notifications        uint64 `json:"notifications"`
	DroppedNotifications uint64 `json:"dropped_notifications"`
	IgnoredNotifications uint64 `json:"ignored_notifications"`

	Batches          uint64 `json:"batches"`
	Migrations       uint64 `json:"migrations"`
	PagesMigrated    uint64 `json:"pages_migrated"`
	BytesMigrated    uint64 `json:"bytes_migrated"`
	Releases         uint64 `json:"releases"`
	ThrashingEvents  uint64 `json:"thrashing_events"`
	Pins             uint64 `json:"pins"`
	Unpins           uint64 `json:"unpins"`
	MigrationTimeSum uint64 `json:"migration_time_sum"`
	CopyTimeSum      uint64 `json:"copy_time_sum"`
	ShootdownTimeSum uint64 `json:"shootdown_time_sum"`
}

// PageMigrationState is the state of the page-migration middleware.
type PageMigrationState struct {
	Units     []migrationUnit `json:"units"`
	UnitIndex map[uint64]int  `json:"unit_index"`

	// Batch is the batch being collected, due at BatchDue; NextBatch holds
	// the faults that arrive while Batch is full, and the faults that wait
	// for a memory copy. Each holds at most BatchSize faults; a notification
	// that finds both full is dropped, as a real fault buffer does on
	// overflow.
	Batch     []pageFault           `json:"batch"`
	BatchDue  timing.VTimeInPicoSec `json:"batch_due"`
	NextBatch []pageFault           `json:"next_batch"`

	// Pending maps the ID of each command in flight to its unit. Out holds
	// the commands waiting to be sent.
	Pending map[uint64]int  `json:"pending"`
	Out     []messaging.Msg `json:"out"`

	Stats PageMigrationStats `json:"stats"`
}

func newPageMigrationState() PageMigrationState {
	return PageMigrationState{
		UnitIndex: make(map[uint64]int),
		Pending:   make(map[uint64]int),
	}
}

// migrationMW is the page-migration service of the driver. It receives the
// MMU's fault notifications and the RDMA engines' access-counter
// notifications, services them in batches, and runs each
// migration with commands to the GPUs' command processors: hold the pages on
// every GPU, copy them with the destination's data mover, update the page
// table, shoot down the old translations once, and release the pages.
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

// Tick sends the commands, takes in the notifications and answers, and
// handles the batches and pins that are due.
func (m *migrationMW) Tick() bool {
	madeProgress := m.send()
	madeProgress = m.receive() || madeProgress
	madeProgress = m.serviceBatch() || madeProgress
	madeProgress = m.expirePins() || madeProgress

	return madeProgress
}

func (m *migrationMW) send() bool {
	s := m.state()
	port := m.comp.Ports.PageMigration
	madeProgress := false

	for len(s.Out) > 0 && port.CanSend() {
		port.Send(s.Out[0])
		s.Out = s.Out[1:]
		madeProgress = true
	}

	return madeProgress
}

// receive always consumes the notifications, so the MMU and the RDMA engines
// never wait for the driver. A notification that finds the queue full is
// dropped.
func (m *migrationMW) receive() bool {
	port := m.comp.Ports.PageMigration
	madeProgress := false

	for {
		msg, ok := port.RetrieveIncoming()
		if !ok {
			return madeProgress
		}

		madeProgress = true

		switch payload := msg.Payload.(type) {
		case addrtransprotocol.PageFault:
			m.addFault(pageFault{
				PID: payload.PID, VAddr: payload.VAddr, GPU: int(payload.DeviceID),
			})
		case protocol.AccessCounterNotification:
			m.addNotification(payload)
		case protocol.GeneralRsp:
			m.ack(msg.RspTo)
		default:
			log.Panicf("driver: cannot handle %s on PageMigration",
				reflect.TypeOf(msg.Payload))
		}
	}
}

func (m *migrationMW) unitKey(pid vm.PID, vAddr uint64) uint64 {
	return uint64(pid)<<44 | vAddr>>m.spec().Log2Unit
}

// addNotification maps the physical address of an access-counter
// notification back to its page and queues it. The unit is found when the
// notification is serviced.
func (m *migrationMW) addNotification(n protocol.AccessCounterNotification) {
	s := m.state()
	s.Stats.Notifications++

	frame := n.PAddr &^ (uint64(1)<<m.log2PageSize() - 1)

	page, ok := m.comp.Resources.PageTable.ReverseLookup(frame)
	if !ok {
		// The page has moved since the RDMA engine counted the access.
		s.Stats.IgnoredNotifications++
		return
	}

	m.addFault(pageFault{
		PID: page.PID, VAddr: page.VAddr, GPU: int(n.DeviceID), Counter: true,
	})
}

// addFault adds a fault or an access-counter notification to the batch being
// collected; the first one of a batch starts its timer. Those that arrive
// while the batch is full go to the next batch; when that is full too, the
// notification is dropped.
func (m *migrationMW) addFault(f pageFault) {
	s := m.state()
	if !f.Counter {
		s.Stats.Faults++
	}

	switch {
	case len(s.Batch) < m.spec().BatchSize:
		m.joinBatch(f)
	case len(s.NextBatch) < m.spec().BatchSize:
		s.NextBatch = append(s.NextBatch, f)
	case f.Counter:
		s.Stats.DroppedNotifications++
	default:
		s.Stats.DroppedFaults++
	}
}

// joinBatch adds a fault to the batch being collected.
func (m *migrationMW) joinBatch(f pageFault) {
	s := m.state()

	if len(s.Batch) == 0 {
		s.BatchDue = m.now() + nsToPS(m.spec().SoftwareLatencyNS)
		m.comp.TickAt(s.BatchDue)
	}

	s.Batch = append(s.Batch, f)
}

// serviceBatch services the batch when its timer expires. The faults that
// wait move to the next batch, whose timer starts at once.
func (m *migrationMW) serviceBatch() bool {
	s := m.state()
	if len(s.Batch) == 0 || m.now() < s.BatchDue {
		return false
	}

	batch := s.Batch
	s.Batch = nil
	s.Stats.Batches++

	var deferred []pageFault

	for _, f := range batch {
		if !m.serviceFault(f) {
			deferred = append(deferred, f)
		}
	}

	// The faults that wait were accepted already: they start the next
	// batch and are never dropped.
	waiting := append(deferred, s.NextBatch...)
	s.NextBatch = nil

	for _, f := range waiting {
		if len(s.Batch) < m.spec().BatchSize {
			m.joinBatch(f)
		} else {
			s.NextBatch = append(s.NextBatch, f)
		}
	}

	return true
}

// serviceFault decides what to do with a fault of GPU f.GPU. It returns
// false if the fault must wait for a memory copy that uses the unit.
func (m *migrationMW) serviceFault(f pageFault) bool {
	if f.Counter {
		m.serviceNotification(f)
		return true
	}

	s := m.state()
	ui := m.getUnit(f.PID, f.VAddr)
	u := &s.Units[ui]

	// A unit that migrates now releases every GPU when it is done; the
	// faulting access is translated again then.
	if u.Phase != unitIdle {
		s.Stats.IgnoredFaults++
		return true
	}

	if m.copyUses(u.PID, u.Index) {
		s.Stats.DeferredFaults++
		return false
	}

	all := m.unitPages(u)
	moving := m.movingPages(all, f.GPU)

	if !u.Pinned && len(moving) > 0 && m.thrashing(u) {
		m.pin(u, all)
	}

	u.Dst = f.GPU
	u.ServiceStart = m.now()
	u.Targets = u.Targets[:0]
	u.VAddrs = u.VAddrs[:0]
	u.OldPages = u.OldPages[:0]

	if u.Pinned || len(moving) == 0 {
		// Nothing moves: the faulting GPU releases the unit. On the replay,
		// the walk finds the page local or pinned and does not fault.
		u.Migrate = false
		u.Targets = append(u.Targets, f.GPU)

		for _, p := range all {
			u.VAddrs = append(u.VAddrs, p.VAddr)
		}

		m.startRelease(ui)

		return true
	}

	m.startMigration(ui, moving)

	return true
}

// serviceNotification migrates the unit of an access-counter notification
// to the notifier, unless the unit is already there, migrating, or pinned.
// No access waits for a notification, so nothing is released when the unit
// does not move.
func (m *migrationMW) serviceNotification(f pageFault) {
	s := m.state()
	ui := m.getUnit(f.PID, f.VAddr)
	u := &s.Units[ui]

	if u.Phase != unitIdle || u.Pinned || m.copyUses(u.PID, u.Index) {
		s.Stats.IgnoredNotifications++
		return
	}

	all := m.unitPages(u)
	moving := m.movingPages(all, f.GPU)

	if len(moving) == 0 {
		s.Stats.IgnoredNotifications++
		return
	}

	if m.thrashing(u) {
		m.pin(u, all)
		return
	}

	u.Dst = f.GPU
	u.ServiceStart = m.now()
	u.Targets = u.Targets[:0]
	u.VAddrs = u.VAddrs[:0]
	u.OldPages = u.OldPages[:0]

	m.startMigration(ui, moving)
}

// startMigration starts moving pages of a unit to u.Dst: every GPU holds
// them first.
func (m *migrationMW) startMigration(ui int, moving []vm.Page) {
	u := &m.state().Units[ui]
	u.Migrate = true
	u.OldPages = append(u.OldPages, moving...)

	for _, p := range moving {
		u.VAddrs = append(u.VAddrs, p.VAddr)
	}

	for gpu := 1; gpu <= len(m.comp.Spec.MigrationAgents); gpu++ {
		u.Targets = append(u.Targets, gpu)
	}

	m.startHold(ui)
}

// movingPages returns the pages that a migration to GPU dst moves: the
// unified, unpinned pages that are on another GPU.
func (m *migrationMW) movingPages(pages []vm.Page, dst int) []vm.Page {
	var moving []vm.Page

	for _, p := range pages {
		if p.Unified && !p.IsPinned && int(p.DeviceID) != dst &&
			m.isGPU(int(p.DeviceID)) {
			moving = append(moving, p)
		}
	}

	return moving
}

// thrashing records a thrashing event if the unit migrated within the lapse,
// and reports whether the unit must be pinned.
func (m *migrationMW) thrashing(u *migrationUnit) bool {
	s := m.state()

	if u.Migrations == 0 ||
		m.now()-u.LastMigration >= nsToPS(m.spec().ThrashingLapseNS) {
		u.ThrashEvents = 0
		return false
	}

	u.ThrashEvents++
	s.Stats.ThrashingEvents++

	return u.ThrashEvents >= m.spec().ThrashingThreshold
}

// pin pins a unit: its pages are mapped remotely, so the other GPUs access
// them remotely and its walks no longer fault, until the pin expires.
func (m *migrationMW) pin(u *migrationUnit, pages []vm.Page) {
	s := m.state()

	for _, p := range pages {
		p.IsPinned = true
		m.comp.Resources.PageTable.Update(p)
	}

	u.Pinned = true
	u.PinnedUntil = m.now() + nsToPS(m.spec().PinNS)
	s.Stats.Pins++
	m.comp.TickAt(u.PinnedUntil)
}

// expirePins unpins the idle units whose pin has expired and shoots down
// their translations on every GPU, so that the next access faults again.
func (m *migrationMW) expirePins() bool {
	s := m.state()
	now := m.now()
	madeProgress := false

	for ui := range s.Units {
		u := &s.Units[ui]
		if !u.Pinned || now < u.PinnedUntil || u.Phase != unitIdle {
			continue
		}

		u.Pinned = false
		u.ThrashEvents = 0
		s.Stats.Unpins++

		u.VAddrs = u.VAddrs[:0]
		for _, p := range m.unitPages(u) {
			p.IsPinned = false
			m.comp.Resources.PageTable.Update(p)
			u.VAddrs = append(u.VAddrs, p.VAddr)
		}

		u.Targets = u.Targets[:0]
		for gpu := 1; gpu <= len(m.comp.Spec.MigrationAgents); gpu++ {
			u.Targets = append(u.Targets, gpu)
		}

		u.Phase = unitUnpin
		m.sendToTargets(ui, func() any {
			return protocol.PageShootdownReq{PID: u.PID, VAddrs: u.VAddrs}
		})

		madeProgress = true
	}

	return madeProgress
}

func (m *migrationMW) isGPU(dev int) bool {
	return dev >= 1 && dev <= len(m.comp.Spec.MigrationAgents)
}

// getUnit returns the index of the unit that holds vAddr, creating it.
func (m *migrationMW) getUnit(pid vm.PID, vAddr uint64) int {
	s := m.state()
	key := m.unitKey(pid, vAddr)

	if i, ok := s.UnitIndex[key]; ok {
		return i
	}

	s.Units = append(s.Units, migrationUnit{
		PID: pid, Index: vAddr >> m.spec().Log2Unit,
	})
	s.UnitIndex[key] = len(s.Units) - 1

	return len(s.Units) - 1
}

// unitPages returns the pages of a unit that are in the page table.
func (m *migrationMW) unitPages(u *migrationUnit) []vm.Page {
	var pages []vm.Page

	pageSize := uint64(1) << m.log2PageSize()
	start := u.Index << m.spec().Log2Unit
	end := start + (uint64(1) << m.spec().Log2Unit)

	for va := start; va < end; va += pageSize {
		if p, ok := m.comp.Resources.PageTable.Find(u.PID, va); ok {
			pages = append(pages, p)
		}
	}

	return pages
}

// copyUses reports whether a memory copy that has started uses the unit. A
// copy uses physical addresses, so a unit never migrates under it.
func (m *migrationMW) copyUses(pid vm.PID, index uint64) bool {
	r := m.comp.Resources.Runtime
	log2Unit := m.spec().Log2Unit

	for _, e := range m.comp.State.Executing {
		if e.WaitsForMigration || r.QueuePID(e.Queue) != pid {
			continue
		}

		start, size, ok := copyRange(e.Command)
		if ok && start>>log2Unit <= index && (start+size-1)>>log2Unit >= index {
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

// migrating reports whether a unit that a memory copy would use is being
// serviced, in which case the copy must not start.
func (m *migrationMW) migrating(pid vm.PID, start, size uint64) bool {
	s := m.state()
	log2Unit := m.spec().Log2Unit

	for _, u := range s.Units {
		if u.Phase != unitIdle && u.PID == pid &&
			start>>log2Unit <= u.Index && (start+size-1)>>log2Unit >= u.Index {
			return true
		}
	}

	return false
}

// sendToTargets queues one command per target GPU of a unit.
func (m *migrationMW) sendToTargets(ui int, payload func() any) {
	u := &m.state().Units[ui]

	for _, gpu := range u.Targets {
		m.sendTo(ui, gpu, payload())
	}
}

func (m *migrationMW) sendTo(ui, gpu int, payload any) {
	s := m.state()
	msg := messaging.Msg{
		ID:      m.comp.NewID(),
		Src:     m.comp.Ports.PageMigration.AsRemote(),
		Dst:     m.comp.Spec.MigrationAgents[gpu-1],
		Payload: payload,
	}

	s.Out = append(s.Out, msg)
	s.Pending[msg.ID] = ui
	s.Units[ui].PendingAcks++
}

func (m *migrationMW) startHold(ui int) {
	u := &m.state().Units[ui]
	u.Phase = unitHold

	m.sendToTargets(ui, func() any {
		return protocol.PageHoldReq{PID: u.PID, VAddrs: u.VAddrs}
	})
}

// ack takes in the answer of a command processor and advances its unit once
// every command of the phase is answered.
func (m *migrationMW) ack(reqID uint64) {
	s := m.state()

	ui, ok := s.Pending[reqID]
	if !ok {
		panic(fmt.Sprintf("driver: unknown page-migration answer to %d", reqID))
	}

	delete(s.Pending, reqID)

	u := &s.Units[ui]
	u.PendingAcks--

	if u.PendingAcks > 0 {
		return
	}

	now := m.now()

	switch u.Phase {
	case unitHold:
		if u.Migrate {
			m.startCopy(ui)
		} else {
			m.startRelease(ui)
		}
	case unitCopy:
		s.Stats.CopyTimeSum += uint64(now - u.CopyStart)
		m.remap(ui)
		u.Phase = unitShootdown
		u.ShootdownStart = now
		m.sendToTargets(ui, func() any {
			return protocol.PageShootdownReq{PID: u.PID, VAddrs: u.VAddrs}
		})
	case unitShootdown:
		s.Stats.ShootdownTimeSum += uint64(now - u.ShootdownStart)
		m.startRelease(ui)
	case unitRelease:
		m.finish(ui)
	case unitUnpin:
		u.Phase = unitIdle
	default:
		panic(fmt.Sprintf("driver: page-migration answer in phase %d", u.Phase))
	}
}

// startCopy allocates the new frames on the destination and asks its
// command processor to copy each page.
func (m *migrationMW) startCopy(ui int) {
	u := &m.state().Units[ui]
	u.Phase = unitCopy
	u.CopyStart = m.now()
	u.NewPAddrs = u.NewPAddrs[:0]

	for _, p := range u.OldPages {
		frame := m.comp.Resources.Runtime.AllocateFrame(u.Dst)
		u.NewPAddrs = append(u.NewPAddrs, frame)
		m.sendTo(ui, u.Dst, protocol.PageCopyReq{
			SrcAddress: p.PAddr,
			DstAddress: frame,
			ByteSize:   p.PageSize,
		})
	}
}

// remap points the page table to the new frames. The old frames are
// retired. No GPU translates the pages while they are held, so no TLB can be
// filled with the old mapping after this point.
func (m *migrationMW) remap(ui int) {
	s := m.state()
	u := &s.Units[ui]

	for i, p := range u.OldPages {
		p.PAddr = u.NewPAddrs[i]
		p.DeviceID = uint64(u.Dst)
		m.comp.Resources.PageTable.Update(p)

		s.Stats.PagesMigrated++
		s.Stats.BytesMigrated += p.PageSize
	}
}

func (m *migrationMW) startRelease(ui int) {
	u := &m.state().Units[ui]
	u.Phase = unitRelease

	m.sendToTargets(ui, func() any {
		return protocol.PageReleaseReq{PID: u.PID, VAddrs: u.VAddrs}
	})
}

func (m *migrationMW) finish(ui int) {
	s := m.state()
	u := &s.Units[ui]

	if u.Migrate {
		u.Migrations++
		u.LastMigration = u.ServiceStart
		s.Stats.Migrations++
		s.Stats.MigrationTimeSum += uint64(m.now() - u.ServiceStart)
	} else {
		s.Stats.Releases++
	}

	u.Phase = unitIdle
	u.Migrate = false
	u.OldPages = u.OldPages[:0]
	u.NewPAddrs = u.NewPAddrs[:0]
}
