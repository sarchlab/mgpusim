package pagemigration

import (
	"fmt"

	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/messaging"
)

// atHook is the migration hook of one address translator of GPU dev.
type atHook struct {
	mw       *managerMW
	dev      uint64
	canFault bool
}

// Resolve holds the access while its unit migrates, corrects a stale
// translation after a unit migrated, and raises a fault (on-fault policy).
func (h *atHook) Resolve(
	waker addresstranslator.Waker,
	pid vm.PID,
	vAddr uint64,
	page vm.Page,
) (vm.Page, bool) {
	mw := h.mw
	state := &mw.comp.State
	spec := mw.comp.Spec()

	ui := mw.unitOf(pid, vAddr)
	if ui >= 0 {
		u := &state.Units[ui]

		if u.blocks(h.dev) {
			mw.addWaiter(u, waker)
			return page, false
		}

		if u.EverMigrated {
			// A TLB may still hold the mapping from before the remap (a
			// walk in flight during the shootdown refills it); the page
			// table is the authority.
			if p, ok := mw.comp.Resources().PageTable.Find(pid, vAddr); ok {
				page = p
			}
		}
	}

	if !h.canFault || spec.Policy != PolicyOnFault || page.DeviceID == h.dev ||
		!mw.isGPU(page.DeviceID) {
		return page, true
	}

	if ui < 0 {
		ui = mw.getUnit(pid, vAddr)
	}

	u := &state.Units[ui]
	if !mw.eligible(u) {
		return page, true
	}

	state.Stats.Faults++
	u.Faulters = append(u.Faulters[:0], h.dev)
	mw.addWaiter(u, waker)
	mw.trigger(ui, h.dev)

	return page, false
}

// Issued counts the access in flight to its frame and records that the GPU
// accessed the frame (its TLBs may hold the translation).
func (h *atHook) Issued(pid vm.PID, vAddr, pAddr uint64) {
	mw := h.mw
	log2PageSize := mw.comp.Spec().Log2PageSize
	f := mw.frame(pAddr>>log2PageSize, pid, vAddr>>log2PageSize<<log2PageSize)

	if f.Status != frameLive {
		mw.violationf("GPU %d issued an access to frame %#x (va %#x) "+
			"in status %d", h.dev, pAddr, vAddr, f.Status)
	}

	f.Inflight++
	f.DevMask |= 1 << h.dev
}

// Completed ends the access in flight to its frame.
func (h *atHook) Completed(pAddr uint64) {
	f := h.mw.frameOf(pAddr)
	if f == nil || f.Inflight <= 0 {
		panic(fmt.Sprintf("page migration: completion without issue at %#x",
			pAddr))
	}

	f.Inflight--
}

// rdmaHook observes the requests that an RDMA engine accepts from its L1
// side, i.e. the accesses of a GPU that leave its L1 for remote memory.
type rdmaHook struct {
	mw  *managerMW
	dev uint64
}

// RemoteAccessHook returns the hook to attach to the "RDMARequestInside"
// port of the RDMA engine of GPU dev. It counts the requests that the engine
// retrieves from the port, i.e. the L1 misses to remote memory, for the
// access counters and the statistics.
func (m *Manager) RemoteAccessHook(dev uint64) hooking.Hook {
	return &rdmaHook{mw: m.mw, dev: dev}
}

// Func implements hooking.Hook.
func (h *rdmaHook) Func(ctx hooking.HookCtx) {
	if ctx.Pos != messaging.HookPosPortMsgRetrieveIncoming {
		return
	}

	req, ok := ctx.Item.(memprotocol.AccessReq)
	if !ok {
		return
	}

	h.mw.remoteAccess(h.dev, req)
}

func (mw *managerMW) remoteAccess(dev uint64, req memprotocol.AccessReq) {
	state := &mw.comp.State
	spec := mw.comp.Spec()
	pAddr := req.GetAddress()

	state.Stats.RemoteAccesses++

	if _, ok := req.(memprotocol.ReadReq); ok {
		state.Stats.RemoteReads++

		owner := mw.ownerOf(pAddr)
		if owner < uint64(len(state.Stats.RemoteReadsByOwner)) {
			state.Stats.RemoteReadsByOwner[owner]++
		}
	}

	f := mw.frameOf(pAddr)
	if f == nil {
		return // not translated by an address translator
	}

	if f.Status == frameRetired {
		mw.violationf("GPU %d sent a remote access to retired frame %#x",
			dev, pAddr)
	}

	if spec.Policy != PolicyAccessCounter {
		return
	}

	ui := mw.getUnit(f.PID, f.VAddr)
	u := &state.Units[ui]

	if u.Phase != phaseIdle {
		return
	}

	u.Counts[dev]++
	if u.Counts[dev] < spec.Threshold || !mw.eligible(u) {
		return
	}

	best := dev
	for d, c := range u.Counts {
		if c > u.Counts[best] {
			best = uint64(d)
		}
	}

	for i := range u.Counts {
		u.Counts[i] = 0
	}

	state.Stats.CounterTriggers++
	u.Faulters = u.Faulters[:0]
	mw.trigger(ui, best)
}
