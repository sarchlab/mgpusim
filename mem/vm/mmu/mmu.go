package mmu

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/vm"
)

type transaction struct {
	req       *vm.TranslationReq
	page      vm.Page
	cycleLeft int
	migration *vm.PageMigrationReqToDriver
}

// MMU is the default mmu implementation. It is also an akita Component.
type MMU struct {
	sim.TickingComponent

	topPort       sim.Port
	migrationPort sim.Port

	MigrationServiceProvider sim.Port

	topSender sim.BufferedSender

	pageTable           vm.PageTable
	latency             int
	maxRequestsInFlight int

	walkingTranslations      []transaction
	migrationQueue           []transaction
	migrationQueueSize       int
	currentOnDemandMigration transaction
	isDoingMigration         bool

	toRemoveFromPTW        []int
	PageAccessedByDeviceID map[uint64][]uint64
}

// Tick defines how the MMU update state each cycle
func (mmu *MMU) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = mmu.topSender.Tick(now) || madeProgress
	madeProgress = mmu.sendMigrationToDriver(now) || madeProgress
	madeProgress = mmu.walkPageTable(now) || madeProgress
	madeProgress = mmu.processMigrationReturn(now) || madeProgress
	madeProgress = mmu.parseFromTop(now) || madeProgress

	return madeProgress
}

func (mmu *MMU) trace(now sim.VTimeInSec, what string) {
	ctx := sim.HookCtx{
		Domain: mmu,
		Item:   what,
	}

	mmu.InvokeHook(ctx)
}

func (mmu *MMU) walkPageTable(now sim.VTimeInSec) bool {
	madeProgress := false
	for i := 0; i < len(mmu.walkingTranslations); i++ {
		if mmu.walkingTranslations[i].cycleLeft > 0 {
			mmu.walkingTranslations[i].cycleLeft--
			madeProgress = true
			continue
		}

		madeProgress = mmu.finalizePageWalk(now, i) || madeProgress
	}

	tmp := mmu.walkingTranslations[:0]
	for i := 0; i < len(mmu.walkingTranslations); i++ {
		if !mmu.toRemove(i) {
			tmp = append(tmp, mmu.walkingTranslations[i])
		}
	}
	mmu.walkingTranslations = tmp
	mmu.toRemoveFromPTW = nil

	return madeProgress
}

func (mmu *MMU) finalizePageWalk(
	now sim.VTimeInSec,
	walkingIndex int,
) bool {
	req := mmu.walkingTranslations[walkingIndex].req
	page, found := mmu.pageTable.Find(req.PID, req.VAddr)

	if !found {
		panic("page not found")
	}

	mmu.walkingTranslations[walkingIndex].page = page

	if page.IsMigrating {
		return mmu.addTransactionToMigrationQueue(walkingIndex)
	}

	if mmu.pageNeedMigrate(mmu.walkingTranslations[walkingIndex]) {
		return mmu.addTransactionToMigrationQueue(walkingIndex)
	}

	return mmu.doPageWalkHit(now, walkingIndex)
}

func (mmu *MMU) addTransactionToMigrationQueue(walkingIndex int) bool {
	if len(mmu.migrationQueue) >= mmu.migrationQueueSize {
		return false
	}

	mmu.toRemoveFromPTW = append(mmu.toRemoveFromPTW, walkingIndex)
	mmu.migrationQueue = append(mmu.migrationQueue,
		mmu.walkingTranslations[walkingIndex])

	page := mmu.walkingTranslations[walkingIndex].page
	page.IsMigrating = true
	mmu.pageTable.Update(page)

	return true
}

func (mmu *MMU) pageNeedMigrate(walking transaction) bool {
	if walking.req.DeviceID == walking.page.DeviceID {
		return false
	}

	if !walking.page.Unified {
		return false
	}

	if walking.page.IsPinned {
		return false
	}

	return true
}

func (mmu *MMU) doPageWalkHit(
	now sim.VTimeInSec,
	walkingIndex int,
) bool {
	if !mmu.topSender.CanSend(1) {
		return false
	}
	walking := mmu.walkingTranslations[walkingIndex]

	rsp := vm.TranslationRspBuilder{}.
		WithSendTime(now).
		WithSrc(mmu.topPort).
		WithDst(walking.req.Src).
		WithRspTo(walking.req.ID).
		WithPage(walking.page).
		Build()

	mmu.topSender.Send(rsp)
	mmu.toRemoveFromPTW = append(mmu.toRemoveFromPTW, walkingIndex)

	tracing.TraceReqComplete(walking.req, mmu)

	return true
}

