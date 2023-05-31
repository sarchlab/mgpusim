package cp

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/protocol"
)

// A DMAEngine is responsible for accessing data that does not belongs to
// the GPU that the DMAEngine works in.
type DMAEngine struct {
	*sim.TickingComponent

	Log2AccessSize uint64

	localDataSource mem.LowModuleFinder

	processingReq sim.Msg

	toSendToMem []sim.Msg
	toSendToCP  []sim.Msg
	pendingReqs []sim.Msg

	ToCP  sim.Port
	ToMem sim.Port
}

// SetLocalDataSource sets the table that maps from addresses to port that can
// provide the data.
func (dma *DMAEngine) SetLocalDataSource(s mem.LowModuleFinder) {
	dma.localDataSource = s
}

// Tick ticks
func (dma *DMAEngine) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = dma.send(now, dma.ToCP, &dma.toSendToCP) || madeProgress
	madeProgress = dma.send(now, dma.ToMem, &dma.toSendToMem) || madeProgress
	madeProgress = dma.parseFromMem(now) || madeProgress
	madeProgress = dma.parseFromCP(now) || madeProgress

	return madeProgress
}

func (dma *DMAEngine) send(
	now sim.VTimeInSec,
	port sim.Port,
	reqs *[]sim.Msg,
) bool {
	if len(*reqs) == 0 {
		return false
	}

	req := (*reqs)[0]
	req.Meta().SendTime = now
	err := port.Send(req)
	if err == nil {
		*reqs = (*reqs)[1:]
		return true
	}

	return false
}

func (dma *DMAEngine) parseFromMem(now sim.VTimeInSec) bool {
	req := dma.ToMem.Retrieve(now)
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		dma.processDataReadyRsp(now, req)
	case *mem.WriteDoneRsp:
		dma.processDoneRsp(now, req)
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (dma *DMAEngine) processDataReadyRsp(
	now sim.VTimeInSec,
	rsp *mem.DataReadyRsp,
) {
	req := dma.removeReqFromPendingReqList(rsp.RespondTo).(*mem.ReadReq)
	tracing.TraceReqFinalize(req, dma)

	processing := dma.processingReq.(*protocol.MemCopyD2HReq)

	offset := req.Address - processing.SrcAddress
	copy(processing.DstBuffer[offset:], rsp.Data)
	// fmt.Printf("Dma DataReady %x, %v\n", req.Address, rsp.Data)

	if len(dma.pendingReqs) == 0 {
		tracing.TraceReqComplete(dma.processingReq, dma)
		dma.processingReq = nil

		rsp := sim.GeneralRspBuilder{}.
			WithDst(processing.Src).
			WithSrc(processing.Dst).
			WithSendTime(now).
			WithOriginalReq(processing).
			Build()
		dma.toSendToCP = append(dma.toSendToCP, rsp)
	}
}

func (dma *DMAEngine) processDoneRsp(
	now sim.VTimeInSec,
	rsp *mem.WriteDoneRsp,
) {
	r := dma.removeReqFromPendingReqList(rsp.RespondTo)
	tracing.TraceReqFinalize(r, dma)

	processing := dma.processingReq.(*protocol.MemCopyH2DReq)
	if len(dma.pendingReqs) == 0 {
		tracing.TraceReqComplete(dma.processingReq, dma)
		dma.processingReq = nil

		rsp := sim.GeneralRspBuilder{}.
			WithDst(processing.Src).
			WithSrc(processing.Dst).
			WithSendTime(now).
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

func (dma *DMAEngine) parseFromCP(now sim.VTimeInSec) bool {
	if dma.processingReq != nil {
		return false
	}

	req := dma.ToCP.Retrieve(now)
	if req == nil {
		return false
	}
	tracing.TraceReqReceive(req, dma)

	dma.processingReq = req
	switch req := req.(type) {
	case *protocol.MemCopyH2DReq:
		dma.parseMemCopyH2D(now, req)
	case *protocol.MemCopyD2HReq:
		dma.parseMemCopyD2H(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}

	return true
}

func (dma *DMAEngine) parseMemCopyH2D(
	now sim.VTimeInSec,
	req *protocol.MemCopyH2DReq,
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
			WithSendTime(now).
			WithSrc(dma.ToMem).
			WithDst(module).
			WithAddress(addr).
			WithData(req.SrcBuffer[offset : offset+length]).
			Build()
		dma.toSendToMem = append(dma.toSendToMem, reqToBottom)
		dma.pendingReqs = append(dma.pendingReqs, reqToBottom)

		tracing.TraceReqInitiate(reqToBottom, dma,
			tracing.MsgIDAtReceiver(dma.processingReq, dma))

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (dma *DMAEngine) parseMemCopyD2H(
	now sim.VTimeInSec,
	req *protocol.MemCopyD2HReq,
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
			WithSendTime(now).
			WithSrc(dma.ToMem).
			WithDst(module).
			WithAddress(addr).
			WithByteSize(length).
			Build()
		dma.toSendToMem = append(dma.toSendToMem, reqToBottom)
		dma.pendingReqs = append(dma.pendingReqs, reqToBottom)

		tracing.TraceReqInitiate(reqToBottom, dma,
			tracing.MsgIDAtReceiver(dma.processingReq, dma))

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
	localDataSource mem.LowModuleFinder,
) *DMAEngine {
	dma := new(DMAEngine)
	dma.TickingComponent = sim.NewTickingComponent(
		name, engine, 1*sim.GHz, dma)

	dma.Log2AccessSize = 6
	dma.localDataSource = localDataSource

	dma.ToCP = sim.NewLimitNumMsgPort(dma, 40960000, name+".ToCP")
	dma.ToMem = sim.NewLimitNumMsgPort(dma, 64, name+".ToMem")

	return dma
}
