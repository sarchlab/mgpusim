package pagemigrationcontroller

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

type PageMigrationController struct {
	*akita.TickingComponent

	RemotePort   akita.Port
	CtrlPort     akita.Port
	LocalMemPort akita.Port

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

//nolint:gocyclo
func (e *PageMigrationController) Tick(now akita.VTimeInSec) bool {
	madeProgress := false

	madeProgress = e.sendMigrationReqToAnotherPMC(now) || madeProgress
	madeProgress = e.sendReadReqLocalMemPort(now) || madeProgress
	madeProgress = e.sendMigrationCompleteRspToCtrlPort(now) || madeProgress
	madeProgress = e.sendDataReadyRspToRequestingPMC(now) || madeProgress
	madeProgress = e.sendWriteReqLocalMemPort(now) || madeProgress
	madeProgress = e.processFromOutside(now) || madeProgress
	madeProgress = e.processFromCtrlPort(now) || madeProgress
	madeProgress = e.processFromMemCtrl(now) || madeProgress
	madeProgress = e.processPageMigrationReqFromCtrlPort(now) || madeProgress
	madeProgress = e.processReadPageReqFromAnotherPMC(now) || madeProgress
	madeProgress = e.processDataReadyRspFromMemCtrl(now) || madeProgress
	madeProgress = e.processDataPullRsp(now) || madeProgress
	madeProgress = e.processWriteDoneRspFromMemCtrl(now) || madeProgress

	return madeProgress
}

func (e *PageMigrationController) processFromOutside(
	now akita.VTimeInSec,
) bool {
	req := e.RemotePort.Peek()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *DataPullReq:
		return e.handleDataPullReq(now, req)
	case *DataPullRsp:
		return e.handleDataPullRsp(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (e *PageMigrationController) processFromCtrlPort(
	now akita.VTimeInSec,
) bool {
	//PMC handles only one page migration req at a time
	if e.isHandlingPageMigration {
		return false
	}

	req := e.CtrlPort.Retrieve(now)
	if req == nil {
		return false
	}

	e.DataTransferStartTime = now

	switch req := req.(type) {
	case *PageMigrationReqToPMC:
		return e.handleMigrationReqFromCtrlPort(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (e *PageMigrationController) handleMigrationReqFromCtrlPort(
	now akita.VTimeInSec,
	req *PageMigrationReqToPMC,
) bool {
	e.currentMigrationRequest = req
	return true
}

func (e *PageMigrationController) processPageMigrationReqFromCtrlPort(
	now akita.VTimeInSec,
) bool {
	if e.currentMigrationRequest == nil {
		return false
	}

	if e.isHandlingPageMigration {
		return false
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

	return true
}

func (e *PageMigrationController) sendMigrationReqToAnotherPMC(
	now akita.VTimeInSec,
) bool {
	if len(e.toPullFromAnotherPMC) == 0 {
		return false
	}

	madeProgress := false
	newInPullFromAnotherPMC := make([]*DataPullReq, 0)

	for i := 0; i < len(e.toPullFromAnotherPMC); i++ {
		sendPacket := e.toPullFromAnotherPMC[i]
		sendPacket.SendTime = now
		sendErr := e.RemotePort.Send(sendPacket)
		if sendErr == nil {
			madeProgress = true
		} else {
			newInPullFromAnotherPMC = append(
				newInPullFromAnotherPMC, sendPacket)
		}
	}

	e.toPullFromAnotherPMC = newInPullFromAnotherPMC
	return madeProgress
}

func (e *PageMigrationController) handleDataPullReq(
	now akita.VTimeInSec,
	req *DataPullReq,
) bool {
	e.RemotePort.Retrieve(now)
	e.currentPullReqFromAnotherPMC = append(e.currentPullReqFromAnotherPMC, req)
	e.requestingPMCtrlPort = req.Src
	return true
}

func (e *PageMigrationController) processReadPageReqFromAnotherPMC(
	now akita.VTimeInSec,
) bool {
	if e.currentPullReqFromAnotherPMC == nil {
		return false
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
	return true
}

func (e *PageMigrationController) sendReadReqLocalMemPort(
	now akita.VTimeInSec,
) bool {
	if len(e.toSendLocalMemPort) == 0 {
		return false
	}

	madeProgress := false
	newInToSendLocalMemPort := make([]*mem.ReadReq, 0)

	for i := 0; i < len(e.toSendLocalMemPort); i++ {
		sendPacket := e.toSendLocalMemPort[i]
		sendPacket.SendTime = now
		sendErr := e.LocalMemPort.Send(sendPacket)
		if sendErr == nil {
			madeProgress = true
		} else {
			newInToSendLocalMemPort = append(
				newInToSendLocalMemPort, sendPacket)
		}
	}

	e.toSendLocalMemPort = newInToSendLocalMemPort
	return madeProgress
}

func (e *PageMigrationController) processFromMemCtrl(
	now akita.VTimeInSec,
) bool {
	req := e.LocalMemPort.Retrieve(now)
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		return e.handleDataReadyRspFromMemCtrl(now, req)
	case *mem.WriteDoneRsp:
		return e.handleWriteDoneRspFromMemCtrl(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (e *PageMigrationController) handleDataReadyRspFromMemCtrl(
	now akita.VTimeInSec,
	rsp *mem.DataReadyRsp,
) bool {
	e.dataReadyRspFromMemCtrl = append(e.dataReadyRspFromMemCtrl, rsp)
	return true
}

func (e *PageMigrationController) processDataReadyRspFromMemCtrl(
	now akita.VTimeInSec,
) bool {
	if e.dataReadyRspFromMemCtrl == nil {
		return false
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
	return true
}

func (e *PageMigrationController) sendDataReadyRspToRequestingPMC(
	now akita.VTimeInSec,
) bool {
	if len(e.toRspToAnotherPMC) == 0 {
		return false
	}

	madeProgress := false
	newInToSendRspToAnotherPMC := make([]*DataPullRsp, 0)

	for i := 0; i < len(e.toRspToAnotherPMC); i++ {
		sendPacket := e.toRspToAnotherPMC[i]
		sendPacket.SendTime = now
		sendErr := e.RemotePort.Send(sendPacket)
		if sendErr == nil {
			madeProgress = true
		} else {
			newInToSendRspToAnotherPMC = append(newInToSendRspToAnotherPMC, sendPacket)
		}
	}

	e.toRspToAnotherPMC = newInToSendRspToAnotherPMC
	return madeProgress
}

func (e *PageMigrationController) handleDataPullRsp(
	now akita.VTimeInSec,
	req *DataPullRsp,
) bool {
	e.receivedDataFromAnothePMC = append(e.receivedDataFromAnothePMC, req)
	e.RemotePort.Retrieve(now)
	return true
}

func (e *PageMigrationController) processDataPullRsp(
	now akita.VTimeInSec,
) bool {
	if e.receivedDataFromAnothePMC == nil {
		return false
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
	return true
}

func (e *PageMigrationController) sendWriteReqLocalMemPort(
	now akita.VTimeInSec,
) bool {
	if e.writeReqLocalMemPort == nil {
		return false
	}

	madeProgress := false
	newInWriteReqLocalMemPort := make([]*mem.WriteReq, 0)

	for i := 0; i < len(e.writeReqLocalMemPort); i++ {
		e.writeReqLocalMemPort[i].SendTime = now
		err := e.LocalMemPort.Send(e.writeReqLocalMemPort[i])
		if err == nil {
			//log.Printf("Sending write req to mem ctrl with ID %d", e.writeReqLocalMemPort[i].ID)
			madeProgress = true
		} else {
			newInWriteReqLocalMemPort = append(newInWriteReqLocalMemPort, e.writeReqLocalMemPort[i])
		}
	}

	e.writeReqLocalMemPort = newInWriteReqLocalMemPort
	return madeProgress
}

func (e *PageMigrationController) handleWriteDoneRspFromMemCtrl(
	now akita.VTimeInSec,
	rsp *mem.WriteDoneRsp,
) bool {
	e.receivedWriteDoneFromMemCtrl = rsp
	return true
}

func (e *PageMigrationController) processWriteDoneRspFromMemCtrl(
	now akita.VTimeInSec,
) bool {
	if e.receivedWriteDoneFromMemCtrl == nil {
		return false
	}

	e.numDataRspPendingForPageMigration--
	e.receivedWriteDoneFromMemCtrl = nil

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

	return true
}

func (e *PageMigrationController) sendMigrationCompleteRspToCtrlPort(
	now akita.VTimeInSec,
) bool {
	if e.toSendToCtrlPort == nil {
		return false
	}

	e.toSendToCtrlPort.SendTime = now
	err := e.CtrlPort.Send(e.toSendToCtrlPort)

	if err == nil {
		e.DataTransferEndTime = now
		e.TotalDataTransferTime = e.TotalDataTransferTime + (e.DataTransferEndTime - e.DataTransferStartTime)
		e.isHandlingPageMigration = false
		e.currentMigrationRequest = nil
		e.toSendToCtrlPort = nil
		return true
	}

	return false
}

func (e *PageMigrationController) SetFreq(freq akita.Freq) {
	panic("not implemented")
}

func NewPageMigrationController(
	name string,
	engine akita.Engine,
	memCtrlFinder cache.LowModuleFinder,
	remoteModules cache.LowModuleFinder,
) *PageMigrationController {
	e := new(PageMigrationController)
	e.TickingComponent = akita.NewTickingComponent(name, engine, 1*akita.GHz, e)
	e.MemCtrlFinder = memCtrlFinder

	e.RemotePort = akita.NewLimitNumMsgPort(e, 1, name+".RemotePort")
	e.LocalMemPort = akita.NewLimitNumMsgPort(e, 1, name+"LocalMemPort")
	e.CtrlPort = akita.NewLimitNumMsgPort(e, 1, name+"CtrlPort")
	e.RemotePMCAddressTable = remoteModules

	e.onDemandPagingDataTransferSize = 256
	e.numDataRspPendingForPageMigration = -1

	e.reqIDToWriteAddressMap = make(map[string]uint64)

	return e
}
