package gcn3

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
	"gitlab.com/yaotsu/mem/cache"
)

// A DMAEngine is responsible for accessing data that does not belongs to
// the GPU that the DMAEngine works in.
type DMAEngine struct {
	*core.ComponentBase
	ticker *core.Ticker

	engine           core.Engine
	localDataSource  cache.LowModuleFinder
	remoteDataSource cache.LowModuleFinder

	Freq core.Freq

	processingReq  core.Req
	progressOffset uint64
	needTick       bool

	processingRDMAReadReq   []*mem.ReadReq
	pendingReadToAnotherGPU map[string]*mem.ReadReq

	ToCommandProcessor *core.Port
	ToMem              *core.Port
	ToOtherGPUs        *core.Port
}

func (dma *DMAEngine) NotifyPortFree(now core.VTimeInSec, port *core.Port) {
	dma.ticker.TickLater(now)
}

func (dma *DMAEngine) NotifyRecv(now core.VTimeInSec, port *core.Port) {
	dma.ticker.TickLater(now)
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
	dma.needTick = false

	req := dma.ToMem.Peek()
	if req != nil {
		switch req := req.(type) {
		case *mem.DoneRsp:
			dma.processDoneRspFromLocalMemory(now, req)
		case *mem.DataReadyRsp:
			dma.processDataReadyRspFromLocalMemory(now, req)
		case *mem.ReadReq:
			dma.processReadReqFromLocalMemory(now, req)
		default:
			log.Panicf("cannot handle request for type %s",
				reflect.TypeOf(req))
		}
	}

	req = dma.ToOtherGPUs.Peek()
	if req != nil {
		switch req := req.(type) {
		case *mem.DataReadyRsp:
			dma.processDataReadyRspFromAnotherGPU(now, req)
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

func (dma *DMAEngine) acceptNewReq(now core.VTimeInSec) {
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

func (dma *DMAEngine) processReadReqFromLocalMemory(now core.VTimeInSec, req *mem.ReadReq) {
	dst := dma.remoteDataSource.Find(req.Address)
	newReq := mem.NewReadReq(now, dma.ToOtherGPUs, dst, req.Address, req.MemByteSize)
	err := dma.ToOtherGPUs.Send(newReq)
	if err == nil {
		dma.pendingReadToAnotherGPU[newReq.ID] = newReq
		dma.processingRDMAReadReq = append(dma.processingRDMAReadReq, req)
		dma.ToMem.Retrieve(now)
		dma.needTick = true
	}
}

func (dma *DMAEngine) processDataReadyRspFromAnotherGPU(now core.VTimeInSec, dataReady *mem.DataReadyRsp) {
	readReqToOtherGPU := dma.pendingReadToAnotherGPU[dataReady.RespondTo]

	var originalRead, read *mem.ReadReq
	var i int
	for i, read = range dma.processingRDMAReadReq {
		if read.Address == readReqToOtherGPU.Address {
			originalRead = read
			break
		}
	}

	if originalRead == nil {
		log.Panic("cannot find the original read from memory")
	}

	newDataReady := mem.NewDataReadyRsp(now, dma.ToMem, originalRead.Src(),
		originalRead.ID)
	err := dma.ToMem.Send(newDataReady)

	if err == nil {
		delete(dma.pendingReadToAnotherGPU, readReqToOtherGPU.ID)
		dma.processingRDMAReadReq = append(dma.processingRDMAReadReq[:i],
			dma.processingRDMAReadReq[i+1:]...)
		dma.ToOtherGPUs.Retrieve(now)
		dma.needTick = true
	}

}

func (dma *DMAEngine) processDoneRspFromLocalMemory(now core.VTimeInSec, rsp *mem.DoneRsp) {
	dma.needTick = true
	dma.ToMem.Retrieve(now)
}

func (dma *DMAEngine) processDataReadyRspFromLocalMemory(now core.VTimeInSec, rsp *mem.DataReadyRsp) {
	offset := dma.progressOffset
	length := uint64(len(rsp.Data))
	req := dma.processingReq.(*MemCopyD2HReq)
	copy(req.DstBuffer[offset-length:offset], rsp.Data)
	dma.ToMem.Retrieve(now)

	dma.needTick = true
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

	writeReq := mem.NewWriteReq(now, dma.ToMem, lowModule, address)
	writeReq.Data = req.SrcBuffer[dma.progressOffset : dma.progressOffset+length]
	err := dma.ToMem.Send(writeReq)
	if err == nil {
		dma.progressOffset += length
		dma.needTick = true
	}
}

func (dma *DMAEngine) replyMemCopyH2D(now core.VTimeInSec, req *MemCopyH2DReq) {
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

func (dma *DMAEngine) doCopyD2H(now core.VTimeInSec, req *MemCopyD2HReq) error {
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

func (dma *DMAEngine) replyMemCopyD2H(now core.VTimeInSec, req *MemCopyD2HReq) {
	req.SwapSrcAndDst()
	req.SetSendTime(now)
	err := dma.ToCommandProcessor.Send(req)
	if err == nil {
		dma.processingReq = nil
		dma.needTick = true
	}
}

func (dma *DMAEngine) readMemory(now core.VTimeInSec, req *MemCopyD2HReq) {
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
	engine core.Engine,
	localDataSource cache.LowModuleFinder,
	remoteDataSource cache.LowModuleFinder,
) *DMAEngine {
	componentBase := core.NewComponentBase(name)
	dma := new(DMAEngine)
	dma.ComponentBase = componentBase
	dma.engine = engine
	dma.localDataSource = localDataSource
	dma.remoteDataSource = remoteDataSource

	dma.Freq = 1 * core.GHz
	dma.ticker = core.NewTicker(dma, engine, dma.Freq)

	dma.ToCommandProcessor = core.NewPort(dma)
	dma.ToMem = core.NewPort(dma)
	dma.ToOtherGPUs = core.NewPort(dma)

	dma.processingRDMAReadReq = make([]*mem.ReadReq, 0)
	dma.pendingReadToAnotherGPU = make(map[string]*mem.ReadReq, 0)

	return dma
}

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
	src, dst *core.Port,
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
	src, dst *core.Port,
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
