package pagemigrationcontroller

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// PageMigrationController control page migration
type PageMigrationController struct {
	*sim.TickingComponent

	remotePort   sim.Port
	ctrlPort     sim.Port
	localMemPort sim.Port

	RemotePMCAddressTable mem.AddressToPortMapper

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

	requestingPMCtrlPort sim.RemotePort

	numDataRspPendingForPageMigration int
	reqIDToWriteAddressMap            map[string]uint64

	MemCtrlFinder mem.AddressToPortMapper

	DataTransferStartTime sim.VTimeInSec
	DataTransferEndTime   sim.VTimeInSec
	TotalDataTransferTime sim.VTimeInSec

	isHandlingPageMigration bool
}

// Tick updates the status of a PageMigrationController.
//
//nolint:gocyclo
func (e *PageMigrationController) Tick() bool {
	madeProgress := false

	madeProgress = e.sendMigrationReqToAnotherPMC() || madeProgress
	madeProgress = e.sendReadReqLocalMemPort() || madeProgress
	madeProgress = e.sendMigrationCompleteRspToCtrlPort() || madeProgress
	madeProgress = e.sendDataReadyRspToRequestingPMC() || madeProgress
	madeProgress = e.sendWriteReqLocalMemPort() || madeProgress
	madeProgress = e.processFromOutside() || madeProgress
	madeProgress = e.processFromCtrlPort() || madeProgress
	madeProgress = e.processFromMemCtrl() || madeProgress
	madeProgress = e.processPageMigrationReqFromCtrlPort() || madeProgress
	madeProgress = e.processReadPageReqFromAnotherPMC() || madeProgress
	madeProgress = e.processDataReadyRspFromMemCtrl() || madeProgress
	madeProgress = e.processDataPullRsp() || madeProgress
	madeProgress = e.processWriteDoneRspFromMemCtrl() || madeProgress

	return madeProgress
}

