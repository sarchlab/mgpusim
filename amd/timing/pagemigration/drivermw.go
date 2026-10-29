package pagemigration

import (
	"fmt"
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// DriverPort is the port of the driver that page migration uses. It
// receives the faults of the MMU, the notifications of the access counters,
// and the answers of the command processors.
const DriverPort = "PageMigration"

// DriverMiddleware is the page-migration service of the driver. It decides
// what to migrate and runs each migration by sending requests to the
// command processors of the GPUs: hold the pages everywhere, copy them with
// the data mover of the destination, update the page table, shoot down the
// old translations, and release the pages. All of its state is in
// DriverState.
type DriverMiddleware struct {
	host      Host
	state     *DriverState
	spec      Spec
	pageTable vm.PageTable
}

// NewDriverMiddleware returns the middleware of the driver that keeps its
// state in state. The page table is the one the driver and the MMU share.
func NewDriverMiddleware(
	host Host,
	state *DriverState,
	spec Spec,
	pageTable vm.PageTable,
) *DriverMiddleware {
	if err := spec.Validate(); err != nil {
		panic(err)
	}

	return &DriverMiddleware{
		host:      host,
		state:     state,
		spec:      spec,
		pageTable: pageTable,
	}
}

// Spec returns the configuration.
func (m *DriverMiddleware) Spec() Spec { return m.spec }

// State returns the state.
func (m *DriverMiddleware) State() *DriverState { return m.state }

// RegisterGPU adds GPU dev, whose command processor answers on port cp.
func (m *DriverMiddleware) RegisterGPU(dev uint64, cp messaging.RemotePort) {
	s := m.state

	if dev == 0 || dev >= 64 {
		panic(fmt.Sprintf("page migration supports GPU IDs 1 to 63, got %d", dev))
	}

	for uint64(len(s.GPUs)) <= dev {
		s.GPUs = append(s.GPUs, gpuState{})
		s.Handlers = append(s.Handlers, handlerState{})
		s.NextFrame = append(s.NextFrame, 0)
	}

	s.GPUs[dev] = gpuState{Registered: true, CP: cp}
	s.NextFrame[dev] = dev*m.spec.GPUMemSize + m.spec.FrameRegionOffset
}

func (m *DriverMiddleware) now() timing.VTimeInPicoSec {
	return m.host.CurrentTime()
}

func (m *DriverMiddleware) pageSize() uint64 {
	return 1 << m.spec.Log2PageSize
}

func (m *DriverMiddleware) isGPU(dev uint64) bool {
	return dev < uint64(len(m.state.GPUs)) && m.state.GPUs[dev].Registered
}

// Tick receives the faults, notifications, and answers, advances the
// software handlers and the migrations, and sends the requests.
func (m *DriverMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.recv() || madeProgress
	madeProgress = m.tickHandlers() || madeProgress
	madeProgress = m.tickShootdowns() || madeProgress
	madeProgress = m.tickRemoteMaps() || madeProgress
	madeProgress = m.send() || madeProgress

	return madeProgress
}

func (m *DriverMiddleware) recv() bool {
	port := m.host.GetPortByName(DriverPort)
	madeProgress := false

	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			return madeProgress
		}

		madeProgress = true

		switch msg := msg.(type) {
		case vmprotocol.PageFault:
			m.state.Stats.Faults++
			m.fault(m.getUnit(msg.PID, msg.VAddr), msg.DeviceID)
		case rdma.AccessCounterNotification:
			m.notification(msg)
		case Rsp:
			m.ack(msg.RspTo)
		default:
			log.Panicf("page-migration driver middleware cannot handle %s",
				reflect.TypeOf(msg))
		}
	}
}

func unitKey(pid vm.PID, index uint64) uint64 {
	return uint64(pid)<<44 | index
}

// getUnit returns the index of the unit that holds vAddr, creating it.
func (m *DriverMiddleware) getUnit(pid vm.PID, vAddr uint64) int {
	s := m.state
	index := vAddr >> m.spec.Log2Unit

	if vAddr>>56 != 0 || uint64(pid)>>20 != 0 {
		panic(fmt.Sprintf("page migration: PID %d or address %#x out of range",
			pid, vAddr))
	}

	if i, ok := s.UnitIndex[unitKey(pid, index)]; ok {
		return i
	}

	s.Units = append(s.Units, unitState{PID: pid, Index: index})
	s.UnitIndex[unitKey(pid, index)] = len(s.Units) - 1

	return len(s.Units) - 1
}

