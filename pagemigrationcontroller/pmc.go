package pagemigrationcontroller

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

type PageMigrationController struct {
	*akita.ComponentBase
	ticker *akita.Ticker

	RemotePort   akita.Port
	CtrlPort     akita.Port
	LocalMemPort akita.Port

	engine                akita.Engine
	RemotePMCAddressTable cache.LowModuleFinder

	currentMigrationRequest      *PageMigrationReqToPMC
	currentPullReqFromAnotherPMC []*DataPullReq

	toPullFromAnotherPMC         []*DataPullReq
	toSendLocalMemPort           []*mem.ReadReq
	dataReadyRspFromMemCtrl      []*mem.DataReadyRsp
	toRspToAnotherPMC            []*DataPullRsp
	receivedDataFromAnothePMC    []*DataPullRsp
	writeReqLocalMemPort         []*mem.WriteReq
	receivedWriteDoneFromMemCtrl *mem.WriteDoneRsp
	toSendToCtrlPort             *PageMigrationRspFromPMC

	freq     akita.Freq
	needTick bool

	onDemandPagingDataTransferSize uint64

	requestingPMCtrlPort akita.Port

	numDataRspPendingForPageMigration int
	reqIDToWriteAddressMap            map[string]uint64

	MemCtrlFinder cache.LowModuleFinder

	DataTransferStartTime akita.VTimeInSec
	DataTransferEndTime   akita.VTimeInSec
	TotalDataTransferTime akita.VTimeInSec

	isHandlingPageMigration bool
}

