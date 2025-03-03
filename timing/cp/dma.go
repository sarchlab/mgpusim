package cp

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
	"github.com/sarchlab/mgpusim/v4/protocol"
)

// A RequestCollection contains a single MemCopy Msg and the IDs of the Read/Write
// requests that correspond to it, as well as the number of remaining requests
type RequestCollection struct {
	superiorRequest       sim.Msg
	subordinateRequestIDs []string
	subordinateCount      int
}

// removeIDIfExists reduces the subordinate count if a specific ID is present in the list
// of subordinate IDs, returning true if it was and false if it was not
func (rqC *RequestCollection) decrementCountIfExists(id string) bool {
	for _, str := range rqC.subordinateRequestIDs {
		if id == str {
			rqC.subordinateCount -= 1
			return true
		}
	}
	return false
}

// isFinished returns true if the subordinate count is zero (i.e. the superior request is finished processing)
func (rqC *RequestCollection) isFinished() bool {
	return rqC.subordinateCount == 0
}

func (rqC *RequestCollection) getSuperior() sim.Msg {
	return rqC.superiorRequest
}

func (rqC *RequestCollection) getSuperiorID() string {
	return rqC.superiorRequest.Meta().ID
}

// appendSubordinateID adds a message ID to the list and increases the count
func (rqC *RequestCollection) appendSubordinateID(id string) {
	rqC.subordinateRequestIDs = append(rqC.subordinateRequestIDs, id)
	rqC.subordinateCount += 1
}

func NewRequestCollection(
	superiorRequest sim.Msg,
) *RequestCollection {
	rqC := new(RequestCollection)
	rqC.superiorRequest = superiorRequest
	rqC.subordinateCount = 0
	return rqC
}

// A DMAEngine is responsible for accessing data that does not belongs to
// the GPU that the DMAEngine works in.
type DMAEngine struct {
	*sim.TickingComponent

	Log2AccessSize uint64

	localDataSource mem.AddressToPortMapper

	processingReqs []*RequestCollection

	processingReq   sim.Msg
	maxRequestCount uint64

	toSendToMem []sim.Msg
	toSendToCP  []sim.Msg
	pendingReqs []sim.Msg

	ToCP  sim.Port
	ToMem sim.Port
}

// SetLocalDataSource sets the table that maps from addresses to port that can
// provide the data.
func (dma *DMAEngine) SetLocalDataSource(s mem.AddressToPortMapper) {
	dma.localDataSource = s
}

// Tick ticks
func (dma *DMAEngine) Tick() bool {
	madeProgress := false

	madeProgress = dma.send(dma.ToCP, &dma.toSendToCP) || madeProgress
	madeProgress = dma.send(dma.ToMem, &dma.toSendToMem) || madeProgress
	madeProgress = dma.parseFromMem() || madeProgress
	madeProgress = dma.parseFromCP() || madeProgress

	return madeProgress
}

func (dma *DMAEngine) send(
	port sim.Port,
	reqs *[]sim.Msg,
) bool {
	if len(*reqs) == 0 {
		return false
	}

	req := (*reqs)[0]
	err := port.Send(req)
	if err == nil {
		*reqs = (*reqs)[1:]
		return true
	}

	return false
}

