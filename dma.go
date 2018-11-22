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
	*akita.ComponentBase
	ticker *akita.Ticker

	engine          akita.Engine
	localDataSource cache.LowModuleFinder

	Freq akita.Freq

	processingReq  akita.Req
	progressOffset uint64
	needTick       bool

	ToCommandProcessor *akita.Port
	ToMem              *akita.Port
}

func (dma *DMAEngine) NotifyPortFree(now akita.VTimeInSec, port *akita.Port) {
	dma.ticker.TickLater(now)
}

func (dma *DMAEngine) NotifyRecv(now akita.VTimeInSec, port *akita.Port) {
	dma.ticker.TickLater(now)
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
	dma.needTick = false

	req := dma.ToMem.Peek()
	if req != nil {
		switch req := req.(type) {
		case *mem.DoneRsp:
			dma.processDoneRspFromLocalMemory(now, req)
		case *mem.DataReadyRsp:
			dma.processDataReadyRspFromLocalMemory(now, req)
		default:
			log.Panicf("cannot handle request for type %s",
				reflect.TypeOf(req))
		}
	}

	if dma.processingReq != nil {
		switch req := dma.processingReq.(type) {
		case *MemCopyH2DReq:
			return dma.doCopyH2D(now, req)
		case *MemCopyD2HReq:
			return dma.doCopyD2H(now, req)
		default:
			log.Panicf("cannot handle request for type %s in tick event",
				reflect.TypeOf(req))
		}
	}

	dma.acceptNewReq(now)

	if dma.needTick == true {
		dma.ticker.TickLater(now)
	}

	return nil
}

func (dma *DMAEngine) acceptNewReq(now akita.VTimeInSec) {
	if dma.processingReq != nil {
		return
	}
	req := dma.ToCommandProcessor.Retrieve(now)
	if req != nil {
		dma.processingReq = req
		dma.progressOffset = 0
		dma.needTick = true
	}
}

func (dma *DMAEngine) processDoneRspFromLocalMemory(now akita.VTimeInSec, rsp *mem.DoneRsp) {
	dma.needTick = true
	dma.ToMem.Retrieve(now)
}

func (dma *DMAEngine) processDataReadyRspFromLocalMemory(now akita.VTimeInSec, rsp *mem.DataReadyRsp) {
	offset := dma.progressOffset
	length := uint64(len(rsp.Data))
	req := dma.processingReq.(*MemCopyD2HReq)
	copy(req.DstBuffer[offset-length:offset], rsp.Data)
	dma.ToMem.Retrieve(now)

	dma.needTick = true
}

func (dma *DMAEngine) doCopyH2D(now akita.VTimeInSec, req *MemCopyH2DReq) error {
	if dma.memCopyH2DCompleted(req) {
		dma.replyMemCopyH2D(now, req)
		return nil
	}
	dma.writeMemory(now, req)
	return nil
}

func (dma *DMAEngine) writeMemory(now akita.VTimeInSec, req *MemCopyH2DReq) {
	address := req.DstAddress + dma.progressOffset
	nextCacheLineAddress := address&0xffffffffffffffc0 + 64

	length := nextCacheLineAddress - address
	lengthLeft := uint64(len(req.SrcBuffer)) - dma.progressOffset
	if length > lengthLeft {
		length = lengthLeft
	}
	lowModule := dma.localDataSource.Find(address)

	writeReq := mem.NewWriteReq(now, dma.ToMem, lowModule, address)
	writeReq.Data = req.SrcBuffer[dma.progressOffset : dma.progressOffset+length]
	err := dma.ToMem.Send(writeReq)
	if err == nil {
		dma.progressOffset += length
		dma.needTick = true
	}
}

func (dma *DMAEngine) replyMemCopyH2D(now akita.VTimeInSec, req *MemCopyH2DReq) {
	req.SwapSrcAndDst()
	req.SetSendTime(now)
	err := dma.ToCommandProcessor.Send(req)
	if err == nil {
		dma.processingReq = nil
		dma.needTick = true
	}
}

func (dma *DMAEngine) memCopyH2DCompleted(req *MemCopyH2DReq) bool {
	return dma.progressOffset >= uint64(len(req.SrcBuffer))
}

func (dma *DMAEngine) doCopyD2H(now akita.VTimeInSec, req *MemCopyD2HReq) error {
	if dma.memCopyD2HCompleted(req) {
		dma.replyMemCopyD2H(now, req)
		return nil
	}
	dma.readMemory(now, req)
	return nil
}

func (dma *DMAEngine) memCopyD2HCompleted(req *MemCopyD2HReq) bool {
	return dma.progressOffset >= uint64(len(req.DstBuffer))
}

func (dma *DMAEngine) replyMemCopyD2H(now akita.VTimeInSec, req *MemCopyD2HReq) {
	req.SwapSrcAndDst()
	req.SetSendTime(now)
	err := dma.ToCommandProcessor.Send(req)
	if err == nil {
		dma.processingReq = nil
		dma.needTick = true
	}
}

func (dma *DMAEngine) readMemory(now akita.VTimeInSec, req *MemCopyD2HReq) {
	address := req.SrcAddress + dma.progressOffset
	nextCacheLineAddress := address&0xffffffffffffffc0 + 64
	length := nextCacheLineAddress - address
	lengthLeft := uint64(len(req.DstBuffer)) - dma.progressOffset
	if length > lengthLeft {
		length = lengthLeft
	}
	lowModule := dma.localDataSource.Find(address)

	readReq := mem.NewReadReq(now, dma.ToMem, lowModule, address, length)
	err := dma.ToMem.Send(readReq)
	if err == nil {
		dma.progressOffset += length
		dma.needTick = true
	}
}

// NewDMAEngine creates a DMAEngine, injecting a engine and a "LowModuleFinder"
// that helps with locating the module that holds the data.
func NewDMAEngine(
	name string,
	engine akita.Engine,
	localDataSource cache.LowModuleFinder,
) *DMAEngine {
	componentBase := akita.NewComponentBase(name)
	dma := new(DMAEngine)
	dma.ComponentBase = componentBase
	dma.engine = engine
	dma.localDataSource = localDataSource

	dma.Freq = 1 * akita.GHz
	dma.ticker = akita.NewTicker(dma, engine, dma.Freq)

	dma.ToCommandProcessor = akita.NewPort(dma)
	dma.ToMem = akita.NewPort(dma)

	return dma
}
