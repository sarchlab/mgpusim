package caches

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/timing/pipelines"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm"
)

type invalidationCompleteEvent struct {
	*akita.EventBase

	req      *mem.InvalidReq
	fromPort akita.Port
}

func newInvalidationCompleteEvent(
	time akita.VTimeInSec,
	handler akita.Handler,
	req *mem.InvalidReq,
	fromPort akita.Port,
) *invalidationCompleteEvent {
	e := new(invalidationCompleteEvent)
	e.EventBase = akita.NewEventBase(time, handler)
	e.req = req
	e.fromPort = fromPort
	return e
}

type FlushL1CacheReq struct {
	*akita.ReqBase
	flushContents bool
}


// NewWriteReq creates a new WriteReq
func NewFlushL1CacheReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
	flushContents bool,
) *FlushL1CacheReq {
	reqBase := akita.NewReqBase()
	req := new(FlushL1CacheReq)
	req.ReqBase = reqBase

	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)

	req.flushContents = flushContents

	return req
}

type L1FlushCompleteRsp struct {
	*akita.ReqBase
	flushComplete bool
}

func NewFlushCompleteRsp(
	time akita.VTimeInSec,
	src, dst akita.Port,
	flushComplete bool,
) *L1FlushCompleteRsp {
	reqBase := akita.NewReqBase()
	req := new(L1FlushCompleteRsp)
	req.ReqBase = reqBase

	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)

	req.flushComplete = flushComplete

	return req
}

type RestartL1CacheReq struct {
	*akita.ReqBase
	restart bool
}

func NewRestartL1CacheReq(
	time akita.VTimeInSec,
	src, dst akita.Port,
	restart bool,
) *RestartL1CacheReq {
	reqBase := akita.NewReqBase()
	req := new(RestartL1CacheReq)
	req.ReqBase = reqBase

	req.SetSendTime(time)
	req.SetSrc(src)
	req.SetDst(dst)

	req.restart = restart

	return req
}


type cacheTransaction struct {
	Req           akita.Req
	Rsp           akita.Req
	Block         *cache.Block
	ReqToBottom   akita.Req
	RspFromBottom akita.Req
	OriginalReqs  []akita.Req
	Page          *vm.Page
}

func (t *cacheTransaction) getPAddr() uint64 {
	vAddr := t.Req.(mem.AccessReq).GetAddress()
	pageVAddr := t.Page.VAddr

	if vAddr < pageVAddr || vAddr >= pageVAddr+t.Page.PageSize {
		panic("vaddr not in page")
	}

	offset := vAddr - pageVAddr
	pagePAddr := t.Page.PAddr
	pAddr := pagePAddr + offset
	return pAddr
}

type inPipelineReqStatus struct {
	Transaction *cacheTransaction
	CycleLeft   int
}

// L1VCache is a tailored write-through cache specific for GCN3 architecture.
type L1VCache struct {
	*akita.TickingComponent

	ToCU  akita.Port
	ToCP  akita.Port
	ToL2  akita.Port
	ToTLB akita.Port

	L2Finder  cache.LowModuleFinder
	Latency   int
	Directory cache.Directory
	Storage   *mem.Storage
	TLB       akita.Port

	addrTranslator subComponent

	BlockSizeAsPowerOf2 uint64
	InvalidationLatency int

	Pipeline        pipelines.Pipeline
	inPipeline      []*inPipelineReqStatus
	postPipelineBuf []*cacheTransaction

	reqBuf                []*cacheTransaction
	reqIDToTransactionMap map[string]*cacheTransaction
	reqBufCapacity        int
	reqBufReadPtr         int

	preCoalesceWriteBuf  []*cacheTransaction
	postCoalesceWriteBuf []*cacheTransaction

	preAddrTranslationBuf  []*cacheTransaction
	postAddrTranslationBuf []*cacheTransaction

	toCUBuffer            []akita.Req
	toL2Buffer            []akita.Req
	pendingDownGoingRead  []*mem.ReadReq
	pendingDownGoingWrite []*cacheTransaction
	mshr                  []*cacheTransaction

	storageTransaction *cacheTransaction
	storageCycleLeft   int

	currentFlushReq *FlushL1CacheReq
	flushLatency    uint64
	flushCycleLeft  uint64

	toSendToCP akita.Req

	isFlushing bool

	gpuID uint64
}