func (dma *DMAEngine) parseFromMem() bool {
	req := dma.ToMem.RetrieveIncoming()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		dma.processDataReadyRsp(req)
	case *mem.WriteDoneRsp:
		dma.processDoneRsp(req)
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (dma *DMAEngine) processDataReadyRsp(
	rsp *mem.DataReadyRsp,
) {
	req := dma.removeReqFromPendingReqList(rsp.RespondTo).(*mem.ReadReq)
	tracing.TraceReqFinalize(req, dma)

	found := false
	result := &RequestCollection{}
	for _, rc := range dma.processingReqs {
		if rc.decrementCountIfExists(req.Meta().ID) {
			result = rc
			found = true
		}
	}

	if !found {
		panic("couldn't find requestcollection")
	}

	processing := result.getSuperior().(*protocol.MemCopyD2HReq)

	offset := req.Address - processing.SrcAddress
	copy(processing.DstBuffer[offset:], rsp.Data)
	// fmt.Printf("Dma DataReady %x, %v\n", req.Address, rsp.Data)

	if result.isFinished() {
		tracing.TraceReqComplete(processing, dma)
		dma.removeReqFromProcessingReqList(processing.Meta().ID)

		rsp := sim.GeneralRspBuilder{}.
			WithDst(processing.Src).
			WithSrc(processing.Dst).
			WithOriginalReq(processing).
			Build()
		dma.toSendToCP = append(dma.toSendToCP, rsp)
	}
}

func (dma *DMAEngine) processDoneRsp(
	rsp *mem.WriteDoneRsp,
) {
	r := dma.removeReqFromPendingReqList(rsp.RespondTo)
	tracing.TraceReqFinalize(r, dma)

	found := false
	result := &RequestCollection{}
	for _, rc := range dma.processingReqs {
		if rc.decrementCountIfExists(r.Meta().ID) {
			result = rc
			found = true
		}
	}

	if !found {
		panic("couldn't find requestcollection")
	}

	if result.isFinished() {
		processing := result.getSuperior().(*protocol.MemCopyH2DReq)
		tracing.TraceReqComplete(processing, dma)
		dma.removeReqFromProcessingReqList(processing.Meta().ID)

		rsp := sim.GeneralRspBuilder{}.
			WithDst(processing.Src).
			WithSrc(processing.Dst).
			WithOriginalReq(processing).
			Build()
		dma.toSendToCP = append(dma.toSendToCP, rsp)
	}
}

func (dma *DMAEngine) removeReqFromPendingReqList(id string) sim.Msg {
	var reqToRet sim.Msg
	newList := make([]sim.Msg, 0, len(dma.pendingReqs)-1)
	for _, r := range dma.pendingReqs {
		if r.Meta().ID == id {
			reqToRet = r
		} else {
			newList = append(newList, r)
		}
	}
	dma.pendingReqs = newList

	if reqToRet == nil {
		panic("not found")
	}

	return reqToRet
}

func (dma *DMAEngine) removeReqFromProcessingReqList(id string) {
	found := false
	newList := make([]*RequestCollection, 0, len(dma.processingReqs)-1)
	for _, r := range dma.processingReqs {
		if r.getSuperiorID() == id {
			found = true
		} else {
			newList = append(newList, r)
		}
	}
	dma.processingReqs = newList

	if !found {
		panic("not found")
	}
}

func (dma *DMAEngine) parseFromCP() bool {
	if uint64(len(dma.processingReqs)) >= dma.maxRequestCount {
		return false
	}

	req := dma.ToCP.RetrieveIncoming()
	if req == nil {
		return false
	}
	tracing.TraceReqReceive(req, dma)

	rqC := NewRequestCollection(req)

	dma.processingReqs = append(dma.processingReqs, rqC)
	switch req := req.(type) {
	case *protocol.MemCopyH2DReq:
		dma.parseMemCopyH2D(req, rqC)
	case *protocol.MemCopyD2HReq:
		dma.parseMemCopyD2H(req, rqC)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (dma *DMAEngine) parseMemCopyH2D(
	req *protocol.MemCopyH2DReq,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.SrcBuffer))
	addr := req.DstAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << dma.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << dma.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := dma.localDataSource.Find(addr)
		reqToBottom := mem.WriteReqBuilder{}.
			WithSrc(dma.ToMem.AsRemote()).
			WithDst(module).
			WithAddress(addr).
			WithData(req.SrcBuffer[offset : offset+length]).
			Build()
		dma.toSendToMem = append(dma.toSendToMem, reqToBottom)
		dma.pendingReqs = append(dma.pendingReqs, reqToBottom)
		rqC.appendSubordinateID(reqToBottom.Meta().ID)

		tracing.TraceReqInitiate(reqToBottom, dma,
			tracing.MsgIDAtReceiver(req, dma))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (dma *DMAEngine) parseMemCopyD2H(
	req *protocol.MemCopyD2HReq,
	rqC *RequestCollection,
) {
	offset := uint64(0)
	lengthLeft := uint64(len(req.DstBuffer))
	addr := req.SrcAddress

	for lengthLeft > 0 {
		addrUnitFirstByte := addr & (^uint64(0) << dma.Log2AccessSize)
		unitOffset := addr - addrUnitFirstByte
		lengthInUnit := (1 << dma.Log2AccessSize) - unitOffset

		length := lengthLeft
		if lengthInUnit < length {
			length = lengthInUnit
		}

		module := dma.localDataSource.Find(addr)
		reqToBottom := mem.ReadReqBuilder{}.
			WithSrc(dma.ToMem.AsRemote()).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		dma.toSendToMem = append(dma.toSendToMem, reqToBottom)
		dma.pendingReqs = append(dma.pendingReqs, reqToBottom)
		rqC.appendSubordinateID(reqToBottom.Meta().ID)

		tracing.TraceReqInitiate(reqToBottom, dma,
			tracing.MsgIDAtReceiver(req, dma))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

// NewDMAEngine creates a DMAEngine, injecting a engine and a "LowModuleFinder"
// that helps with locating the module that holds the data.
func NewDMAEngine(
	name string,
	engine sim.Engine,
	localDataSource mem.AddressToPortMapper,
) *DMAEngine {
	dma := new(DMAEngine)
	dma.TickingComponent = sim.NewTickingComponent(
		name, engine, 1*sim.GHz, dma)

	dma.Log2AccessSize = 6
	dma.localDataSource = localDataSource

	dma.maxRequestCount = 4

	dma.ToCP = sim.NewPort(dma, 40960000, 40960000, name+".ToCP")
	dma.ToMem = sim.NewPort(dma, 64, 64, name+".ToMem")

	return dma
}