func (m *DriverMiddleware) eligible(u *unitState) bool {
	if !m.spec.Migrates() || u.Pinned {
		return false
	}

	if m.spec.MaxMigrations > 0 && u.Migrations >= m.spec.MaxMigrations {
		return false
	}

	if u.EverMigrated && m.now()-u.LastEnd < nsToPS(m.spec.CooldownNS) {
		return false
	}

	return true
}

// fault handles a fault of GPU dev on unit ui. A fault on an idle unit
// starts a service with dev as the destination; a fault from the
// destination of a service that has not started joins it; any other fault
// waits until the unit is idle again.
func (m *DriverMiddleware) fault(ui int, dev uint64) {
	u := &m.state.Units[ui]

	switch {
	case u.Phase == phaseIdle:
		u.Faulters = append(u.Faulters[:0], dev)
		m.trigger(ui, dev)
	case (u.Phase == phaseQueued || u.Phase == phaseService) && u.Dst == dev:
		u.Faulters = appendUnique(u.Faulters, dev)
	default:
		u.LateFaults = appendUnique(u.LateFaults, dev)
	}
}

func appendUnique(list []uint64, v uint64) []uint64 {
	for _, x := range list {
		if x == v {
			return list
		}
	}

	return append(list, v)
}

// notification handles a notification of an access counter: the unit of
// the page migrates to the GPU that accessed it, if it may.
func (m *DriverMiddleware) notification(n rdma.AccessCounterNotification) {
	s := m.state
	s.Stats.CounterNotifications++

	if m.spec.Policy != PolicyAccessCounter {
		return
	}

	page, ok := m.pageOfFrame(n.PAddr)
	if !ok || page.DeviceID == n.DeviceID || !m.isGPU(page.DeviceID) {
		return
	}

	ui := m.getUnit(page.PID, page.VAddr)
	u := &s.Units[ui]

	if u.Phase != phaseIdle || !m.eligible(u) {
		return
	}

	s.Stats.CounterTriggers++
	u.Faulters = u.Faulters[:0]
	m.trigger(ui, n.DeviceID)
}

// pageOfFrame returns the page mapped to the frame at pAddr. Frames are
// cached; a cached frame is checked against the page table, since the
// driver may have reused it.
func (m *DriverMiddleware) pageOfFrame(pAddr uint64) (vm.Page, bool) {
	s := m.state
	frame := pAddr >> m.spec.Log2PageSize
	frameAddr := frame << m.spec.Log2PageSize

	if ref, ok := s.Frames[frame]; ok {
		if p, found := m.pageTable.Find(ref.PID, ref.VAddr); found &&
			p.PAddr == frameAddr {
			return p, true
		}
	}

	p, found := m.pageTable.ReverseLookup(frameAddr)
	if !found {
		return vm.Page{}, false
	}

	s.Frames[frame] = pageRef{PID: p.PID, VAddr: p.VAddr}

	return p, true
}

func (m *DriverMiddleware) trigger(ui int, dst uint64) {
	s := m.state
	u := &s.Units[ui]
	u.Dst = dst
	u.Phase = phaseQueued
	u.TriggerTime = m.now()

	s.Handlers[dst].Queue = append(s.Handlers[dst].Queue, ui)
}

// tickHandlers finishes the batches whose service time is over and starts
// the next batches.
func (m *DriverMiddleware) tickHandlers() bool {
	s := m.state
	now := m.now()
	madeProgress := false

	for dev := range s.Handlers {
		h := &s.Handlers[dev]

		if h.Busy && now >= h.BusyUntil {
			h.Busy = false
			service := h.Service
			h.Service = nil

			for _, ui := range service {
				m.startService(ui)
			}

			madeProgress = true
		}

		// startService does not touch the handlers, so h stays valid.
		if !h.Busy && len(h.Queue) > 0 {
			n := min(len(h.Queue), m.spec.BatchSize)
			batch := append([]int(nil), h.Queue[:n]...)
			h.Queue = h.Queue[n:]
			s.Stats.Batches++

			for _, ui := range batch {
				s.Units[ui].Phase = phaseService
			}

			if m.spec.SoftwareLatencyNS == 0 {
				for _, ui := range batch {
					m.startService(ui)
				}
			} else {
				h.Service = batch
				h.Busy = true
				h.BusyUntil = now + nsToPS(m.spec.SoftwareLatencyNS)
			}

			madeProgress = true
		}

		if h.Busy {
			madeProgress = true // waiting for the service time
		}
	}

	return madeProgress
}