func (e *PageMigrationController) Handle(evt akita.Event) error {
	switch evt := evt.(type) {
	case akita.TickEvent:
		e.tick(evt.Time())
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (e *PageMigrationController) tick(now akita.VTimeInSec) {
	e.needTick = false

	e.sendMigrationReqToAnotherPMC(now)
	e.sendReadReqLocalMemPort(now)
	e.sendMigrationCompleteRspToCtrlPort(now)
	e.sendDataReadyRspToRequestingPMC(now)
	e.sendWriteReqLocalMemPort(now)
	e.processFromOutside(now)
	e.processFromCtrlPort(now)
	e.processFromMemCtrl(now)
	e.processPageMigrationReqFromCtrlPort(now)
	e.processReadPageReqFromAnotherPMC(now)
	e.processDataReadyRspFromMemCtrl(now)
	e.processDataPullRsp(now)
	e.processWriteDoneRspFromMemCtrl(now)

	if e.needTick {
		e.ticker.TickLater(now)
	}
}

func (e *PageMigrationController) processFromOutside(now akita.VTimeInSec) {
	req := e.RemotePort.Peek()
	if req == nil {
		return
	}

	switch req := req.(type) {
	case *DataPullReq:
		e.handleDataPullReq(now, req)
	case *DataPullRsp:
		e.handleDataPullRsp(now, req)

	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
}

func (e *PageMigrationController) processFromCtrlPort(now akita.VTimeInSec) {
	//PMC handles only one page migration req at a time
	if e.isHandlingPageMigration {
		return
	}

	req := e.CtrlPort.Retrieve(now)
	if req == nil {
		return
	}

	e.DataTransferStartTime = now

	switch req := req.(type) {
	case *PageMigrationReqToPMC:
		e.handleMigrationReqFromCtrlPort(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
}

func (e *PageMigrationController) handleMigrationReqFromCtrlPort(now akita.VTimeInSec, req *PageMigrationReqToPMC) {
	e.currentMigrationRequest = req
	e.needTick = true
}

func (e *PageMigrationController) processPageMigrationReqFromCtrlPort(
	now akita.VTimeInSec,
) {
	if e.currentMigrationRequest == nil {
		return
	}

	if e.isHandlingPageMigration {
		return
	}

	destination := e.currentMigrationRequest.PMCPortOfRemoteGPU
	pageSize := e.currentMigrationRequest.PageSize

	//Break down each request into the data transfer size supported by PMC
	numDataTransfersForPage := pageSize / e.onDemandPagingDataTransferSize
	startingPhysicalAddress := e.currentMigrationRequest.ToReadFromPhysicalAddress

	e.numDataRspPendingForPageMigration = int(numDataTransfersForPage)
	currentWriteAddress := e.currentMigrationRequest.ToWriteToPhysicalAddress

	for i := 0; i < int(numDataTransfersForPage); i++ {
		req := DataPullReqBuilder{}.
			WithSendTime(now).
			WithSrc(e.RemotePort).
			WithDst(destination).
			WithDataTransferSize(e.onDemandPagingDataTransferSize).
			WithReadFromPhyAddress(startingPhysicalAddress).
			Build()
		startingPhysicalAddress = startingPhysicalAddress + e.onDemandPagingDataTransferSize
		e.toPullFromAnotherPMC = append(e.toPullFromAnotherPMC, req)
		e.reqIDToWriteAddressMap[req.ID] = currentWriteAddress
		currentWriteAddress = currentWriteAddress + e.onDemandPagingDataTransferSize
	}

	e.isHandlingPageMigration = true

	e.needTick = true
}

func (e *PageMigrationController) sendMigrationReqToAnotherPMC(
	now akita.VTimeInSec,
) {
	if len(e.toPullFromAnotherPMC) == 0 {
		return
	}

	newInPullFromAnotherPMC := make([]*DataPullReq, 0)

	for i := 0; i < len(e.toPullFromAnotherPMC); i++ {
		sendPacket := e.toPullFromAnotherPMC[i]
		sendPacket.SendTime = now
		sendErr := e.RemotePort.Send(sendPacket)
		if sendErr == nil {
			e.needTick = true
		} else {
			newInPullFromAnotherPMC = append(newInPullFromAnotherPMC, sendPacket)
		}
	}

	e.toPullFromAnotherPMC = newInPullFromAnotherPMC
}

func (e *PageMigrationController) handleDataPullReq(
	now akita.VTimeInSec,
	req *DataPullReq,
) {
	e.RemotePort.Retrieve(now)
	e.currentPullReqFromAnotherPMC = append(e.currentPullReqFromAnotherPMC, req)
	e.requestingPMCtrlPort = req.Src
	e.needTick = true
}

func (e *PageMigrationController) processReadPageReqFromAnotherPMC(now akita.VTimeInSec) {
	if e.currentPullReqFromAnotherPMC == nil {
		return
	}

	for i := 0; i < len(e.currentPullReqFromAnotherPMC); i++ {
		address := e.currentPullReqFromAnotherPMC[i].ToReadFromPhyAddress
		dataTransferSize := e.currentPullReqFromAnotherPMC[i].DataTransferSize
		req := mem.ReadReqBuilder{}.
			WithSendTime(now).
			WithSrc(e.LocalMemPort).
			WithDst(e.MemCtrlFinder.Find(address)).
			WithAddress(address).
			WithByteSize(dataTransferSize).
			Build()

		req.ID = e.currentPullReqFromAnotherPMC[i].ID
		e.toSendLocalMemPort = append(e.toSendLocalMemPort, req)
	}

	e.currentPullReqFromAnotherPMC = nil
	e.needTick = true
}

func (e *PageMigrationController) sendReadReqLocalMemPort(
	now akita.VTimeInSec,
) {
	if len(e.toSendLocalMemPort) == 0 {
		return
	}

	newInToSendLocalMemPort := make([]*mem.ReadReq, 0)

	for i := 0; i < len(e.toSendLocalMemPort); i++ {
		sendPacket := e.toSendLocalMemPort[i]
		sendPacket.SendTime = now
		sendErr := e.LocalMemPort.Send(sendPacket)
		if sendErr == nil {
			e.needTick = true
		} else {
			newInToSendLocalMemPort = append(newInToSendLocalMemPort, sendPacket)
		}
	}

	e.toSendLocalMemPort = newInToSendLocalMemPort
}

func (e *PageMigrationController) processFromMemCtrl(now akita.VTimeInSec) {
	req := e.LocalMemPort.Retrieve(now)
	if req == nil {
		return
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		e.handleDataReadyRspFromMemCtrl(now, req)
	case *mem.WriteDoneRsp:
		e.handleWriteDoneRspFromMemCtrl(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
}

func (e *PageMigrationController) handleDataReadyRspFromMemCtrl(
	now akita.VTimeInSec,
	rsp *mem.DataReadyRsp,
) {
	e.needTick = true
	e.dataReadyRspFromMemCtrl = append(e.dataReadyRspFromMemCtrl, rsp)
}

func (e *PageMigrationController) processDataReadyRspFromMemCtrl(
	now akita.VTimeInSec,
) {
	if e.dataReadyRspFromMemCtrl == nil {
		return
	}

	for i := 0; i < len(e.dataReadyRspFromMemCtrl); i++ {
		data := e.dataReadyRspFromMemCtrl[i].Data
		rsp := DataPullRspBuilder{}.
			WithSendTime(now).
			WithSrc(e.RemotePort).
			WithDst(e.requestingPMCtrlPort).
			WithData(data).
			Build()
		rsp.ID = e.dataReadyRspFromMemCtrl[i].RespondTo

		e.toRspToAnotherPMC = append(e.toRspToAnotherPMC, rsp)
	}

	e.dataReadyRspFromMemCtrl = nil
	e.needTick = true
}

func (e *PageMigrationController) sendDataReadyRspToRequestingPMC(
	now akita.VTimeInSec,
) {
	if len(e.toRspToAnotherPMC) == 0 {
		return
	}

	newInToSendRspToAnotherPMC := make([]*DataPullRsp, 0)

	for i := 0; i < len(e.toRspToAnotherPMC); i++ {
		sendPacket := e.toRspToAnotherPMC[i]
		sendPacket.SendTime = now
		sendErr := e.RemotePort.Send(sendPacket)
		if sendErr == nil {
			e.needTick = true
		} else {
			newInToSendRspToAnotherPMC = append(newInToSendRspToAnotherPMC, sendPacket)
		}
	}

	e.toRspToAnotherPMC = newInToSendRspToAnotherPMC
}

func (e *PageMigrationController) handleDataPullRsp(now akita.VTimeInSec, req *DataPullRsp) {
	e.needTick = true
	e.receivedDataFromAnothePMC = append(e.receivedDataFromAnothePMC, req)
	e.RemotePort.Retrieve(now)
}

func (e *PageMigrationController) processDataPullRsp(now akita.VTimeInSec) {
	if e.receivedDataFromAnothePMC == nil {
		return
	}

	for i := 0; i < len(e.receivedDataFromAnothePMC); i++ {
		data := e.receivedDataFromAnothePMC[i].Data
		address, found := e.reqIDToWriteAddressMap[e.receivedDataFromAnothePMC[i].ID]
		if !found {
			log.Panicf("We do not know where the mem controller should write")
		}
		req := mem.WriteReqBuilder{}.
			WithSendTime(now).
			WithSrc(e.LocalMemPort).
			WithDst(e.MemCtrlFinder.Find(address)).
			WithData(data).
			WithAddress(address).
			Build()

		e.writeReqLocalMemPort = append(e.writeReqLocalMemPort, req)
		delete(e.reqIDToWriteAddressMap, e.receivedDataFromAnothePMC[i].ID)
	}

	e.receivedDataFromAnothePMC = nil
	e.needTick = true
}

func (e *PageMigrationController) sendWriteReqLocalMemPort(now akita.VTimeInSec) {
	if e.writeReqLocalMemPort == nil {
		return
	}

	newInWriteReqLocalMemPort := make([]*mem.WriteReq, 0)

	for i := 0; i < len(e.writeReqLocalMemPort); i++ {
		e.writeReqLocalMemPort[i].SendTime = now
		err := e.LocalMemPort.Send(e.writeReqLocalMemPort[i])
		if err == nil {
			//log.Printf("Sending write req to mem ctrl with ID %d", e.writeReqLocalMemPort[i].ID)
			e.needTick = true
		} else {
			newInWriteReqLocalMemPort = append(newInWriteReqLocalMemPort, e.writeReqLocalMemPort[i])
		}
	}

	e.writeReqLocalMemPort = newInWriteReqLocalMemPort
}

func (e *PageMigrationController) handleWriteDoneRspFromMemCtrl(now akita.VTimeInSec, rsp *mem.WriteDoneRsp) {
	e.needTick = true
	e.receivedWriteDoneFromMemCtrl = rsp
}

func (e *PageMigrationController) processWriteDoneRspFromMemCtrl(now akita.VTimeInSec) {
	if e.receivedWriteDoneFromMemCtrl == nil {
		return
	}

	e.numDataRspPendingForPageMigration--
	e.receivedWriteDoneFromMemCtrl = nil
	e.needTick = true

	if e.numDataRspPendingForPageMigration < 0 {
		log.Panicf("Not possible")
	}
	if e.numDataRspPendingForPageMigration == 0 {
		//log.Printf("Sending migration complete rsp to CtrlPort \n")
		rsp := PageMigrationRspFromPMCBuilder{}.
			WithSendTime(now).
			WithSrc(e.CtrlPort).
			WithDst(e.currentMigrationRequest.Src).
			Build()

		e.toSendToCtrlPort = rsp
		e.currentMigrationRequest = nil
		e.numDataRspPendingForPageMigration = -1
	}
}

func (e *PageMigrationController) sendMigrationCompleteRspToCtrlPort(now akita.VTimeInSec) {
	if e.toSendToCtrlPort == nil {
		return
	}

	e.toSendToCtrlPort.SendTime = now
	err := e.CtrlPort.Send(e.toSendToCtrlPort)

	if err == nil {
		e.DataTransferEndTime = now
		e.TotalDataTransferTime = e.TotalDataTransferTime + (e.DataTransferEndTime - e.DataTransferStartTime)
		e.needTick = true
		e.isHandlingPageMigration = false
		e.currentMigrationRequest = nil
		e.toSendToCtrlPort = nil
		return
	}
}

func (e *PageMigrationController) NotifyRecv(now akita.VTimeInSec, port akita.Port) {
	e.ticker.TickLater(now)
}

func (e *PageMigrationController) NotifyPortFree(now akita.VTimeInSec, port akita.Port) {
	e.ticker.TickLater(now)
}

func (e *PageMigrationController) SetFreq(freq akita.Freq) {
	e.freq = freq
}

func NewPageMigrationController(
	name string,
	engine akita.Engine,
	memCtrlFinder cache.LowModuleFinder,
	remoteModules cache.LowModuleFinder,
) *PageMigrationController {
	e := new(PageMigrationController)
	e.freq = 1 * akita.GHz
	e.ComponentBase = akita.NewComponentBase(name)
	e.ticker = akita.NewTicker(e, engine, e.freq)

	e.engine = engine
	e.MemCtrlFinder = memCtrlFinder

	e.RemotePort = akita.NewLimitNumMsgPort(e, 1)
	e.LocalMemPort = akita.NewLimitNumMsgPort(e, 1)
	e.CtrlPort = akita.NewLimitNumMsgPort(e, 1)
	e.RemotePMCAddressTable = remoteModules

	e.onDemandPagingDataTransferSize = 256
	e.numDataRspPendingForPageMigration = -1

	e.reqIDToWriteAddressMap = make(map[string]uint64)

	return e
}