// Handle processes the events scheduled on L1VCache
func (c *L1VCache) Handle(e akita.Event) error {
	switch e := e.(type) {
	case akita.TickEvent:
		c.handleTickEvent(e)
	case *invalidationCompleteEvent:
		c.handleInvalidationCompleteEvent(e)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(e))
	}
	return nil
}

func (c *L1VCache) handleTickEvent(e akita.TickEvent) {
	now := e.Time()
	c.NeedTick = false


	c.parseFromCP(now)

	if !c.isFlushing {

		c.parseFromL2(now)
		c.parseFromCU(now)
		c.sendToCU(now)
		c.sendToL2(now)
		c.sendToCP(now)
		c.doReadWrite(now)
		c.parseFromPostPipelineBuf(now)
		c.countDownPipeline(now)
		c.processPostCoalescingWrites(now)
		c.processFlushReq(now)
		c.parseFromPostAddrTranslationBuf(now)
		c.NeedTick = c.addrTranslator.tick(now) || c.NeedTick
	}

	if c.NeedTick {
		c.TickLater(now)
	}
}

func (c *L1VCache) parseFromCP(now akita.VTimeInSec) {
	if len(c.reqBuf) > 0 {
		return
	}

	req := c.ToCP.Retrieve(now)
	if req == nil {
		return
	}

	switch req := req.(type) {
	case *mem.InvalidReq:
		c.doInvalidation(now, req)
	case *FlushL1CacheReq:
		c.addFlushToPipeline(now, req)
	case *RestartL1CacheReq:
		c.restartL1Cache(now)
	default:
		log.Panicf("cannot handle request of type %s from CP", reflect.TypeOf(req))
	}

	c.NeedTick = true
}

func (c *L1VCache) doInvalidation(now akita.VTimeInSec, req *mem.InvalidReq) {
	completeTime := c.Freq.NCyclesLater(c.InvalidationLatency, now)
	invalidComplete := newInvalidationCompleteEvent(
		completeTime, c, req, c.ToCP)
	c.Engine.Schedule(invalidComplete)
}

func (c *L1VCache) addFlushToPipeline(now akita.VTimeInSec, req *FlushL1CacheReq) {
	c.currentFlushReq = req
	c.flushCycleLeft = c.flushLatency
}

func (c *L1VCache) processFlushReq(now akita.VTimeInSec) {
	if c.currentFlushReq == nil {
		return
	}

	if c.flushCycleLeft <= 0 {
		c.doFlush(now, c.currentFlushReq)
		c.currentFlushReq = nil
	}

	c.flushCycleLeft--
	c.NeedTick = true

}

func (c *L1VCache) doFlush(now akita.VTimeInSec, req *FlushL1CacheReq) {

	c.inPipeline = nil
	c.postPipelineBuf = nil
	c.reqBuf = nil
	c.reqIDToTransactionMap = nil
	c.preCoalesceWriteBuf = nil
	c.postCoalesceWriteBuf = nil
	c.preAddrTranslationBuf = nil
	c.postAddrTranslationBuf = nil
	c.toCUBuffer = nil
	c.toL2Buffer = nil
	c.pendingDownGoingRead = nil
	c.pendingDownGoingWrite = nil
	c.mshr = nil
	c.storageTransaction = nil

	rsp := NewFlushCompleteRsp(now, c.ToCP, req.Src(), true)
	c.toSendToCP = rsp

	c.isFlushing = true

	c.NeedTick = true

}

func (c *L1VCache) restartL1Cache(now akita.VTimeInSec) {
	c.isFlushing = false
	c.NeedTick = true
}

func (c *L1VCache) sendToCP(now akita.VTimeInSec) {
	if c.toSendToCP == nil {
		return
	}

	rsp := c.toSendToCP
	err := c.ToCP.Send(rsp)

	if err == nil {
		c.toSendToCP = nil
		c.NeedTick = true
	}

}

