package mmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"

	// pageTable aggregates all the methods of the page table that are used in the MMU package.
	"github.com/sarchlab/akita/v5/messaging"
)

type pageTable interface {
	Insert(page vm.Page)
	Remove(pid vm.PID, vAddr uint64)
	Find(pid vm.PID, Addr uint64) (vm.Page, bool)
	Update(page vm.Page)
	ReverseLookup(pAddr uint64) (vm.Page, bool)
	GetLog2PageSize() uint64
}

// translationMW handles translation requests: parsing from top,
// page table walks, and sending responses for local hits.
type translationMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

// Port helpers.

func (m *translationMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *translationMW) pageTable() vm.PageTable {
	return m.comp.Resources().PageTable
}

// Tick runs the translation stages. Paused MMUs make no progress;
// draining MMUs continue walks but accept no new requests.
func (m *translationMW) Tick() bool {
	if m.comp.State.ControlState == memcontrolprotocol.StatePaused {
		return false
	}

	madeProgress := false

	madeProgress = m.walkPageTable() || madeProgress
	if m.comp.State.ControlState == memcontrolprotocol.StateEnabled {
		madeProgress = m.parseFromTop() || madeProgress
	}

	return madeProgress
}

func (m *translationMW) walkPageTable() bool {
	madeProgress := false

	state := &m.comp.State

	for i := 0; i < len(state.WalkingTranslations); i++ {
		if state.WalkingTranslations[i].CycleLeft > 0 {
			state.WalkingTranslations[i].CycleLeft--
			madeProgress = true

			continue
		}

		madeProgress = m.finalizePageWalk(i) || madeProgress
	}

	tmp := state.WalkingTranslations[:0]

	for i := 0; i < len(state.WalkingTranslations); i++ {
		if !m.toRemove(i) {
			tmp = append(tmp, state.WalkingTranslations[i])
		}
	}

	state.WalkingTranslations = tmp
	state.ToRemoveFromPTW = nil

	return madeProgress
}

func (m *translationMW) finalizePageWalk(walkingIndex int) bool {
	spec := m.comp.Spec()
	state := &m.comp.State
	walking := state.WalkingTranslations[walkingIndex]

	page, found := m.pageTable().Find(
		vm.PID(walking.PID), walking.VAddr)

	if !found {
		if spec.AutoPageAllocation {
			page = m.createDefaultPage(
				vm.PID(walking.PID), walking.VAddr, walking.DeviceID)
			m.pageTable().Insert(page)
		} else {
			panic("page not found")
		}
	}

	state.WalkingTranslations[walkingIndex].Page = page

	return m.doPageWalkHit(walkingIndex)
}

func (m *translationMW) doPageWalkHit(walkingIndex int) bool {
	if !m.topPort().CanSend() {
		return false
	}

	state := &m.comp.State
	walking := state.WalkingTranslations[walkingIndex]

	rsp := vmprotocol.TranslationRsp{
		Page: walking.Page,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = walking.ReqSrc
	rsp.RspTo = walking.ReqID
	rsp.TrafficClass = "vmprotocol.TranslationRsp"

	m.topPort().Send(rsp)
	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	m.traceReqComplete(walking.RecvTaskID, walking.ReqID)

	return true
}

func (m *translationMW) parseFromTop() bool {
	spec := m.comp.Spec()
	state := &m.comp.State

	if len(state.WalkingTranslations) >= spec.MaxRequestsInFlight {
		return false
	}

	reqI := m.topPort().RetrieveIncoming()
	if reqI == nil {
		return false
	}

	switch req := reqI.(type) {
	case vmprotocol.TranslationReq:
		tracing.TraceReqReceive(m.comp, req)
		m.startWalking(req)
	default:
		log.Panicf("MMU canot handle request of type %s",
			fmt.Sprintf("%T", reqI))
	}

	return true
}

func (m *translationMW) startWalking(req vmprotocol.TranslationReq) {
	spec := m.comp.Spec()
	state := &m.comp.State

	ts := transactionState{
		ReqID:        req.ID,
		RecvTaskID:   tracing.MsgIDAtReceiver(req, m.comp),
		ReqSrc:       req.Src,
		ReqDst:       req.Dst,
		PID:          uint32(req.PID),
		VAddr:        req.VAddr,
		DeviceID:     req.DeviceID,
		TransLatency: req.TransLatency,
		CycleLeft:    spec.Latency,
	}

	state.WalkingTranslations = append(state.WalkingTranslations, ts)
}

func (m *translationMW) toRemove(index int) bool {
	state := &m.comp.State

	for i := 0; i < len(state.ToRemoveFromPTW); i++ {
		remove := state.ToRemoveFromPTW[i]
		if remove == index {
			return true
		}
	}

	return false
}

func (m *translationMW) createDefaultPage(
	pid vm.PID, vAddr uint64, deviceID uint64,
) vm.Page {
	spec := m.comp.Spec()
	alignedVAddr := (vAddr >> spec.Log2PageSize) << spec.Log2PageSize
	pageSize := uint64(1) << spec.Log2PageSize
	pAddr := m.allocatePhysicalPage()

	return vm.Page{
		PID:         pid,
		VAddr:       alignedVAddr,
		PAddr:       pAddr,
		PageSize:    pageSize,
		Valid:       true,
		DeviceID:    deviceID,
		Unified:     true,
		IsMigrating: false,
		IsPinned:    false,
	}
}

func (m *translationMW) allocatePhysicalPage() uint64 {
	spec := m.comp.Spec()
	state := &m.comp.State
	pageSize := uint64(1) << spec.Log2PageSize

	for {
		candidatePage := (state.NextPhysicalPage >> spec.Log2PageSize) << spec.Log2PageSize

		if _, found := m.pageTable().ReverseLookup(candidatePage); !found {
			state.NextPhysicalPage = candidatePage + pageSize
			return candidatePage
		}

		state.NextPhysicalPage += pageSize
	}
}

func (m *translationMW) traceReqComplete(recvTaskID, reqMsgID uint64) {
	tracing.EndTask(m.comp, tracing.TaskEnd{ID: recvTaskID})
	tracing.ForgetMsgIDAtReceiver(reqMsgID, m.comp)
}
