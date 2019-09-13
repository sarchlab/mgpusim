package pagemigrationcontroller

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mem/vm/tlb"
	"log"
	"reflect"
)

type PageMigrationController struct {
	*akita.ComponentBase
	ticker *akita.Ticker

	ToOutside akita.Port
	ToCtrlPort      akita.Port
	CtrlPort       akita.Port
	ToMemCtrl akita.Port

	engine         akita.Engine
	RemotePMCAddressTable cache.LowModuleFinder



	currentMigrationRequest       *PageMigrationReqToPMC
	currentPullReqFromAnotherPMC []*DataPullReqToRemotePMC

	toPullFromAnotherPMC        []*DataPullReqToRemotePMC
	toSendToMemCtrl              []*mem.ReadReq
	dataReadyRspFromMemCtrl      []*mem.DataReadyRsp
	toRspToAnotherPMC           []*DataPullRspFromRemotePMC
	receivedDataFromAnothePMC   []*DataPullRspFromRemotePMC
	writeReqToMemCtrl            []*mem.WriteReq
	receivedWriteDoneFromMemCtrl *mem.DoneRsp
	toSendToCtrlPort                  *PageMigrationRspFromPMC

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


	ishandlingPageMigration        bool

	requestingRDMAPort akita.Port

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
	e.sendReadReqtoMemCtrl(now)
	e.sendMigrationCompleteRspToCtrlPort(now)
	e.sendDataReadyRspToRequestingPMC(now)
	e.sendWriteReqToMemCtrl(now)
	e.sendWriteReqToMemCtrl(now)
	e.processFromOutside(now)
	e.processFromCtrlPort(now)
	e.processFromMemCtrl(now)
	e.processPageMigrationReqFromCtrlPort(now)
	e.processReadPageReqFromAnotherPMC(now)
	e.processDataReadyRspFromMemCtrl(now)
	e.processDataPullRspFromRemotePMC(now)
	e.processWriteDoneRspFromMemCtrl(now)

	e.needTick = true
	if e.needTick {
		e.ticker.TickLater(now)
	}
}




func (e *PageMigrationController) processFromOutside(now akita.VTimeInSec) {
	req := e.ToOutside.Peek()
	if req == nil {
		return
	}

	switch req := req.(type) {

	case *DataPullReqToRemotePMC:
		e.handleDataPullReqToRemotePMC(now, req)
	case *DataPullRspFromRemotePMC:
		e.handleDataPullRspFromRemotePMC(now, req)

	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
}


func (e *PageMigrationController) processFromCtrlPort(now akita.VTimeInSec) {
	//PMC handles only one page migration req at a time
	if e.ishandlingPageMigration {
		return
	}

	req := e.ToCtrlPort.Retrieve(now)
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
	e.ishandlingPageMigration = true
}



func (e *PageMigrationController) processPageMigrationReqFromCtrlPort(now akita.VTimeInSec) {

	if e.currentMigrationRequest == nil {
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
		req := DataPullReqToRemotePMCBuilder{}.
			WithSendTime(now).
			WithSrc(e.ToOutside).
			WithDst(destination).
			WithDataTransferSize(e.onDemandPagingDataTransferSize).
			WithReadFromPhyAddress(startingPhysicalAddress).
			Build()
		startingPhysicalAddress = startingPhysicalAddress + e.onDemandPagingDataTransferSize
		e.toPullFromAnotherPMC = append(e.toPullFromAnotherPMC, req)
		e.reqIDToWriteAddressMap[req.ID] = currentWriteAddress
		currentWriteAddress = currentWriteAddress + e.onDemandPagingDataTransferSize
	}

	e.currentMigrationRequest = nil

	e.needTick = true

}

func (e *PageMigrationController) sendMigrationReqToAnotherPMC(now akita.VTimeInSec) {
	if len(e.toPullFromAnotherPMC) == 0 {
		return
	}

	newInPullFromAnotherPMC := make([]*DataPullReqToRemotePMC, 0)

	for i := 0; i < len(e.toPullFromAnotherPMC); i++ {
		sendPacket := e.toPullFromAnotherPMC[i]
		sendPacket.SendTime = now
		sendErr := e.ToOutside.Send(sendPacket)
		if sendErr == nil {
			e.needTick = true
		} else {
			newInPullFromAnotherPMC = append(newInPullFromAnotherPMC, sendPacket)
		}
	}

	e.toPullFromAnotherPMC = newInPullFromAnotherPMC

}

func (e *PageMigrationController) handleDataPullReqToRemotePMC(now akita.VTimeInSec, req *DataPullReqToRemotePMC) {
	e.ToOutside.Retrieve(now)
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
		req := mem.NewReadReq(now, e.ToMemCtrl, e.MemCtrlFinder.Find(address), address, dataTransferSize)
		req.ID = e.currentPullReqFromAnotherPMC[i].ID
		e.toSendToMemCtrl = append(e.toSendToMemCtrl, req)
	}

	e.currentPullReqFromAnotherPMC = nil
	e.needTick = true

}