func (c *L1VCache) handleInvalidationCompleteEvent(
	evt *invalidationCompleteEvent,
) {
	now := evt.Time()
	req := evt.req

	rsp := mem.NewInvalidDoneRsp(now, evt.fromPort, req.Src(), req.GetID())
	err := c.ToCP.Send(rsp)
	if err == nil {
		c.Directory.Reset()
		c.TickLater(now)
	} else {
		time := c.Freq.NextTick(now)
		evt := newInvalidationCompleteEvent(time, c, req, evt.fromPort)
		c.Engine.Schedule(evt)
	}
}

func (c *L1VCache) parseFromCU(now akita.VTimeInSec) {
	if len(c.reqBuf) >= c.reqBufCapacity {
		return
	}

	req := c.ToCU.Retrieve(now)
	if req == nil {
		return
	}

	transaction := c.createTransaction(req)
	c.NeedTick = true

	c.traceMem(req, now, "translation",
		req.(mem.AccessReq).GetAddress(),
		req.(mem.AccessReq).GetByteSize(),
		nil)

	c.preAddrTranslationBuf = append(c.preAddrTranslationBuf, transaction)
}

func (c *L1VCache) coalesceWrites(now akita.VTimeInSec) {
	var cWrite *mem.WriteReq
	var cWriteTrans *cacheTransaction

	for i, trans := range c.preCoalesceWriteBuf {
		write := trans.Req.(*mem.WriteReq)
		if i == 0 {
			cWrite = mem.NewWriteReq(now, c.ToL2,
				c.L2Finder.Find(trans.getPAddr()),
				trans.getPAddr())
			cWrite.Data = write.Data
			cWriteTrans = new(cacheTransaction)
			cWriteTrans.Req = cWrite
			cWriteTrans.OriginalReqs = append(cWriteTrans.OriginalReqs, write)
			continue
		}

		writeCacheLineID, _ := cache.GetCacheLineID(trans.getPAddr(),
			c.BlockSizeAsPowerOf2)
		cWriteCacheLineID, _ := cache.GetCacheLineID(cWrite.Address,
			c.BlockSizeAsPowerOf2)
		if trans.getPAddr() == cWrite.Address+uint64(len(cWrite.Data)) &&
			writeCacheLineID == cWriteCacheLineID {
			cWrite.Data = append(cWrite.Data, write.Data...)
			cWriteTrans.OriginalReqs = append(cWriteTrans.OriginalReqs, write)
			continue
		}

		// When a write is not coalescable
		c.postCoalesceWriteBuf = append(c.postCoalesceWriteBuf, cWriteTrans)

		cWrite = mem.NewWriteReq(now, c.ToL2,
			c.L2Finder.Find(trans.getPAddr()),
			trans.getPAddr())
		cWrite.Data = write.Data
		cWriteTrans = new(cacheTransaction)
		cWriteTrans.Req = cWrite
		cWriteTrans.OriginalReqs = append(cWriteTrans.OriginalReqs, write)
	}

	if cWrite != nil {
		c.postCoalesceWriteBuf = append(c.postCoalesceWriteBuf, cWriteTrans)
	}
	c.preCoalesceWriteBuf = nil
}

func (c *L1VCache) parseFromPostAddrTranslationBuf(now akita.VTimeInSec) {
	if len(c.postAddrTranslationBuf) == 0 {
		return
	}

	trans := c.postAddrTranslationBuf[0]
	req := trans.Req

	switch req := req.(type) {
	case *mem.ReadReq:
		pipelineStatus := &inPipelineReqStatus{
			CycleLeft:   c.Pipeline.Accept(now, trans),
			Transaction: trans,
		}
		c.inPipeline = append(c.inPipeline, pipelineStatus)
		c.traceMem(req, now, "done-translation",
			req.GetAddress(), req.GetByteSize(), nil)

	case *mem.WriteReq:
		c.preCoalesceWriteBuf = append(c.preCoalesceWriteBuf, trans)
		if req.IsLastInWave {
			c.coalesceWrites(now)
		}
		c.traceMem(req, now, "done-translation",
			req.GetAddress(), req.GetByteSize(), req.Data)

	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}

	c.postAddrTranslationBuf = c.postAddrTranslationBuf[1:]
	c.NeedTick = true
}