func (mmu *MMU) sendMigrationToDriver(
	now sim.VTimeInSec,
) (madeProgress bool) {
	if len(mmu.migrationQueue) == 0 {
		return false
	}

	trans := mmu.migrationQueue[0]
	req := trans.req
	page, found := mmu.pageTable.Find(req.PID, req.VAddr)
	if !found {
		panic("page not found")
	}
	trans.page = page

	if req.DeviceID == page.DeviceID || page.IsPinned {
		mmu.sendTranlationRsp(now, trans)
		mmu.migrationQueue = mmu.migrationQueue[1:]
		mmu.markPageAsNotMigratingIfNotInTheMigrationQueue(page)

		return true
	}

	if mmu.isDoingMigration {
		return false
	}

	migrationInfo := new(vm.PageMigrationInfo)
	migrationInfo.GPUReqToVAddrMap = make(map[uint64][]uint64)
	migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID] =
		append(migrationInfo.GPUReqToVAddrMap[trans.req.DeviceID],
			trans.req.VAddr)

	mmu.PageAccessedByDeviceID[page.VAddr] =
		append(mmu.PageAccessedByDeviceID[page.VAddr], page.DeviceID)

	migrationReq := vm.NewPageMigrationReqToDriver(
		now, mmu.migrationPort, mmu.MigrationServiceProvider)
	migrationReq.PID = page.PID
	migrationReq.PageSize = page.PageSize
	migrationReq.CurrPageHostGPU = page.DeviceID
	migrationReq.MigrationInfo = migrationInfo
	migrationReq.CurrAccessingGPUs = unique(mmu.PageAccessedByDeviceID[page.VAddr])
	migrationReq.RespondToTop = true

	err := mmu.migrationPort.Send(migrationReq)
	if err != nil {
		return false
	}

	trans.page.IsMigrating = true
	mmu.pageTable.Update(trans.page)
	trans.migration = migrationReq
	mmu.isDoingMigration = true
	mmu.currentOnDemandMigration = trans
	mmu.migrationQueue = mmu.migrationQueue[1:]

	return true
}

func (mmu *MMU) markPageAsNotMigratingIfNotInTheMigrationQueue(
	page vm.Page,
) vm.Page {
	inQueue := false
	for _, t := range mmu.migrationQueue {
		if page.PAddr == t.page.PAddr {
			inQueue = true
			break
		}
	}

	if !inQueue {
		page.IsMigrating = false
		mmu.pageTable.Update(page)
		return page
	}

	return page
}

func (mmu *MMU) sendTranlationRsp(
	now sim.VTimeInSec,
	trans transaction,
) (madeProgress bool) {
	req := trans.req
	page := trans.page

	rsp := vm.TranslationRspBuilder{}.
		WithSendTime(now).
		WithSrc(mmu.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()
	mmu.topSender.Send(rsp)

	return true
}

func (mmu *MMU) processMigrationReturn(now sim.VTimeInSec) bool {
	item := mmu.migrationPort.Peek()
	if item == nil {
		return false
	}

	if !mmu.topSender.CanSend(1) {
		return false
	}

	req := mmu.currentOnDemandMigration.req
	page, found := mmu.pageTable.Find(req.PID, req.VAddr)
	if !found {
		panic("page not found")
	}

	rsp := vm.TranslationRspBuilder{}.
		WithSendTime(now).
		WithSrc(mmu.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()
	mmu.topSender.Send(rsp)

	mmu.isDoingMigration = false

	page = mmu.markPageAsNotMigratingIfNotInTheMigrationQueue(page)
	page.IsPinned = true
	mmu.pageTable.Update(page)

	mmu.migrationPort.Retrieve(now)

	return true
}

func (mmu *MMU) parseFromTop(now sim.VTimeInSec) bool {
	if len(mmu.walkingTranslations) >= mmu.maxRequestsInFlight {
		return false
	}

	req := mmu.topPort.Retrieve(now)
	if req == nil {
		return false
	}

	tracing.TraceReqReceive(req, mmu)

	switch req := req.(type) {
	case *vm.TranslationReq:
		mmu.startWalking(req)
	default:
		log.Panicf("MMU canot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (mmu *MMU) startWalking(req *vm.TranslationReq) {
	translationInPipeline := transaction{
		req:       req,
		cycleLeft: mmu.latency,
	}

	mmu.walkingTranslations = append(mmu.walkingTranslations, translationInPipeline)
}

func (mmu *MMU) toRemove(index int) bool {
	for i := 0; i < len(mmu.toRemoveFromPTW); i++ {
		remove := mmu.toRemoveFromPTW[i]
		if remove == index {
			return true
		}
	}
	return false
}

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