func (e *PageMigrationController) sendReadReqtoMemCtrl(now akita.VTimeInSec) {

	if len(e.toSendToMemCtrl) == 0 {
		return
	}

	newInToSendToMemCtrl := make([]*mem.ReadReq, 0)

	for i := 0; i < len(e.toSendToMemCtrl); i++ {
		sendPacket := e.toSendToMemCtrl[i]
		sendPacket.SendTime = now
		sendErr := e.ToMemCtrl.Send(sendPacket)
		if sendErr == nil {
			e.needTick = true
		} else {
			newInToSendToMemCtrl = append(newInToSendToMemCtrl, sendPacket)
		}
	}

	e.toSendToMemCtrl = newInToSendToMemCtrl

}

func (e *PageMigrationController) processFromMemCtrl(now akita.VTimeInSec) {
	req := e.ToMemCtrl.Retrieve(now)
	if req == nil {
		return
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		e.handleDataReadyRspFromMemCtrl(now, req)
	case *mem.DoneRsp:
		e.handleWriteDoneRspFromMemCtrl(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))

	}
}

func (e *PageMigrationController) handleDataReadyRspFromMemCtrl(now akita.VTimeInSec, rsp *mem.DataReadyRsp) {
	e.needTick = true
	e.dataReadyRspFromMemCtrl = append(e.dataReadyRspFromMemCtrl, rsp)

}

func (e *PageMigrationController) processDataReadyRspFromMemCtrl(now akita.VTimeInSec) {
	if e.dataReadyRspFromMemCtrl == nil {
		return
	}

	for i := 0; i < len(e.dataReadyRspFromMemCtrl); i++ {
		data := e.dataReadyRspFromMemCtrl[i].Data
		rsp := NewDataPullRspFromRemotePMCToAnotherPM(now, e.ToOutside, e.requestingPMCtrlPortort, data)
		rsp.ID = e.dataReadyRspFromMemCtrl[i].RespondTo
		//log.Printf("Read data address ID from memctrl  is %d \n", rsp.ID)

		e.toRspToAnotherPMC = append(e.toRspToAnotherPMC, rsp)

	}

	e.dataReadyRspFromMemCtrl = nil
	e.needTick = true

}

func (e *PageMigrationController) sendDataReadyRspToRequestingPMC(now akita.VTimeInSec) {
	if len(e.toRspToAnotherPMC) == 0 {
		return
	}

	newInToSendRspToAnotherPMC := make([]*DataPullRspFromRemotePMC, 0)

	for i := 0; i < len(e.toRspToAnotherPMC); i++ {
		sendPacket := e.toRspToAnotherPMC[i]
		sendPacket.SendTime = now
		sendErr := e.ToOutside.Send(sendPacket)
		if sendErr == nil {
			//log.Printf("rEPLYING FOR REQ to PMC %d ", e.toRspToAnotherPMC[i].ID)
			e.needTick = true
		} else {
			newInToSendRspToAnotherPMC = append(newInToSendRspToAnotherPMC, sendPacket)
		}
	}

	e.toRspToAnotherPMC = newInToSendRspToAnotherPMC
	//log.Printf("rdma %s sending  RSP length %d \n", e.Name(), len(e.toRspToAnotherPMC))

}

func (e *PageMigrationController) handleDataPullRspFromRemotePMC(now akita.VTimeInSec, req *DataPullRspFromRemotePMC) {
	e.needTick = true
	e.receivedDataFromAnothePMC = append(e.receivedDataFromAnothePMC, req)
	//log.Printf(" RSP from  PMC %d", req.ID)
	e.ToOutside.Retrieve(now)

}