func (e *PageMigrationController) processFromOutside() bool {
	req := e.remotePort.PeekIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *DataPullReq:
		return e.handleDataPullReq(req)
	case *DataPullRsp:
		return e.handleDataPullRsp(req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (e *PageMigrationController) processFromCtrlPort() bool {
	//PMC handles only one page migration req at a time
	if e.isHandlingPageMigration {
		return false
	}

	req := e.ctrlPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	e.DataTransferStartTime = e.TickingComponent.TickScheduler.CurrentTime()

	switch req := req.(type) {
	case *PageMigrationReqToPMC:
		return e.handleMigrationReqFromCtrlPort(req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (e *PageMigrationController) handleMigrationReqFromCtrlPort(
	req *PageMigrationReqToPMC,
) bool {
	e.currentMigrationRequest = req
	return true
}

func (e *PageMigrationController) processPageMigrationReqFromCtrlPort() bool {
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
			WithSrc(e.remotePort.AsRemote()).
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

func (e *PageMigrationController) sendMigrationReqToAnotherPMC() bool {
	if len(e.toPullFromAnotherPMC) == 0 {
		return false
	}

	madeProgress := false
	newInPullFromAnotherPMC := make([]*DataPullReq, 0)

	for i := 0; i < len(e.toPullFromAnotherPMC); i++ {
		sendPacket := e.toPullFromAnotherPMC[i]
		sendErr := e.remotePort.Send(sendPacket)
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
	req *DataPullReq,
) bool {
	e.remotePort.RetrieveIncoming()
	e.currentPullReqFromAnotherPMC = append(e.currentPullReqFromAnotherPMC, req)
	e.requestingPMCtrlPort = req.Src
	return true
}

func (e *PageMigrationController) processReadPageReqFromAnotherPMC() bool {
	if e.currentPullReqFromAnotherPMC == nil {
		return false
	}

	for i := 0; i < len(e.currentPullReqFromAnotherPMC); i++ {
		address := e.currentPullReqFromAnotherPMC[i].ToReadFromPhyAddress
		dataTransferSize := e.currentPullReqFromAnotherPMC[i].DataTransferSize
		req := mem.ReadReqBuilder{}.
			WithSrc(e.localMemPort.AsRemote()).
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

func (e *PageMigrationController) sendReadReqLocalMemPort() bool {
	if len(e.toSendLocalMemPort) == 0 {
		return false
	}

	madeProgress := false
	newInToSendLocalMemPort := make([]*mem.ReadReq, 0)

	for i := 0; i < len(e.toSendLocalMemPort); i++ {
		sendPacket := e.toSendLocalMemPort[i]
		sendErr := e.localMemPort.Send(sendPacket)
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

func (e *PageMigrationController) processFromMemCtrl() bool {
	req := e.localMemPort.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		return e.handleDataReadyRspFromMemCtrl(req)
	case *mem.WriteDoneRsp:
		return e.handleWriteDoneRspFromMemCtrl(req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
		return false
	}
}

func (e *PageMigrationController) handleDataReadyRspFromMemCtrl(
	rsp *mem.DataReadyRsp,
) bool {
	e.dataReadyRspFromMemCtrl = append(e.dataReadyRspFromMemCtrl, rsp)
	return true
}

func (e *PageMigrationController) processDataReadyRspFromMemCtrl() bool {
	if e.dataReadyRspFromMemCtrl == nil {
		return false
	}

	for i := 0; i < len(e.dataReadyRspFromMemCtrl); i++ {
		data := e.dataReadyRspFromMemCtrl[i].Data
		rsp := DataPullRspBuilder{}.
			WithSrc(e.remotePort.AsRemote()).
			WithDst(e.requestingPMCtrlPort).
			WithData(data).
			Build()
		rsp.ID = e.dataReadyRspFromMemCtrl[i].RespondTo

		e.toRspToAnotherPMC = append(e.toRspToAnotherPMC, rsp)
	}

	e.dataReadyRspFromMemCtrl = nil
	return true
}

func (e *PageMigrationController) sendDataReadyRspToRequestingPMC() bool {
	if len(e.toRspToAnotherPMC) == 0 {
		return false
	}

	madeProgress := false
	newInToSendRspToAnotherPMC := make([]*DataPullRsp, 0)

	for i := 0; i < len(e.toRspToAnotherPMC); i++ {
		sendPacket := e.toRspToAnotherPMC[i]
		sendErr := e.remotePort.Send(sendPacket)
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
	req *DataPullRsp,
) bool {
	e.receivedDataFromAnothePMC = append(e.receivedDataFromAnothePMC, req)
	e.remotePort.RetrieveIncoming()
	return true
}

func (e *PageMigrationController) processDataPullRsp() bool {
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
			WithSrc(e.localMemPort.AsRemote()).
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

func (e *PageMigrationController) sendWriteReqLocalMemPort() bool {
	if e.writeReqLocalMemPort == nil {
		return false
	}

	madeProgress := false
	newInWriteReqLocalMemPort := make([]*mem.WriteReq, 0)

	for i := 0; i < len(e.writeReqLocalMemPort); i++ {
		err := e.localMemPort.Send(e.writeReqLocalMemPort[i])
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
	rsp *mem.WriteDoneRsp,
) bool {
	e.receivedWriteDoneFromMemCtrl = rsp
	return true
}

func (e *PageMigrationController) processWriteDoneRspFromMemCtrl() bool {
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
			WithSrc(e.ctrlPort.AsRemote()).
			WithDst(e.currentMigrationRequest.Src).
			Build()

		e.toSendToCtrlPort = rsp
		e.currentMigrationRequest = nil
		e.numDataRspPendingForPageMigration = -1
	}

	return true
}

func (e *PageMigrationController) sendMigrationCompleteRspToCtrlPort() bool {
	if e.toSendToCtrlPort == nil {
		return false
	}

	err := e.ctrlPort.Send(e.toSendToCtrlPort)

	if err == nil {
		e.DataTransferEndTime = e.TickingComponent.TickScheduler.CurrentTime()
		e.TotalDataTransferTime = e.TotalDataTransferTime + (e.DataTransferEndTime - e.DataTransferStartTime)
		e.isHandlingPageMigration = false
		e.currentMigrationRequest = nil
		e.toSendToCtrlPort = nil
		return true
	}

	return false
}

// SetFreq sets freq
func (e *PageMigrationController) SetFreq(freq sim.Freq) {
	panic("not implemented")
}

// NewPageMigrationController returns a new controller
func NewPageMigrationController(
	name string,
	engine sim.Engine,
	memCtrlFinder mem.AddressToPortMapper,
	remoteModules mem.AddressToPortMapper,
) *PageMigrationController {
	e := new(PageMigrationController)
	e.TickingComponent = sim.NewTickingComponent(name, engine, 1*sim.GHz, e)
	e.MemCtrlFinder = memCtrlFinder

	e.remotePort = sim.NewPort(e, 1, 1, name+".RemotePort")
	e.AddPort("Remote", e.remotePort)

	e.localMemPort = sim.NewPort(e, 1, 1, name+"LocalMemPort")
	e.AddPort("LocalMem", e.localMemPort)

	e.ctrlPort = sim.NewPort(e, 1, 1, name+"CtrlPort")
	e.AddPort("Control", e.ctrlPort)

	e.RemotePMCAddressTable = remoteModules

	e.onDemandPagingDataTransferSize = 64
	e.numDataRspPendingForPageMigration = -1

	e.reqIDToWriteAddressMap = make(map[string]uint64)

	return e
}