func (c *L1VCache) processPostCoalescingWrites(now akita.VTimeInSec) {
	if len(c.postCoalesceWriteBuf) == 0 {
		return
	}

	trans := c.postCoalesceWriteBuf[0]
	pipeStatus := new(inPipelineReqStatus)
	pipeStatus.CycleLeft = c.Pipeline.Accept(now, trans)
	pipeStatus.Transaction = trans
	c.inPipeline = append(c.inPipeline, pipeStatus)

	c.postCoalesceWriteBuf = c.postCoalesceWriteBuf[1:]
	c.NeedTick = true
}

func (c *L1VCache) countDownPipeline(now akita.VTimeInSec) {
	newInPipeline := make([]*inPipelineReqStatus, 0)
	for _, inPipeline := range c.inPipeline {
		inPipeline.CycleLeft--
		c.NeedTick = true

		if inPipeline.CycleLeft > 0 {
			newInPipeline = append(newInPipeline, inPipeline)
		} else {
			c.postPipelineBuf = append(c.postPipelineBuf, inPipeline.Transaction)
			req := inPipeline.Transaction.Req.(mem.AccessReq)
			c.traceMem(req, now, "exit-pipeline",
				req.GetAddress(), req.GetByteSize(), nil)

		}
	}

	c.inPipeline = newInPipeline
}

func (c *L1VCache) parseFromPostPipelineBuf(now akita.VTimeInSec) {
	if len(c.postPipelineBuf) == 0 {
		return
	}

	transaction := c.postPipelineBuf[0]
	req := transaction.Req

	switch req := req.(type) {
	case *mem.ReadReq:
		c.handleReadReq(now, transaction)
	case *mem.WriteReq:
		c.handleWriteReq(now, transaction)
	default:
		log.Panicf("cannot process request of type %s",
			reflect.TypeOf(req))
	}
}

func (c *L1VCache) handleReadReq(
	now akita.VTimeInSec,
	transaction *cacheTransaction,
) {
	req := transaction.Req.(*mem.ReadReq)
	address := transaction.getPAddr()
	cacheLineID, _ := cache.GetCacheLineID(address, c.BlockSizeAsPowerOf2)

	if c.isInMSHR(cacheLineID) {
		c.insertIntoMSHR(transaction)
		c.postPipelineBuf = c.postPipelineBuf[1:]
		c.NeedTick = true
		return
	}

	block := c.Directory.Lookup(cacheLineID)

	if block == nil {
		c.handleReadMiss(now, transaction)
	} else {
		c.handleReadHit(now, req, block)
	}
}

func (c *L1VCache) isInMSHR(cacheLineID uint64) bool {
	for _, entry := range c.mshr {
		if entry.ReqToBottom != nil &&
			entry.ReqToBottom.(*mem.ReadReq).Address == cacheLineID {
			return true
		}
	}
	return false
}

func (c *L1VCache) insertIntoMSHR(transaction *cacheTransaction) {
	c.mshr = append(c.mshr, transaction)
}

func (c *L1VCache) handleReadMiss(now akita.VTimeInSec, trans *cacheTransaction) {
	req := trans.Req.(*mem.ReadReq)
	address := trans.getPAddr()
	cacheLineID, _ := cache.GetCacheLineID(address, c.BlockSizeAsPowerOf2)

	block := c.Directory.Evict(cacheLineID)
	if block.IsLocked {
		return
	}

	l2 := c.L2Finder.Find(cacheLineID)
	readBottom := mem.NewReadReq(now, c.ToL2, l2,
		cacheLineID, 1<<c.BlockSizeAsPowerOf2)
	c.pendingDownGoingRead = append(c.pendingDownGoingRead, readBottom)
	c.toL2Buffer = append(c.toL2Buffer, readBottom)

	transaction := c.reqIDToTransactionMap[req.GetID()]
	transaction.ReqToBottom = readBottom
	transaction.Block = block
	block.IsValid = true
	block.IsLocked = true
	block.Tag = cacheLineID
	c.insertIntoMSHR(transaction)

	c.postPipelineBuf = c.postPipelineBuf[1:]
	c.NeedTick = true

	c.traceMem(req, now, "read-miss", address, req.MemByteSize, nil)
}