// unitPages returns the pages of the unit that are in the page table.
func (m *DriverMiddleware) unitPages(u *unitState) []vm.Page {
	var pages []vm.Page

	start := u.Index << m.spec.Log2Unit
	end := start + (uint64(1) << m.spec.Log2Unit)

	for va := start; va < end; va += m.pageSize() {
		if p, ok := m.pageTable.Find(u.PID, va); ok && p.VAddr == va {
			pages = append(pages, p)
		}
	}

	return pages
}

// startService starts the service of a unit at the end of its batch. If the
// unit may migrate and has pages on other GPUs than the destination, every
// GPU holds them. Otherwise, the service maps the unit remotely when it may
// not migrate, and lets the faulted accesses go: the faulting GPUs hold the
// unit (so that their faulted translations have returned) and release it.
func (m *DriverMiddleware) startService(ui int) {
	s := m.state
	u := &s.Units[ui]
	all := m.unitPages(u)
	s.Stats.Services++

	u.Pages, u.VAddrs, u.Targets = u.Pages[:0], u.VAddrs[:0], u.Targets[:0]

	for _, p := range all {
		if p.DeviceID != u.Dst && m.isGPU(p.DeviceID) {
			u.Pages = append(u.Pages, p)
		}
	}

	u.Migrate = m.eligible(u) && len(u.Pages) > 0

	if u.Migrate {
		for _, p := range u.Pages {
			u.VAddrs = append(u.VAddrs, p.VAddr)
		}

		for dev := range s.GPUs {
			if s.GPUs[dev].Registered {
				u.Targets = append(u.Targets, uint64(dev))
			}
		}
	} else {
		u.Pages = u.Pages[:0]
		m.mapRemotely(ui, all)

		for _, p := range all {
			u.VAddrs = append(u.VAddrs, p.VAddr)
		}

		u.Targets = append(u.Targets, u.Faulters...)
	}

	if len(u.Targets) == 0 || len(u.VAddrs) == 0 {
		m.finish(ui)
		return
	}

	u.Phase = phaseHold
	u.HoldStart = m.now()

	for _, dev := range u.Targets {
		m.state.OutHolds = append(m.state.OutHolds, HoldReq{
			MsgMeta: m.reqMeta(ui, dev, "pagemigration.HoldReq"),
			PID:     u.PID,
			VAddrs:  u.VAddrs,
		})
		s.Stats.HoldReqs++
	}
}

// mapRemotely pins the pages of a unit that a service does not move, so
// that their walks do not fault again: every page while the unit may not
// migrate (until the end of its cooldown, or for good once pinned), and the
// pages on a device that is not a GPU.
func (m *DriverMiddleware) mapRemotely(ui int, pages []vm.Page) {
	s := m.state
	u := &s.Units[ui]
	eligible := m.eligible(u)

	if eligible {
		s.Stats.NoopServices++
	} else {
		s.Stats.RemoteMaps++
	}

	for _, p := range pages {
		if p.IsPinned || p.DeviceID == u.Dst ||
			(eligible && m.isGPU(p.DeviceID)) {
			continue
		}

		p.IsPinned = true
		m.pageTable.Update(p)
	}

	if !eligible && !u.Pinned && m.spec.Migrates() {
		s.RemoteMaps = append(s.RemoteMaps, remoteMapState{
			Unit:  ui,
			Until: u.LastEnd + nsToPS(m.spec.CooldownNS),
		})
	}
}

