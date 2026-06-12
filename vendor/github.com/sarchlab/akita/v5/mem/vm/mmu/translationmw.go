package mmu

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// pageTable aggregates all the methods of the page table that are used in the MMU package.
type pageTable interface {
	Insert(page vm.Page)
	Remove(pid vm.PID, vAddr uint64)
	Find(pid vm.PID, Addr uint64) (vm.Page, bool)
	Update(page vm.Page)
	ReverseLookup(pAddr uint64) (vm.Page, bool)
	GetLog2PageSize() uint64
}

// PageTable returns the page table from an MMU component.
func PageTable(c *modeling.Component[Spec, State]) vm.PageTable {
	return c.Middlewares()[0].(*translationMW).pageTable
}

// translationMW handles translation requests: parsing from top,
// page table walks, and sending responses for local hits.
type translationMW struct {
	comp      *modeling.Component[Spec, State]
	pageTable vm.PageTable
}

// Port helpers.

func (m *translationMW) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

// Tick runs the translation stages.
func (m *translationMW) Tick() bool {
	madeProgress := false

	madeProgress = m.walkPageTable() || madeProgress
	madeProgress = m.parseFromTop() || madeProgress

	return madeProgress
}

func (m *translationMW) walkPageTable() bool {
	madeProgress := false

	state := m.comp.GetNextState()

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
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()
	walking := state.WalkingTranslations[walkingIndex]

	page, found := m.pageTable.Find(
		vm.PID(walking.PID), walking.VAddr)

	if !found {
		if spec.AutoPageAllocation {
			page = m.createDefaultPage(
				vm.PID(walking.PID), walking.VAddr, walking.DeviceID)
			m.pageTable.Insert(page)
		} else {
			panic(fmt.Sprintf("page not found: PID=%d, VAddr=0x%x, DeviceID=%d",
				walking.PID, walking.VAddr, walking.DeviceID))
		}
	}

	state.WalkingTranslations[walkingIndex].Page = page

	if page.IsMigrating {
		return m.addTransactionToMigrationQueue(walkingIndex)
	}

	if m.pageNeedMigrate(state.WalkingTranslations[walkingIndex]) {
		return m.addTransactionToMigrationQueue(walkingIndex)
	}

	return m.doPageWalkHit(walkingIndex)
}

func (m *translationMW) addTransactionToMigrationQueue(
	walkingIndex int,
) bool {
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()

	if len(state.MigrationQueue) >= spec.MigrationQueueSize {
		return false
	}

	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)
	state.MigrationQueue = append(state.MigrationQueue,
		state.WalkingTranslations[walkingIndex])

	page := state.WalkingTranslations[walkingIndex].Page
	page.IsMigrating = true
	m.pageTable.Update(page)

	return true
}

func (m *translationMW) pageNeedMigrate(
	walking transactionState,
) bool {
	if walking.DeviceID == walking.Page.DeviceID {
		return false
	}

	if !walking.Page.Unified {
		return false
	}

	if walking.Page.IsPinned {
		return false
	}

	return true
}

func (m *translationMW) doPageWalkHit(walkingIndex int) bool {
	if !m.topPort().CanSend() {
		return false
	}

	state := m.comp.GetNextState()
	walking := state.WalkingTranslations[walkingIndex]

	rsp := &vm.TranslationRsp{
		Page: walking.Page,
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = walking.ReqSrc
	rsp.RspTo = walking.ReqID
	rsp.TrafficClass = "vm.TranslationRsp"

	m.topPort().Send(rsp)
	state.ToRemoveFromPTW = append(state.ToRemoveFromPTW, walkingIndex)

	m.traceReqComplete(walking.RecvTaskID)

	return true
}

func (m *translationMW) parseFromTop() bool {
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()

	if len(state.WalkingTranslations) >= spec.MaxRequestsInFlight {
		return false
	}

	reqI := m.topPort().RetrieveIncoming()
	if reqI == nil {
		return false
	}

	switch req := reqI.(type) {
	case *vm.TranslationReq:
		tracing.TraceReqReceive(req, m.comp)
		m.startWalking(req)
	default:
		log.Panicf("MMU canot handle request of type %s",
			fmt.Sprintf("%T", reqI))
	}

	return true
}

func (m *translationMW) startWalking(req *vm.TranslationReq) {
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()

	ts := transactionState{
		ReqID:        req.ID,
		RecvTaskID:   req.RecvTaskID,
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
	state := m.comp.GetNextState()

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
	spec := m.comp.GetSpec()
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
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()
	pageSize := uint64(1) << spec.Log2PageSize

	for {
		candidatePage := (state.NextPhysicalPage >> spec.Log2PageSize) << spec.Log2PageSize

		if _, found := m.pageTable.ReverseLookup(candidatePage); !found {
			state.NextPhysicalPage = candidatePage + pageSize
			return candidatePage
		}

		state.NextPhysicalPage += pageSize
	}
}

func (m *translationMW) traceReqComplete(recvTaskID uint64) {
	tracing.EndTask(recvTaskID, m.comp)
}