func (c *L1VCache) handleReadHit(
	now akita.VTimeInSec,
	req *mem.ReadReq,
	block *cache.Block,
) {
	if block.IsLocked {
		return
	}

	c.postPipelineBuf = c.postPipelineBuf[1:]
	c.NeedTick = true

	//cycleLeft := c.Pipeline.Accept(now, req)
	//c.inPipeline = append(c.inPipeline,
	//	&inPipelineReqStatus{req, cycleLeft})

	transaction := c.reqIDToTransactionMap[req.GetID()]
	transaction.Block = block
	block.IsLocked = true

	c.storageCycleLeft = c.Latency
	c.storageTransaction = transaction

	c.traceMem(req, now, "read-hit", req.Address, req.MemByteSize, nil)
}

func (c *L1VCache) handleWriteReq(now akita.VTimeInSec, transaction *cacheTransaction) {
	req := transaction.Req.(*mem.WriteReq)
	cacheLineID, offset := cache.GetCacheLineID(req.Address, c.BlockSizeAsPowerOf2)
	block := c.Directory.Lookup(cacheLineID)
	if block == nil && len(req.Data) == 1<<c.BlockSizeAsPowerOf2 {
		// Write allocate
		block = c.Directory.Evict(cacheLineID)
		block.IsValid = true
		block.Tag = req.Address
	}

	if block != nil && block.IsLocked {
		return
	}

	if block != nil {
		block.IsLocked = true
	}

	transaction.Block = block

	c.writeToLowModule(now, transaction)
	if block != nil {
		c.doWriteLine(now, req, block, offset)
	}

	c.postPipelineBuf = c.postPipelineBuf[1:]
	c.NeedTick = true

	c.traceMem(req, now, "write", req.Address, uint64(len(req.Data)),
		req.Data)
}

func (c *L1VCache) writeToLowModule(
	now akita.VTimeInSec,
	transaction *cacheTransaction,
) {
	req := transaction.Req.(*mem.WriteReq)
	l2 := c.L2Finder.Find(req.Address)
	writeBottom := mem.NewWriteReq(now, c.ToL2, l2, req.Address)
	writeBottom.Data = req.Data

	c.toL2Buffer = append(c.toL2Buffer, writeBottom)
	c.pendingDownGoingWrite = append(c.pendingDownGoingWrite, transaction)

	transaction.ReqToBottom = writeBottom
}

func (c *L1VCache) doWriteLine(
	now akita.VTimeInSec,
	req *mem.WriteReq,
	block *cache.Block,
	offset uint64,
) {
	err := c.Storage.Write(block.CacheAddress+offset, req.Data)
	if err != nil {
		log.Panic(err)
	}
}

func (c *L1VCache) parseFromL2(now akita.VTimeInSec) {
	req := c.ToL2.Peek()

	if req == nil {
		return
	}

	switch req := req.(type) {
	case *mem.DataReadyRsp:
		c.handleDataReadyRsp(now, req)
	case *mem.DoneRsp:
		c.handleDoneRsp(now, req)
	default:
		log.Panicf("cannot process request of type %s",
			reflect.TypeOf(req))
	}
}

func (c *L1VCache) handleDataReadyRsp(now akita.VTimeInSec, dataReady *mem.DataReadyRsp) {
	//readBottom := c.pendingDownGoingRead[0]
	//address := readBottom.Address

	transaction := c.getTransactionByReqToBottomID(dataReady.RespondTo)
	if transaction == nil {
		log.Panic("transaction not found")
	}

	address := transaction.ReqToBottom.(*mem.ReadReq).Address
	block := transaction.Block
	block.IsValid = true
	block.IsDirty = false
	block.IsLocked = false
	block.Tag = address
	err := c.Storage.Write(block.CacheAddress, dataReady.Data)
	if err != nil {
		log.Panic(err)
	}

	for _, mshrEntry := range c.mshr {
		readFromTop, ok := mshrEntry.Req.(*mem.ReadReq)
		if !ok {
			continue
		}

		cacheLineID, offset := cache.GetCacheLineID(
			mshrEntry.getPAddr(), c.BlockSizeAsPowerOf2)
		if cacheLineID != address {
			continue
		}

		dataReadyToTop := mem.NewDataReadyRsp(
			now, c.ToCU, readFromTop.Src(), readFromTop.GetID())
		dataReadyToTop.Data =
			dataReady.Data[offset : offset+readFromTop.MemByteSize]
		mshrEntry.Rsp = dataReadyToTop

		c.traceMem(readFromTop, now, "data-ready",
			address, uint64(len(dataReady.Data)), dataReady.Data)
	}

	newMSHR := make([]*cacheTransaction, 0)
	for _, mshrEntry := range c.mshr {
		_, ok := mshrEntry.Req.(*mem.ReadReq)
		if !ok {
			continue
		}

		cacheLineID, _ := cache.GetCacheLineID(
			mshrEntry.getPAddr(), c.BlockSizeAsPowerOf2)
		if cacheLineID != address {
			newMSHR = append(newMSHR, mshrEntry)
		}
	}
	c.mshr = newMSHR

	c.ToL2.Retrieve(now)
	c.pendingDownGoingRead = c.pendingDownGoingRead[1:]
	c.NeedTick = true

}