func (e *PageMigrationController) processDataPullRspFromRemotePMC(now akita.VTimeInSec) {
	if e.receivedDataFromAnothePMC == nil {
		return
	}

	for i := 0; i < len(e.receivedDataFromAnothePMC); i++ {
		data := e.receivedDataFromAnothePMC[i].Data
		address, found := e.reqIDToWriteAddressMap[e.receivedDataFromAnothePMC[i].ID]
		if !found {
			log.Panicf("We do not know where the mem controller should write")
		}
		//log.Printf("PMC writing to address %d  with length %d \n", address, len(data))
		req := mem.NewWriteReq(now, e.ToMemCtrl, e.MemCtrlFinder.Find(address), address)
		req.Data = data
		e.writeReqToMemCtrl = append(e.writeReqToMemCtrl, req)
		delete(e.reqIDToWriteAddressMap, e.receivedDataFromAnothePMC[i].ID)
	}

	e.receivedDataFromAnothePMC = nil
	e.needTick = true

}

func (e *PageMigrationController) sendWriteReqToMemCtrl(now akita.VTimeInSec) {
	if e.writeReqToMemCtrl == nil {
		return
	}

	newInWriteReqToMemCtrl := make([]*mem.WriteReq, 0)

	for i := 0; i < len(e.writeReqToMemCtrl); i++ {
		e.writeReqToMemCtrl[i].SetSendTime(now)
		err := e.ToMemCtrl.Send(e.writeReqToMemCtrl[i])
		if err == nil {
			//log.Printf("Sending write req to mem ctrl with ID %d", e.writeReqToMemCtrl[i].ID)
			e.needTick = true
		} else {
			newInWriteReqToMemCtrl = append(newInWriteReqToMemCtrl, e.writeReqToMemCtrl[i])
		}

	}

	e.writeReqToMemCtrl = newInWriteReqToMemCtrl
}

func (e *PageMigrationController) handleWriteDoneRspFromMemCtrl(now akita.VTimeInSec, rsp *mem.DoneRsp) {
	e.needTick = true
	e.receivedWriteDoneFromMemCtrl = rsp
}

func (e *PageMigrationController) processWriteDoneRspFromMemCtrl(now akita.VTimeInSec) {
	if e.receivedWriteDoneFromMemCtrl == nil {
		return
	}

	e.numDataRspPendingForPageMigration--
	//log.Printf("Pending rsp %d \n", e.numDataRspPendingForPageMigration)
	//log.Printf("Write done for req %d \n", e.receivedWriteDoneFromMemCtrl.ID)
	e.receivedWriteDoneFromMemCtrl = nil
	e.needTick = true

	if e.numDataRspPendingForPageMigration < 0 {
		log.Panicf("Not possible")
	}
	if e.numDataRspPendingForPageMigration == 0 {
		//log.Printf("Sending migration complete rsp to CtrlPort \n")
		rsp := NewOnDemandPageMigrationRsp(now, e.ToCtrlPort, e.CtrlPort, true)
		e.toSendToCtrlPort = rsp
		e.numDataRspPendingForPageMigration = -1

	}

}

func (e *PageMigrationController) sendMigrationCompleteRspToCtrlPort(now akita.VTimeInSec) {
	if e.toSendToCtrlPort == nil {
		return
	}

	e.toSendToCtrlPort.SetSendTime(now)
	err := e.ToCtrlPort.Send(e.toSendToCtrlPort)

	if err == nil {
		e.DataTransferEndTime = now
		e.TotalDataTransferTime = e.TotalDataTransferTime + (e.DataTransferEndTime - e.DataTransferStartTime)
		e.needTick = true
		e.ishandlingPageMigration = false
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


func NewEngine(
	name string,
	engine akita.Engine,
	memCtrlFinder cache.LowModuleFinder,
) *PageMigrationController {
	e := new(PageMigrationController)
	e.freq = 1 * akita.GHz
	e.ComponentBase = akita.NewComponentBase(name)
	e.ticker = akita.NewTicker(e, engine, e.freq)

	e.engine = engine
	e.MemCtrlFinder = memCtrlFinder

	e.ToOutside = akita.NewLimitNumReqPort(e, 1)
	e.ToMemCtrl = akita.NewLimitNumReqPort(e, 1)
	e.ToCtrlPort = akita.NewLimitNumReqPort(e, 1)
	e.CtrlPort = akita.NewLimitNumReqPort(e, 1)

	e.onDemandPagingDataTransferSize = 256
	e.numDataRspPendingForPageMigration = -1

	e.reqIDToWriteAddressMap = make(map[string]uint64)
	e.requestingRDMAPort = akita.NewLimitNumReqPort(e, 1)

	return e
}

