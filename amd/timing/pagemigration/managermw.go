package pagemigration

import (
	"fmt"
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// managerMW implements the manager. It keeps the live objects that the
// hooks hand to it: the address translators to wake (by name) and the GPU
// descriptors.
type managerMW struct {
	m      *Manager
	comp   *ManagerComp
	wakers map[string]addresstranslator.Waker
	gpus   []GPU
}

func (mw *managerMW) now() timing.VTimeInPicoSec {
	return mw.comp.CurrentTime()
}

func (mw *managerMW) violationf(format string, args ...any) {
	panic(Violation(fmt.Sprintf("page migration invariant: "+format, args...)))
}

func (mw *managerMW) pageSize() uint64 {
	return 1 << mw.comp.Spec().Log2PageSize
}

func unitKey(pid vm.PID, index uint64) uint64 {
	return uint64(pid)<<44 | index
}

// unitOf returns the index of the unit that holds vAddr, or -1.
func (mw *managerMW) unitOf(pid vm.PID, vAddr uint64) int {
	index := vAddr >> mw.comp.Spec().Log2Unit

	i, ok := mw.comp.State.UnitIndex[unitKey(pid, index)]
	if !ok {
		return -1
	}

	return i
}

// getUnit returns the index of the unit that holds vAddr, creating it.
func (mw *managerMW) getUnit(pid vm.PID, vAddr uint64) int {
	if i := mw.unitOf(pid, vAddr); i >= 0 {
		return i
	}

	state := &mw.comp.State
	spec := mw.comp.Spec()
	index := vAddr >> spec.Log2Unit

	if vAddr>>56 != 0 || uint64(pid)>>20 != 0 {
		panic(fmt.Sprintf("page migration: PID %d or address %#x out of range",
			pid, vAddr))
	}

	u := unitState{PID: pid, Index: index}
	if spec.Policy == PolicyAccessCounter {
		u.Counts = make([]uint32, len(state.GPUs))
	}

	state.Units = append(state.Units, u)
	state.UnitIndex[unitKey(pid, index)] = len(state.Units) - 1

	return len(state.Units) - 1
}

func (mw *managerMW) eligible(u *unitState) bool {
	spec := mw.comp.Spec()

	if !spec.Migrates() || u.Phase != phaseIdle {
		return false
	}

	if spec.MaxMigrations > 0 && u.Migrations >= spec.MaxMigrations {
		return false
	}

	if u.EverMigrated && mw.now()-u.LastEnd < nsToPS(spec.CooldownNS) {
		return false
	}

	return true
}

// isGPU reports whether dev is a registered GPU, i.e. a device whose pages
// can migrate.
func (mw *managerMW) isGPU(dev uint64) bool {
	gpus := mw.comp.State.GPUs
	return dev < uint64(len(gpus)) && gpus[dev].Registered
}

func (mw *managerMW) ownerOf(pAddr uint64) uint64 {
	return pAddr / mw.comp.Spec().GPUMemSize
}

func (mw *managerMW) addWaiter(u *unitState, waker addresstranslator.Waker) {
	named, ok := waker.(naming.Named)
	if !ok {
		panic("page migration: an address translator must have a name")
	}

	name := named.Name()
	mw.wakers[name] = waker

	for _, w := range u.Waiters {
		if w == name {
			return
		}
	}

	u.Waiters = append(u.Waiters, name)
}

func (mw *managerMW) wake(u *unitState) {
	for _, name := range u.Waiters {
		mw.wakers[name].TickLater()
	}

	u.Waiters = nil
}

// frame returns the record of frame number ppn, creating it for a frame of
// page p that no access has touched yet.
func (mw *managerMW) frame(ppn uint64, pid vm.PID, vAddr uint64) *frameState {
	state := &mw.comp.State

	i, ok := state.Frames[ppn]
	if !ok {
		state.FrameInfo = append(state.FrameInfo, frameState{
			PID:   pid,
			VAddr: vAddr,
		})
		i = len(state.FrameInfo) - 1
		state.Frames[ppn] = i
	}

	return &state.FrameInfo[i]
}

func (mw *managerMW) frameOf(pAddr uint64) *frameState {
	state := &mw.comp.State

	i, ok := state.Frames[pAddr>>mw.comp.Spec().Log2PageSize]
	if !ok {
		return nil
	}

	return &state.FrameInfo[i]
}

func (mw *managerMW) trigger(ui int, dst uint64) {
	state := &mw.comp.State
	u := &state.Units[ui]
	u.Dst = dst
	u.Phase = phaseQueued
	u.TriggerTime = mw.now()

	h := &state.Handlers[dst]
	h.Queue = append(h.Queue, ui)

	mw.comp.TickLater()
}

// Tick receives the responses of the engines, advances the software
// handlers (start a batch, finish a batch) and the migrations in flight
// (shootdown done -> drain; drained -> copy), and sends the requests to the
// engines. A migration finishes when its last chunk is copied.
func (mw *managerMW) Tick() bool {
	madeProgress := false

	if mw.comp.Spec().Migrates() {
		madeProgress = mw.recv() || madeProgress
	}

	madeProgress = mw.tickHandlers() || madeProgress
	madeProgress = mw.tickActive() || madeProgress

	if mw.comp.Spec().Migrates() {
		madeProgress = mw.send() || madeProgress
	}

	return madeProgress
}

func (mw *managerMW) tickHandlers() bool {
	state := &mw.comp.State
	spec := mw.comp.Spec()
	now := mw.now()
	madeProgress := false

	for dev := range state.Handlers {
		h := &state.Handlers[dev]

		if h.Busy && now >= h.BusyUntil {
			h.Busy = false
			service := h.Service
			h.Service = nil

			for _, ui := range service {
				mw.startShootdown(ui)
			}

			madeProgress = true
		}

		// startShootdown does not touch the handlers, so h stays valid.
		if !h.Busy && len(h.Queue) > 0 {
			n := min(len(h.Queue), spec.BatchSize)
			batch := append([]int(nil), h.Queue[:n]...)
			h.Queue = h.Queue[n:]
			state.Stats.Batches++

			for _, ui := range batch {
				state.Units[ui].Phase = phaseService
			}

			if spec.SoftwareLatencyNS == 0 {
				for _, ui := range batch {
					mw.startShootdown(ui)
				}
			} else {
				h.Service = batch
				h.Busy = true
				h.BusyUntil = now + nsToPS(spec.SoftwareLatencyNS)
			}

			madeProgress = true
		}

		if h.Busy {
			madeProgress = true // waiting for the service time
		}
	}

	return madeProgress
}

func (mw *managerMW) tickActive() bool {
	state := &mw.comp.State
	now := mw.now()

	stillActive := state.Active[:0]

	for _, ui := range state.Active {
		u := &state.Units[ui]

		if u.Phase == phaseShootdown && now >= u.ReadyAt &&
			u.PendingInvalidations == 0 {
			u.Phase = phaseDrain
			u.DrainStart = now
		}

		if u.Phase == phaseDrain && mw.drained(u) {
			state.Stats.DrainWaitSum += uint64(now - u.DrainStart)
			mw.startCopy(ui)
		}

		if u.Phase != phaseIdle {
			stillActive = append(stillActive, ui)
		}
	}

	state.Active = stillActive

	return len(state.Active) > 0
}

// startShootdown collects the pages of the unit that are not on the
// destination yet. From now on every access to the unit is held. The TLB
// entries of the pages are invalidated in every GPU that accessed them.
func (mw *managerMW) startShootdown(ui int) {
	state := &mw.comp.State
	spec := mw.comp.Spec()
	u := &state.Units[ui]
	pageSize := mw.pageSize()

	start := u.Index << spec.Log2Unit
	end := start + (uint64(1) << spec.Log2Unit)

	u.Pages = u.Pages[:0]
	for va := start; va < end; va += pageSize {
		p, ok := mw.comp.Resources().PageTable.Find(u.PID, va)
		if !ok || p.VAddr != va || p.DeviceID == u.Dst || !mw.isGPU(p.DeviceID) {
			continue
		}

		u.Pages = append(u.Pages, p)
	}

	if len(u.Pages) == 0 {
		state.Stats.NoopMigrations++
		u.Phase = phaseIdle
		u.Faulters = nil
		mw.wake(u)

		return
	}

	now := mw.now()
	u.Phase = phaseShootdown
	u.BlockStart = now
	u.ReadyAt = now + nsToPS(spec.ShootdownLatencyNS)
	state.Stats.Shootdowns++

	for _, p := range u.Pages {
		mw.invalidate(ui, p)
	}

	state.Active = append(state.Active, ui)
}

// invalidate asks the engine of every GPU that accessed page p to drop it
// from the GPU's TLBs. The pages of the shootdowns that start in the same
// cycle are merged into one request per GPU.
func (mw *managerMW) invalidate(ui int, p vm.Page) {
	state := &mw.comp.State

	var mask uint64
	if f := mw.frameOf(p.PAddr); f != nil {
		mask = f.DevMask
	}

	for dev := range state.GPUs {
		if mask&(1<<uint(dev)) == 0 || !state.GPUs[dev].Registered {
			continue
		}

		req := mw.invalidateReqTo(state.GPUs[dev].Engine, p.PID)
		req.VAddrs = append(req.VAddrs, p.VAddr)

		units := state.InvUnits[req.ID]
		if len(units) == 0 || units[len(units)-1] != ui {
			state.InvUnits[req.ID] = append(units, ui)
			state.Units[ui].PendingInvalidations++
		}
	}
}

// invalidateReqTo returns the unsent invalidation request to engine for
// process pid, creating it.
func (mw *managerMW) invalidateReqTo(
	engine messaging.RemotePort,
	pid vm.PID,
) *InvalidateReq {
	state := &mw.comp.State

	for i := range state.OutInv {
		if state.OutInv[i].Dst == engine && state.OutInv[i].PID == pid {
			return &state.OutInv[i]
		}
	}

	state.OutInv = append(state.OutInv, InvalidateReq{
		MsgMeta: messaging.MsgMeta{
			ID:           timing.GetIDGenerator().Generate(),
			Src:          mw.comp.GetPortByName("ToEngines").AsRemote(),
			Dst:          engine,
			TrafficBytes: ctrlMsgBytes,
			TrafficClass: "pagemigration.InvalidateReq",
		},
		PID: pid,
	})
	state.Stats.InvalidateReqs++

	return &state.OutInv[len(state.OutInv)-1]
}

func (mw *managerMW) drained(u *unitState) bool {
	for _, p := range u.Pages {
		if f := mw.frameOf(p.PAddr); f != nil && f.Inflight > 0 {
			return false
		}
	}

	return true
}

// allocFrame takes the next frame of the frame region of GPU dev. Frames are
// never reused.
func (mw *managerMW) allocFrame(dev uint64) uint64 {
	state := &mw.comp.State
	spec := mw.comp.Spec()
	pageSize := mw.pageSize()

	f := state.NextFrame[dev]
	regionEnd := dev*spec.GPUMemSize + spec.FrameRegionOffset +
		spec.FrameRegionSize

	if f+pageSize > regionEnd {
		panic(fmt.Sprintf("page migration: GPU %d ran out of migration frames",
			dev))
	}

	state.NextFrame[dev] = f + pageSize

	// A new frame has never been accessed, so no cache holds a line of it,
	// and the copy may write it to the DRAM directly.
	if mw.frameOf(f) != nil {
		mw.violationf("new frame %#x was accessed before", f)
	}

	if spec.Check {
		mw.checkNoCachedLines(f, "allocation")
	}

	return f
}

// checkNoCachedLines (Spec.Check) verifies that no L2 of any GPU holds or is
// fetching a line of the frame.
func (mw *managerMW) checkNoCachedLines(frame uint64, when string) {
	spec := mw.comp.Spec()

	for a := frame; a < frame+mw.pageSize(); a += spec.LineBytes {
		for _, g := range mw.gpus {
			if g.HoldsLine != nil && g.HoldsLine(a) {
				mw.violationf("GPU %d caches line %#x of new frame %#x at %s",
					g.DeviceID, a, frame, when)
			}
		}
	}

	mw.comp.State.Stats.CheckedFrames++
}

func (mw *managerMW) startCopy(ui int) {
	state := &mw.comp.State
	spec := mw.comp.Spec()
	u := &state.Units[ui]
	pageSize := mw.pageSize()

	u.Phase = phaseCopy
	u.CopyStart = mw.now()
	u.NewPAddrs = u.NewPAddrs[:0]
	u.PendingChunks = 0

	for _, p := range u.Pages {
		mw.frame(p.PAddr>>spec.Log2PageSize, p.PID, p.VAddr).Status = frameFrozen

		newFrame := mw.allocFrame(u.Dst)
		u = &state.Units[ui]
		u.NewPAddrs = append(u.NewPAddrs, newFrame)

		for offset := uint64(0); offset < pageSize; offset += spec.ChunkBytes {
			req := CopyReq{
				MsgMeta: messaging.MsgMeta{
					ID:           timing.GetIDGenerator().Generate(),
					Src:          mw.comp.GetPortByName("ToEngines").AsRemote(),
					Dst:          state.GPUs[u.Dst].Engine,
					TrafficBytes: ctrlMsgBytes,
					TrafficClass: "pagemigration.CopyReq",
				},
				Source:   state.GPUs[p.DeviceID].Engine,
				SrcPAddr: p.PAddr + offset,
				DstPAddr: newFrame + offset,
				Size:     spec.ChunkBytes,
			}
			state.OutCopies = append(state.OutCopies, req)
			state.CopyUnits[req.ID] = ui
			u.PendingChunks++
		}
	}
}

func (mw *managerMW) chunkDone(ui int) {
	state := &mw.comp.State
	u := &state.Units[ui]

	u.PendingChunks--
	if u.PendingChunks > 0 {
		return
	}

	state.Stats.CopyTimeSum += uint64(mw.now() - u.CopyStart)
	mw.remap(ui)
}

// remap points the page table to the new frames and lets the held accesses
// go.
func (mw *managerMW) remap(ui int) {
	state := &mw.comp.State
	spec := mw.comp.Spec()
	now := mw.now()
	u := &state.Units[ui]

	var prevOwners uint64

	for i, p := range u.Pages {
		if spec.Check {
			// Nothing may have cached the new frame while it was filled.
			mw.checkNoCachedLines(u.NewPAddrs[i], "remap")
		}

		mw.frame(p.PAddr>>spec.Log2PageSize, p.PID, p.VAddr).Status = frameRetired
		prevOwners |= 1 << p.DeviceID

		p.PAddr = u.NewPAddrs[i]
		p.DeviceID = u.Dst
		mw.comp.Resources().PageTable.Update(p)

		state.Stats.PagesMigrated++
		state.Stats.BytesMigrated += mw.pageSize()
	}

	if u.Owners&(1<<u.Dst) != 0 {
		state.Stats.PingPong++
	}

	u.Owners |= prevOwners
	u.Migrations++

	if spec.MaxMigrations > 0 && u.Migrations == spec.MaxMigrations {
		state.Stats.Pinned++
	}

	u.EverMigrated = true
	u.LastEnd = now
	state.Stats.Migrations++
	state.Stats.MigrationTimeSum += uint64(now - u.TriggerTime)
	state.Stats.BlockedTimeSum += uint64(now - u.BlockStart)

	u.Phase = phaseIdle
	u.Faulters = nil
	u.Pages = u.Pages[:0]
	mw.wake(u)
	mw.comp.TickLater()
}

func (mw *managerMW) recv() bool {
	state := &mw.comp.State
	port := mw.comp.GetPortByName("ToEngines")
	madeProgress := false

	for {
		msg := port.RetrieveIncoming()
		if msg == nil {
			return madeProgress
		}

		madeProgress = true

		switch msg := msg.(type) {
		case InvalidateRsp:
			for _, ui := range state.InvUnits[msg.RspTo] {
				state.Units[ui].PendingInvalidations--
			}

			delete(state.InvUnits, msg.RspTo)
		case CopyRsp:
			ui, ok := state.CopyUnits[msg.RspTo]
			if !ok {
				panic(fmt.Sprintf("page migration: unknown copy %d", msg.RspTo))
			}

			delete(state.CopyUnits, msg.RspTo)
			mw.chunkDone(ui)
		default:
			log.Panicf("page-migration manager cannot handle %s",
				reflect.TypeOf(msg))
		}
	}
}

func (mw *managerMW) send() bool {
	state := &mw.comp.State
	port := mw.comp.GetPortByName("ToEngines")
	madeProgress := false

	for len(state.OutInv) > 0 && port.CanSend() {
		port.Send(state.OutInv[0])
		state.OutInv = state.OutInv[1:]
		madeProgress = true
	}

	for len(state.OutCopies) > 0 && port.CanSend() {
		port.Send(state.OutCopies[0])
		state.OutCopies = state.OutCopies[1:]
		madeProgress = true
	}

	return madeProgress
}