func (c *L1VCache) handleDoneRsp(now akita.VTimeInSec, rsp *mem.DoneRsp) {
	var transaction *cacheTransaction
	for i, trans := range c.pendingDownGoingWrite {
		if trans.ReqToBottom.GetID() == rsp.RespondTo {
			transaction = trans
			c.pendingDownGoingWrite = append(
				c.pendingDownGoingWrite[:i], c.pendingDownGoingWrite[i+1:]...)
		}
	}

	if transaction == nil {
		log.Panic("transaction not found")
	}

	for _, originalReq := range transaction.OriginalReqs {
		originalTrans := c.reqIDToTransactionMap[originalReq.GetID()]
		write := originalTrans.Req.(*mem.WriteReq)
		done := mem.NewDoneRsp(now, c.ToCU, write.Src(), write.GetID())
		originalTrans.Rsp = done

		c.traceMem(transaction.Req, now, "write-done",
			write.Address, uint64(len(write.Data)), write.Data)
	}

	if transaction.Block != nil {
		transaction.Block.IsLocked = false
	}

	c.ToL2.Retrieve(now)

	c.NeedTick = true
}

func (c *L1VCache) doReadWrite(now akita.VTimeInSec) {
	if c.storageTransaction == nil {
		return
	}

	c.storageCycleLeft--
	c.NeedTick = true

	if c.storageCycleLeft <= 0 {
		switch req := c.storageTransaction.Req.(type) {
		case *mem.ReadReq:
			c.finishLocalRead(now, req)
		default:
			log.Panicf("cannot handle request of type %s",
				reflect.TypeOf(req))
		}
		c.storageTransaction = nil
	}
}

func (c *L1VCache) finishLocalRead(now akita.VTimeInSec, read *mem.ReadReq) {
	transaction := c.reqIDToTransactionMap[read.GetID()]
	block := transaction.Block

	_, offset := cache.GetCacheLineID(read.Address, c.BlockSizeAsPowerOf2)
	data, err := c.Storage.Read(
		block.CacheAddress+offset, read.MemByteSize)
	if err != nil {
		log.Panic(err)
	}

	dataReady := mem.NewDataReadyRsp(now, c.ToCU, read.Src(), read.ID)
	dataReady.Data = data

	transaction.Rsp = dataReady
	block.IsLocked = false

	c.traceMem(transaction.Req, now, "local_read_done", read.Address,
		uint64(len(dataReady.Data)), data)
}

func (c *L1VCache) sendToCU(now akita.VTimeInSec) {
	if len(c.reqBuf) == 0 {
		return
	}

	transaction := c.reqBuf[0]
	if transaction.Rsp == nil {
		return
	}

	rsp := transaction.Rsp
	rsp.SetSendTime(now)
	err := c.ToCU.Send(rsp)
	if err == nil {
		c.removeTransaction(transaction)
		c.NeedTick = true

		switch req := transaction.Req.(type) {
		case *mem.ReadReq:
			c.traceMem(req, now, "fulfill", req.Address,
				uint64(req.MemByteSize), rsp.(*mem.DataReadyRsp).Data)
		case *mem.WriteReq:
			c.traceMem(req, now, "fulfill", req.Address,
				uint64(len(req.Data)), req.Data)
		}

	}
}

func (c *L1VCache) sendToL2(now akita.VTimeInSec) {
	if len(c.toL2Buffer) > 0 {
		req := c.toL2Buffer[0]
		req.SetSendTime(now)
		err := c.ToL2.Send(req)
		if err == nil {
			c.toL2Buffer = c.toL2Buffer[1:]
			c.NeedTick = true
		}
	}
}

