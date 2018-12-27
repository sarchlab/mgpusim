package gcn3

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

// A DMAEngine is responsible for accessing data that does not belongs to
// the GPU that the DMAEngine works in.
type DMAEngine struct {
	*akita.TickingComponent

	Log2AccessSize uint64

	localDataSource cache.LowModuleFinder

	processingReq akita.Req

	toSendToMem []akita.Req
	toSendToCP  []akita.Req
	pendingReqs []akita.Req

	ToCP  akita.Port
	ToMem akita.Port
}

func (dma *DMAEngine) Handle(evt akita.Event) error {
	switch evt := evt.(type) {
	case akita.TickEvent:
		return dma.tick(evt)
	default:
		log.Panicf("cannot handle event for type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (dma *DMAEngine) tick(evt akita.TickEvent) error {
	now := evt.Time()
	dma.NeedTick = false

	dma.send(now, dma.ToCP, &dma.toSendToCP)
	dma.send(now, dma.ToMem, &dma.toSendToMem)
	dma.parseFromMem(now)
	dma.parseFromCP(now)

	if dma.NeedTick == true {
		dma.TickLater(now)
	}

	return nil
}

func (dma *DMAEngine) send(
	now akita.VTimeInSec,
	port akita.Port,
	reqs *[]akita.Req,
) {
	if len(*reqs) == 0 {
		return
	}

	req := (*reqs)[0]
	req.SetSendTime(now)
	err := port.Send(req)
	if err == nil {
		dma.NeedTick = true
		*reqs = (*reqs)[1:]
	}
}

func (dma *DMAEngine) parseFromMem(now akita.VTimeInSec) {
	req := dma.ToMem.Retrieve(now)
	if req == nil {
		return
	}

	dma.NeedTick = true

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		dma.processDataReadyRsp(now, req)
	case *mem.DoneRsp:
		dma.processDoneRsp(now, req)
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(req))
	}
}

func (dma *DMAEngine) processDataReadyRsp(
	now akita.VTimeInSec,
	rsp *mem.DataReadyRsp,
) {
	req := dma.removeReqFromPendingReqList(rsp.RespondTo).(*mem.ReadReq)
	processing := dma.processingReq.(*MemCopyD2HReq)

	offset := req.Address - processing.SrcAddress
	copy(processing.DstBuffer[offset:], rsp.Data)

	if len(dma.pendingReqs) == 0 {
		dma.processingReq = nil
		processing.SwapSrcAndDst()
		dma.toSendToCP = append(dma.toSendToCP, processing)
	}
}

func (dma *DMAEngine) processDoneRsp(
	now akita.VTimeInSec,
	rsp *mem.DoneRsp,
) {
	dma.removeReqFromPendingReqList(rsp.RespondTo)
	processing := dma.processingReq.(*MemCopyH2DReq)
	if len(dma.pendingReqs) == 0 {
		dma.processingReq = nil
		processing.SwapSrcAndDst()
		dma.toSendToCP = append(dma.toSendToCP, processing)
	}
}

func (dma *DMAEngine) removeReqFromPendingReqList(id string) akita.Req {
	var reqToRet akita.Req
	newList := make([]akita.Req, 0, len(dma.pendingReqs)-1)
	for _, r := range dma.pendingReqs {
		if r.GetID() == id {
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

func (dma *DMAEngine) parseFromCP(now akita.VTimeInSec) {
	if dma.processingReq != nil {
		return
	}

	req := dma.ToCP.Retrieve(now)
	if req == nil {
		return
	}

	dma.processingReq = req
	dma.NeedTick = true

	switch req := req.(type) {
	case *MemCopyH2DReq:
		dma.parseMemCopyH2D(now, req)
	case *MemCopyD2HReq:
		dma.parseMemCopyD2H(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
}

func (dma *DMAEngine) parseMemCopyH2D(
	now akita.VTimeInSec,
	req *MemCopyH2DReq,
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
		reqToBottom := mem.NewWriteReq(now, dma.ToMem, module, addr)
		reqToBottom.Data = req.SrcBuffer[offset : offset+length]
		dma.toSendToMem = append(dma.toSendToMem, reqToBottom)
		dma.pendingReqs = append(dma.pendingReqs, reqToBottom)

		addr += length
		lengthLeft -= length
		offset += length
	}
}

func (dma *DMAEngine) parseMemCopyD2H(
	now akita.VTimeInSec,
	req *MemCopyD2HReq,
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
		reqToBottom := mem.NewReadReq(now, dma.ToMem, module, addr, length)
		dma.toSendToMem = append(dma.toSendToMem, reqToBottom)
		dma.pendingReqs = append(dma.pendingReqs, reqToBottom)

		addr += length
		lengthLeft -= length
		offset += length
	}
}

// NewDMAEngine creates a DMAEngine, injecting a engine and a "LowModuleFinder"
// that helps with locating the module that holds the data.
func NewDMAEngine(
	name string,
	engine akita.Engine,
	localDataSource cache.LowModuleFinder,
) *DMAEngine {
	dma := new(DMAEngine)
	dma.TickingComponent = akita.NewTickingComponent(name, engine,
		1*akita.GHz, dma)

	dma.Log2AccessSize = 6

	dma.localDataSource = localDataSource

	dma.ToCP = akita.NewLimitNumReqPort(dma, 40960000)
	dma.ToMem = akita.NewLimitNumReqPort(dma, 64)

	return dma
}