// tickRemoteMaps unpins the units whose cooldown has ended, so that their
// walks fault again.
func (m *DriverMiddleware) tickRemoteMaps() bool {
	s := m.state
	if len(s.RemoteMaps) == 0 {
		return false
	}

	now := m.now()
	left := s.RemoteMaps[:0]

	for _, r := range s.RemoteMaps {
		if now < r.Until {
			left = append(left, r)
			continue
		}

		u := &s.Units[r.Unit]
		if u.Pinned {
			continue
		}

		for _, p := range m.unitPages(u) {
			if p.IsPinned {
				p.IsPinned = false
				m.pageTable.Update(p)
			}
		}
	}

	s.RemoteMaps = left

	return true // waiting for the end of a cooldown
}

func (m *DriverMiddleware) reqMeta(
	ui int,
	dev uint64,
	class string,
) messaging.MsgMeta {
	meta := messaging.MsgMeta{
		ID:           timing.GetIDGenerator().Generate(),
		Src:          m.host.GetPortByName(DriverPort).AsRemote(),
		Dst:          m.state.GPUs[dev].CP,
		TrafficBytes: ctrlMsgBytes,
		TrafficClass: class,
	}

	m.state.Pending[meta.ID] = ui
	m.state.Units[ui].PendingAcks++

	return meta
}

// ack takes in the answer of a command processor and advances the unit
// once every request of its phase is answered.
func (m *DriverMiddleware) ack(reqID uint64) {
	s := m.state

	ui, ok := s.Pending[reqID]
	if !ok {
		panic(fmt.Sprintf("page migration: unknown answer to %d", reqID))
	}

	delete(s.Pending, reqID)

	u := &s.Units[ui]
	u.PendingAcks--

	if u.PendingAcks > 0 {
		return
	}

	switch u.Phase {
	case phaseHold:
		s.Stats.HoldTimeSum += uint64(m.now() - u.HoldStart)

		if u.Migrate {
			m.startCopy(ui)
		} else {
			m.startRelease(ui)
		}
	case phaseCopy:
		s.Stats.CopyTimeSum += uint64(m.now() - u.CopyStart)
		m.remap(ui)
		m.startShootdown(ui)
	case phaseShootdown:
		// tickShootdowns waits for the shootdown latency.
	case phaseRelease:
		m.finish(ui)
	default:
		panic(fmt.Sprintf("page migration: answer in phase %d", u.Phase))
	}
}

// startCopy allocates the new frames on the destination and asks its
// command processor to copy each page.
func (m *DriverMiddleware) startCopy(ui int) {
	u := &m.state.Units[ui]
	u.Phase = phaseCopy
	u.CopyStart = m.now()
	u.NewPAddrs = u.NewPAddrs[:0]

	for _, p := range u.Pages {
		newFrame := m.allocFrame(u.Dst)
		u = &m.state.Units[ui]
		u.NewPAddrs = append(u.NewPAddrs, newFrame)

		m.state.OutCopies = append(m.state.OutCopies, CopyReq{
			MsgMeta:  m.reqMeta(ui, u.Dst, "pagemigration.CopyReq"),
			SrcPAddr: p.PAddr,
			DstPAddr: newFrame,
			Size:     m.pageSize(),
		})
	}
}

// allocFrame takes the next frame of the frame region of GPU dev. Frames are
// never reused, so no cache can hold a line of a new frame.
func (m *DriverMiddleware) allocFrame(dev uint64) uint64 {
	s := m.state
	f := s.NextFrame[dev]
	regionEnd := dev*m.spec.GPUMemSize + m.spec.FrameRegionOffset +
		m.spec.FrameRegionSize

	if f+m.pageSize() > regionEnd {
		panic(fmt.Sprintf("page migration: GPU %d ran out of migration frames",
			dev))
	}

	s.NextFrame[dev] = f + m.pageSize()

	return f
}

// remap points the page table to the new frames. No GPU can translate the
// pages while they are held, so no TLB is filled with the old mapping
// after this point; the shootdown that follows drops the mappings that the
// TLBs cached before the hold.
func (m *DriverMiddleware) remap(ui int) {
	s := m.state
	u := &s.Units[ui]

	u.Migrations++
	if m.spec.MaxMigrations > 0 && u.Migrations >= m.spec.MaxMigrations {
		u.Pinned = true
		s.Stats.Pinned++
	}

	var prevOwners uint64

	for i, p := range u.Pages {
		prevOwners |= 1 << p.DeviceID
		delete(s.Frames, p.PAddr>>m.spec.Log2PageSize)

		p.PAddr = u.NewPAddrs[i]
		p.DeviceID = u.Dst
		p.IsPinned = u.Pinned
		m.pageTable.Update(p)

		s.Frames[p.PAddr>>m.spec.Log2PageSize] = pageRef{PID: p.PID, VAddr: p.VAddr}
		s.Stats.PagesMigrated++
		s.Stats.BytesMigrated += m.pageSize()
	}

	if u.Owners&(1<<u.Dst) != 0 {
		s.Stats.PingPong++
	}

	u.Owners |= prevOwners
	u.EverMigrated = true
	m.dropRemoteMap(ui)
}

