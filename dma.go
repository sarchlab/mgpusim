package gcn3

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/mem"
	"gitlab.com/yaotsu/mem/cache"
)

// A MemCopyH2DReq is a request that asks the DMAEngine to copy memory
// from the host to the device
type MemCopyH2DReq struct {
	*core.ReqBase
	SrcBuffer  []byte
	DstAddress uint64
}

// NewMemCopyH2DReq created a new MemCopyH2DReq
func NewMemCopyH2DReq(
	time core.VTimeInSec,
	src, dst core.Component,
	srcBuffer []byte,
	dstAddress uint64,
) *MemCopyH2DReq {
	reqBase := core.NewReqBase()
	req := new(MemCopyH2DReq)
	req.ReqBase = reqBase
	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)
	req.SrcBuffer = srcBuffer
	req.DstAddress = dstAddress
	return req
}

// A MemCopyD2HReq is a request that asks the DMAEngine to copy memory
// from the host to the device
type MemCopyD2HReq struct {
	*core.ReqBase
	SrcAddress uint64
	DstBuffer  []byte
}

// NewMemCopyD2HReq created a new MemCopyH2DReq
func NewMemCopyD2HReq(
	time core.VTimeInSec,
	src, dst core.Component,
	srcAddress uint64,
	dstBuffer []byte,
) *MemCopyD2HReq {
	reqBase := core.NewReqBase()
	req := new(MemCopyD2HReq)
	req.ReqBase = reqBase
	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)
	req.SrcAddress = srcAddress
	req.DstBuffer = dstBuffer
	return req
}

// A DMAEngine is responsible for accessing data that does not belongs to
// the GPU that the DMAEngine works in.
type DMAEngine struct {
	*core.ComponentBase

	engine          core.Engine
	localDataSource cache.LowModuleFinder

	Freq util.Freq

	processingReq  core.Req
	progressOffset uint64
	tickEvent      *core.TickEvent
}

func (dma *DMAEngine) Recv(req core.Req) *core.Error {
	switch req := req.(type) {
	case *MemCopyH2DReq:
		return dma.processMemCopyH2DReq(req)
	case *mem.DoneRsp:
		return dma.processDoneRsp(req)
	default:
		log.Panicf("cannot process request for type %s", reflect.TypeOf(req))
	}
	return nil
}

func (dma *DMAEngine) processMemCopyH2DReq(req *MemCopyH2DReq) *core.Error {
	now := req.RecvTime()

	if dma.processingReq != nil {
		return core.NewError("Busy", true, dma.Freq.NextTick(now))
	}

	dma.progressOffset = 0
	dma.processingReq = req
	dma.tickLater(now)
	return nil
}

func (dma *DMAEngine) processDoneRsp(rsp *mem.DoneRsp) *core.Error {
	now := rsp.RecvTime()
	dma.tickLater(dma.Freq.NextTick(now))
	return nil
}

func (dma *DMAEngine) Handle(evt core.Event) error {
	switch evt := evt.(type) {
	case *core.TickEvent:
		return dma.tick(evt)
	default:
		log.Panicf("cannot handle event for type %s", reflect.TypeOf(evt))
	}
	return nil
}

func (dma *DMAEngine) tick(evt *core.TickEvent) error {
	now := evt.Time()
	switch req := dma.processingReq.(type) {
	case *MemCopyH2DReq:
		return dma.doCopyH2D(now, req)
	}
	return nil
}

func (dma *DMAEngine) doCopyH2D(now core.VTimeInSec, req *MemCopyH2DReq) error {
	if dma.memCopyH2DCompleted(req) {
		dma.replyMemCopyH2D(now, req)
		return nil
	}
	dma.writeMemory(now, req)
	return nil
}

func (dma *DMAEngine) writeMemory(now core.VTimeInSec, req *MemCopyH2DReq) {
	address := req.DstAddress + dma.progressOffset
	nextCacheLineAddress := address&0xffffffffffffffc0 + 64
	length := nextCacheLineAddress - address
	lengthLeft := uint64(len(req.SrcBuffer)) - dma.progressOffset
	if length > lengthLeft {
		length = lengthLeft
	}
	lowModule := dma.localDataSource.Find(address)

	writeReq := mem.NewWriteReq(now, dma, lowModule, address)
	writeReq.Data = req.SrcBuffer[dma.progressOffset : dma.progressOffset+length]
	err := dma.GetConnection("ToMem").Send(writeReq)
	if err == nil {
		dma.progressOffset += length
	} else {
		dma.tickLater(err.EarliestRetry)
	}
}

func (dma *DMAEngine) replyMemCopyH2D(now core.VTimeInSec, req *MemCopyH2DReq) {
	req.SwapSrcAndDst()
	req.SetSendTime(now)
	dma.GetConnection("ToCommandProcessor").Send(req)
}

func (dma *DMAEngine) memCopyH2DCompleted(req *MemCopyH2DReq) bool {
	return dma.progressOffset >= uint64(len(req.SrcBuffer))
}

func (dma *DMAEngine) tickLater(time core.VTimeInSec) {
	if time > dma.tickEvent.Time() {
		dma.tickEvent.SetTime(dma.Freq.ThisTick(time))
		dma.engine.Schedule(dma.tickEvent)
	}
}

// NewDMAEngine creates a DMAEngine, injecting a engine and a "LowModuleFinder"
// that helps with locating the module that holds the data.
func NewDMAEngine(
	name string,
	engine core.Engine,
	localDataSource cache.LowModuleFinder,
) *DMAEngine {
	componentBase := core.NewComponentBase(name)
	dma := new(DMAEngine)
	dma.ComponentBase = componentBase
	dma.engine = engine
	dma.localDataSource = localDataSource

	dma.Freq = 1 * util.GHz

	dma.tickEvent = core.NewTickEvent(-1, dma)

	dma.AddPort("ToCommandProcessor")
	dma.AddPort("ToMem")

	return dma
}
