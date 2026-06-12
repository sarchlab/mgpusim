package mmu

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
)

// migrationMW handles migration: sending migration requests to the driver,
// processing migration returns, and sending translation responses.
type migrationMW struct {
	comp      *modeling.Component[Spec, State]
	pageTable vm.PageTable
}

// Port helpers.

func (m *migrationMW) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *migrationMW) migrationPort() sim.Port {
	return m.comp.GetPortByName("Migration")
}

// Tick runs the migration stages.
func (m *migrationMW) Tick() bool {
	madeProgress := false

	madeProgress = m.sendMigrationToDriver() || madeProgress
	madeProgress = m.processMigrationReturn() || madeProgress

	return madeProgress
}

func (m *migrationMW) sendMigrationToDriver() (madeProgress bool) {
	state := m.comp.GetNextState()

	if len(state.MigrationQueue) == 0 {
		return false
	}

	trans := state.MigrationQueue[0]
	page, found := m.pageTable.Find(
		vm.PID(trans.PID), trans.VAddr)

	if !found {
		panic("page not found")
	}

	trans.Page = page

	if trans.DeviceID == page.DeviceID || page.IsPinned {
		if !m.topPort().CanSend() {
			return false
		}

		m.sendTranslationRsp(trans)
		state.MigrationQueue = state.MigrationQueue[1:]
		m.markPageAsNotMigratingIfNotInTheMigrationQueue(page)

		return true
	}

	if state.IsDoingMigration {
		return false
	}

	migrationReq := m.createMigrationRequest(trans, page)

	err := m.migrationPort().Send(migrationReq)
	if err != nil {
		return false
	}

	trans.Page.IsMigrating = true
	m.pageTable.Update(trans.Page)
	trans.HasMigration = true
	trans.MigrationReqID = migrationReq.ID
	trans.MigrationReqSrc = migrationReq.Src
	trans.MigrationReqDst = migrationReq.Dst
	state.IsDoingMigration = true
	state.CurrentOnDemandMigration = trans
	state.MigrationQueue = state.MigrationQueue[1:]

	return true
}

func (m *migrationMW) markPageAsNotMigratingIfNotInTheMigrationQueue(
	page vm.Page,
) vm.Page {
	state := m.comp.GetNextState()
	inQueue := false

	for _, t := range state.MigrationQueue {
		if page.PAddr == t.Page.PAddr {
			inQueue = true
			break
		}
	}

	if !inQueue {
		page.IsMigrating = false
		m.pageTable.Update(page)

		return page
	}

	return page
}

func (m *migrationMW) sendTranslationRsp(
	trans transactionState,
) (madeProgress bool) {
	rsp := &vm.TranslationRsp{
		Page: trans.Page,
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = trans.ReqSrc
	rsp.RspTo = trans.ReqID
	rsp.TrafficClass = "vm.TranslationRsp"
	m.topPort().Send(rsp)

	return true
}

func (m *migrationMW) processMigrationReturn() bool {
	item := m.migrationPort().PeekIncoming()
	if item == nil {
		return false
	}

	if !m.topPort().CanSend() {
		return false
	}

	state := m.comp.GetNextState()
	trans := state.CurrentOnDemandMigration

	page, found := m.pageTable.Find(
		vm.PID(trans.PID), trans.VAddr)

	if !found {
		panic("page not found")
	}

	rsp := &vm.TranslationRsp{
		Page: page,
	}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = trans.ReqSrc
	rsp.RspTo = trans.ReqID
	rsp.TrafficClass = "vm.TranslationRsp"
	m.topPort().Send(rsp)

	state.IsDoingMigration = false

	page = m.markPageAsNotMigratingIfNotInTheMigrationQueue(page)
	page.IsPinned = true
	m.pageTable.Update(page)

	m.migrationPort().RetrieveIncoming()

	return true
}

func (m *migrationMW) createMigrationRequest(
	trans transactionState,
	page vm.Page,
) *vm.PageMigrationReqToDriver {
	spec := m.comp.GetSpec()
	state := m.comp.GetNextState()

	migrationInfo := new(vm.PageMigrationInfo)
	migrationInfo.GPUReqToVAddrMap = make(map[uint64][]uint64)
	migrationInfo.GPUReqToVAddrMap[trans.DeviceID] =
		append(migrationInfo.GPUReqToVAddrMap[trans.DeviceID],
			trans.VAddr)

	state.PageAccessedByDeviceID = appendDeviceID(
		state.PageAccessedByDeviceID, page.VAddr, page.DeviceID)

	migrationReq := vm.NewPageMigrationReqToDriver(
		m.migrationPort().AsRemote(), spec.MigrationServiceProvider)
	migrationReq.PID = page.PID
	migrationReq.PageSize = page.PageSize
	migrationReq.CurrPageHostGPU = page.DeviceID
	migrationReq.MigrationInfo = migrationInfo
	migrationReq.CurrAccessingGPUs = unique(
		getDeviceIDs(state.PageAccessedByDeviceID, page.VAddr))
	migrationReq.RespondToTop = true

	return migrationReq
}

// Helper functions for devicePageAccess slice.

func unique(intSlice []uint64) []uint64 {
	keys := make(map[int]bool)
	list := []uint64{}

	for _, entry := range intSlice {
		if _, value := keys[int(entry)]; !value {
			keys[int(entry)] = true

			list = append(list, entry)
		}
	}

	return list
}

func getDeviceIDs(accesses []devicePageAccess, vaddr uint64) []uint64 {
	for _, a := range accesses {
		if a.PageVAddr == vaddr {
			return a.DeviceIDs
		}
	}

	return nil
}

func appendDeviceID(
	accesses []devicePageAccess, vaddr, deviceID uint64,
) []devicePageAccess {
	for i, a := range accesses {
		if a.PageVAddr == vaddr {
			accesses[i].DeviceIDs = append(a.DeviceIDs, deviceID)
			return accesses
		}
	}

	return append(accesses, devicePageAccess{
		PageVAddr: vaddr,
		DeviceIDs: []uint64{deviceID},
	})
}