func (m *DriverMiddleware) dropRemoteMap(ui int) {
	left := m.state.RemoteMaps[:0]

	for _, r := range m.state.RemoteMaps {
		if r.Unit != ui {
			left = append(left, r)
		}
	}

	m.state.RemoteMaps = left
}

func (m *DriverMiddleware) startShootdown(ui int) {
	u := &m.state.Units[ui]
	u.Phase = phaseShootdown
	u.ShootdownStart = m.now()
	m.state.Stats.Shootdowns++

	for _, dev := range u.Targets {
		m.state.OutShootdowns = append(m.state.OutShootdowns, ShootdownReq{
			MsgMeta: m.reqMeta(ui, dev, "pagemigration.ShootdownReq"),
			PID:     u.PID,
			VAddrs:  u.VAddrs,
		})
	}
}

// tickShootdowns releases the units whose shootdown every GPU has
// acknowledged, once the shootdown latency has passed.
func (m *DriverMiddleware) tickShootdowns() bool {
	s := m.state
	now := m.now()
	madeProgress := false

	for ui := range s.Units {
		u := &s.Units[ui]
		if u.Phase != phaseShootdown || u.PendingAcks > 0 {
			continue
		}

		madeProgress = true

		if now-u.ShootdownStart < nsToPS(m.spec.ShootdownLatencyNS) {
			continue // waiting for the shootdown latency
		}

		s.Stats.ShootdownTimeSum += uint64(now - u.ShootdownStart)
		m.startRelease(ui)
	}

	return madeProgress
}

func (m *DriverMiddleware) startRelease(ui int) {
	u := &m.state.Units[ui]
	u.Phase = phaseRelease

	for _, dev := range u.Targets {
		m.state.OutReleases = append(m.state.OutReleases, ReleaseReq{
			MsgMeta: m.reqMeta(ui, dev, "pagemigration.ReleaseReq"),
			PID:     u.PID,
			VAddrs:  u.VAddrs,
		})
	}
}

// finish ends the service of a unit and handles the faults that arrived
// during it.
func (m *DriverMiddleware) finish(ui int) {
	s := m.state
	u := &s.Units[ui]
	now := m.now()

	if u.Migrate {
		u.LastEnd = now
		s.Stats.Migrations++
		s.Stats.MigrationTimeSum += uint64(now - u.TriggerTime)
		s.Stats.BlockedTimeSum += uint64(now - u.HoldStart)
	}

	u.Phase = phaseIdle
	u.Migrate = false
	u.Faulters = u.Faulters[:0]
	u.Pages = u.Pages[:0]

	late := u.LateFaults
	u.LateFaults = nil

	for _, dev := range late {
		m.fault(ui, dev)
	}
}

func (m *DriverMiddleware) send() bool {
	s := m.state
	port := m.host.GetPortByName(DriverPort)
	madeProgress := false

	for len(s.OutHolds) > 0 && port.CanSend() {
		port.Send(s.OutHolds[0])
		s.OutHolds = s.OutHolds[1:]
		madeProgress = true
	}

	for len(s.OutCopies) > 0 && port.CanSend() {
		port.Send(s.OutCopies[0])
		s.OutCopies = s.OutCopies[1:]
		madeProgress = true
	}

	for len(s.OutShootdowns) > 0 && port.CanSend() {
		port.Send(s.OutShootdowns[0])
		s.OutShootdowns = s.OutShootdowns[1:]
		madeProgress = true
	}

	for len(s.OutReleases) > 0 && port.CanSend() {
		port.Send(s.OutReleases[0])
		s.OutReleases = s.OutReleases[1:]
		madeProgress = true
	}

	return madeProgress
}