func (c *L1VCache) traceMem(
	req akita.Req,
	time akita.VTimeInSec,
	what string,
	address, byteSize uint64,
	data []byte,
) {
	traceInfo := new(mem.TraceInfo)
	traceInfo.Where = c.Name()
	traceInfo.When = time
	traceInfo.What = what
	traceInfo.Address = address
	traceInfo.ByteSize = byteSize
	traceInfo.Data = data
	//fmt.Printf("%.15f,%s,%s,%x,%d\n", time, c.Name(), what, address, byteSize)
	c.InvokeHook(nil, c, akita.AnyHookPos, traceInfo)
}

func (c *L1VCache) createTransaction(req akita.Req) *cacheTransaction {
	if _, found := c.reqIDToTransactionMap[req.GetID()]; found {
		log.Panic("request already recorded as transaction.")
	}

	transaction := new(cacheTransaction)
	transaction.Req = req

	c.reqBuf = append(c.reqBuf, transaction)
	c.reqIDToTransactionMap[req.GetID()] = transaction

	//fmt.Printf("create transaction %s\n", req.GetID())

	return transaction
}

func (c *L1VCache) removeTransaction(transaction *cacheTransaction) {
	if transaction != c.reqBuf[0] {
		log.Panic("can only remove from the head of the buf")
	}

	c.reqBuf = c.reqBuf[1:]
	c.reqBufReadPtr--
	delete(c.reqIDToTransactionMap, transaction.Req.GetID())

	//fmt.Printf("remove transaction %s\n", transaction.Req.GetID())
}

func (c *L1VCache) getTransactionByReqToBottomID(id string) *cacheTransaction {
	for _, transaction := range c.reqBuf {
		if transaction.ReqToBottom != nil &&
			transaction.ReqToBottom.GetID() == id {
			return transaction
		}
	}
	return nil
}

// NewL1VCache creates a new L1VCache
func NewL1VCache(name string, engine akita.Engine, freq akita.Freq) *L1VCache {
	c := new(L1VCache)
	c.TickingComponent = akita.NewTickingComponent(name, engine, freq, c)

	c.Latency = 1

	c.reqBufCapacity = 256
	c.reqIDToTransactionMap = make(map[string]*cacheTransaction)

	c.Pipeline = pipelines.NewPipeline()
	c.Pipeline.SetStageLatency(2)
	c.Pipeline.SetNumStages(50)
	c.Pipeline.SetFrequency(freq)
	c.Pipeline.SetNumLines(4)

	c.ToCU = akita.NewLimitNumReqPort(c, 4)
	c.ToCP = akita.NewLimitNumReqPort(c, 4)
	c.ToL2 = akita.NewLimitNumReqPort(c, 4)
	c.ToTLB = akita.NewLimitNumReqPort(c, 4)
	return c
}

// BuildL1VCache configures an L1VCache with specified parameters.
func BuildL1VCache(
	name string,
	engine akita.Engine, freq akita.Freq,
	latency int,
	blockSizeAsPowerOf2, way, sizeAsPowerOf2 uint64,
	l2Finder cache.LowModuleFinder,
	TLB akita.Port,
	gpuID uint64,
) *L1VCache {
	c := NewL1VCache(name, engine, freq)

	blockSize := uint64(1 << blockSizeAsPowerOf2)
	totalSize := uint64(1 << sizeAsPowerOf2)

	lruEvictor := cache.NewLRUEvictor()
	directory := cache.NewDirectory(
		int(totalSize/way/blockSize),
		int(way), int(blockSize), lruEvictor)
	storage := mem.NewStorage(totalSize)

	addrTranslator := &addressTranslator{}
	addrTranslator.l1vCache = c
	c.addrTranslator = addrTranslator

	c.Directory = directory
	c.Storage = storage
	c.L2Finder = l2Finder
	c.TLB = TLB

	c.Latency = latency

	c.BlockSizeAsPowerOf2 = blockSizeAsPowerOf2
	c.InvalidationLatency = int(totalSize / way / blockSize)
	c.gpuID = gpuID

	return c
}
