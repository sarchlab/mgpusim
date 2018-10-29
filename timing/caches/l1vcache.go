package caches

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/akita/gcn3/timing/pipelines"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

type invalidationCompleteEvent struct {
	*akita.EventBase

	req      *mem.InvalidReq
	fromPort *akita.Port
}

func newInvalidationCompleteEvent(
	time akita.VTimeInSec,
	handler akita.Handler,
	req *mem.InvalidReq,
	fromPort *akita.Port,
) *invalidationCompleteEvent {
	e := new(invalidationCompleteEvent)
	e.EventBase = akita.NewEventBase(time, handler)
	e.req = req
	e.fromPort = fromPort
	return e
}

type cacheTransaction struct {
	Req           akita.Req
	Rsp           akita.Req
	Block         *cache.Block
	ReqToBottom   akita.Req
	RspFromBottom akita.Req
}

type inPipelineReqStatus struct {
	Req       akita.Req
	CycleLeft int
}

// L1VCache is a tailored write-through cache specific for GCN3 architecture.
type L1VCache struct {
	*akita.TickingComponent

	ToCU *akita.Port
	ToCP *akita.Port
	ToL2 *akita.Port

	L2Finder cache.LowModuleFinder

	Directory cache.Directory
	Storage   *mem.Storage

	BlockSizeAsPowerOf2 uint64
	Latency             int
	InvalidationLatency int

	isStorageBusy bool
	pipeline      pipelines.Pipeline
	inPipeline    []*inPipelineReqStatus

	reqBuf                []*cacheTransaction
	reqIDToTransactionMap map[string]*cacheTransaction
	reqBufCapacity        int
	reqBufReadPtr         int

	toCUBuffer            []akita.Req
	toL2Buffer            []akita.Req
	pendingDownGoingRead  []*mem.ReadReq
	pendingDownGoingWrite []*mem.WriteReq
}

// Handle processes the events scheduled on L1VCache
func (c *L1VCache) Handle(e akita.Event) error {
	switch e := e.(type) {
	case *akita.TickEvent:
		c.handleTickEvent(e)
	case *invalidationCompleteEvent:
		c.handleInvalidationCompleteEvent(e)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(e))
	}
	return nil
}

func (c *L1VCache) createTransaction(req akita.Req) *cacheTransaction {
	if _, found := c.reqIDToTransactionMap[req.GetID()]; found {
		log.Panic("request already recorded as transaction.")
	}

	transaction := new(cacheTransaction)
	transaction.Req = req

	c.reqBuf = append(c.reqBuf, transaction)
	c.reqIDToTransactionMap[req.GetID()] = transaction

	fmt.Printf("Enqueue transaction %s\n", req.GetID())

	return transaction
}

func (c *L1VCache) removeTransaction(transaction *cacheTransaction) {
	if transaction != c.reqBuf[0] {
		log.Panic("can only remove from the head of the buf")
	}

	c.reqBuf = c.reqBuf[1:]
	c.reqBufReadPtr--
	delete(c.reqIDToTransactionMap, transaction.Req.GetID())

	fmt.Printf("Dequeue transaction %s\n", transaction.Req.GetID())
}

func (c *L1VCache) handleTickEvent(e *akita.TickEvent) {
	now := e.Time()
	c.NeedTick = false

	c.sendToCU(now)
	c.sendToL2(now)
	c.doReadWrite(now)
	c.parseFromReqBuf(now)
	c.parseFromCP(now)
	c.parseFromL2(now)
	c.parseFromCU(now)

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
	default:
		log.Panicf("cannot handle request of type %s from CP", reflect.TypeOf(req))
	}
}

func (c *L1VCache) doInvalidation(now akita.VTimeInSec, req *mem.InvalidReq) {
	completeTime := c.Freq.NCyclesLater(c.InvalidationLatency, now)
	invalidComplete := newInvalidationCompleteEvent(
		completeTime, c, req, c.ToCP)
	c.Engine.Schedule(invalidComplete)
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

	c.createTransaction(req)
	c.NeedTick = true

	switch req := req.(type) {
	case *mem.ReadReq:
		c.traceMem(req, now, "parse", req.Address,
			uint64(req.MemByteSize), nil)
	case *mem.WriteReq:
		c.traceMem(req, now, "parse", req.Address,
			uint64(len(req.Data)), req.Data)
	}
}

func (c *L1VCache) parseFromReqBuf(now akita.VTimeInSec) {
	if c.reqBufReadPtr >= len(c.reqBuf) {
		return
	}

	transaction := c.reqBuf[c.reqBufReadPtr]
	req := transaction.Req
	c.reqBufReadPtr++
	c.NeedTick = true

	switch req := req.(type) {
	case *mem.ReadReq:
		c.handleReadReq(now, req)
	case *mem.WriteReq:
		c.handleWriteReq(now, req)
	default:
		log.Panicf("cannot process request of type %s",
			reflect.TypeOf(req))
	}
}

func (c *L1VCache) handleReadReq(now akita.VTimeInSec, req *mem.ReadReq) {
	cacheLineID, _ := cache.GetCacheLineID(req.Address, c.BlockSizeAsPowerOf2)
	block := c.Directory.Lookup(cacheLineID)
	if block == nil {
		c.handleReadMiss(now, req)
	} else {
		c.handleReadHit(now, req, block)
	}
}

func (c *L1VCache) handleReadMiss(now akita.VTimeInSec, req *mem.ReadReq) {
	address := req.Address
	cacheLineID, _ := cache.GetCacheLineID(address, c.BlockSizeAsPowerOf2)

	inMSHR := false
	for _, readToBottom := range c.pendingDownGoingRead {
		if readToBottom.Address == cacheLineID {
			inMSHR = true
		}
	}

	if !inMSHR {
		l2 := c.L2Finder.Find(cacheLineID)
		readBottom := mem.NewReadReq(now, c.ToL2, l2, cacheLineID, 1<<c.BlockSizeAsPowerOf2)
		c.pendingDownGoingRead = append(c.pendingDownGoingRead, readBottom)
		c.toL2Buffer = append(c.toL2Buffer, readBottom)
	}

	c.traceMem(req, now, "read-miss", address, req.MemByteSize, nil)
}

func (c *L1VCache) handleReadHit(now akita.VTimeInSec, req *mem.ReadReq, block *cache.Block) {
	cycleLeft := c.pipeline.Accept(now, req)
	c.inPipeline = append(c.inPipeline,
		&inPipelineReqStatus{req, cycleLeft})

	transaction := c.reqIDToTransactionMap[req.GetID()]
	transaction.Block = block

	c.traceMem(req, now, "read-hit", req.Address, req.MemByteSize, nil)
}

func (c *L1VCache) handleWriteReq(now akita.VTimeInSec, req *mem.WriteReq) {
	c.writeToLowModule(now, req)
	c.writeToLocalStorage(now, req)

	c.traceMem(req, now, "write", req.Address, uint64(len(req.Data)),
		req.Data)
}

func (c *L1VCache) writeToLowModule(now akita.VTimeInSec, req *mem.WriteReq) {
	l2 := c.L2Finder.Find(req.Address)
	writeBottom := mem.NewWriteReq(now, c.ToL2, l2, req.Address)
	writeBottom.Data = req.Data

	c.toL2Buffer = append(c.toL2Buffer, writeBottom)
	c.pendingDownGoingWrite = append(c.pendingDownGoingWrite, writeBottom)

	transaction := c.reqIDToTransactionMap[req.GetID()]
	transaction.ReqToBottom = writeBottom
}

func (c *L1VCache) writeToLocalStorage(now akita.VTimeInSec, req *mem.WriteReq) {
	cacheLineID, _ := cache.GetCacheLineID(req.Address, c.BlockSizeAsPowerOf2)
	block := c.Directory.Lookup(cacheLineID)

	transaction := c.reqIDToTransactionMap[req.GetID()]
	transaction.Block = block

	if block == nil {
		c.doWriteMiss(now, req)
	} else {
		c.doWriteHit(now, req, block)
	}
}

func (c *L1VCache) doWriteMiss(
	now akita.VTimeInSec,
	req *mem.WriteReq,
) {
	if len(req.Data) == 1<<c.BlockSizeAsPowerOf2 {
		evict := c.Directory.Evict(req.Address)
		c.doWriteLine(now, req, evict)
		return
	}
	// No partial write when write miss
}

func (c *L1VCache) doWriteHit(
	now akita.VTimeInSec,
	req *mem.WriteReq,
	block *cache.Block,
) {
	if len(req.Data) == 1<<c.BlockSizeAsPowerOf2 {
		c.doWriteLine(now, req, block)
		return
	}
	c.doWritePartialLine(now, req, block)
}

func (c *L1VCache) doWriteLine(
	now akita.VTimeInSec,
	req *mem.WriteReq,
	block *cache.Block,
) {
	block.IsValid = true
	block.Tag = req.Address

	transaction := c.reqIDToTransactionMap[req.GetID()]
	transaction.Block = block

	c.Storage.Write(block.CacheAddress, req.Data)
}

func (c *L1VCache) doWritePartialLine(
	now akita.VTimeInSec,
	req *mem.WriteReq,
	block *cache.Block,
) {
	_, offset := cache.GetCacheLineID(req.Address, c.BlockSizeAsPowerOf2)
	c.Storage.Write(block.CacheAddress+offset, req.Data)
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
	readBottom := c.pendingDownGoingRead[0]
	address := readBottom.Address

	block := c.Directory.Evict(readBottom.Address)
	block.IsValid = true
	block.IsDirty = false
	block.Tag = address
	c.Storage.Write(block.CacheAddress, dataReady.Data)

	for i, reqFromTop := range c.reqBuf {
		readFromTop, ok := reqFromTop.Req.(*mem.ReadReq)
		if !ok {
			continue
		}

		cacheLineID, offset := cache.GetCacheLineID(
			readFromTop.Address, c.BlockSizeAsPowerOf2)
		if cacheLineID != address {
			continue
		}

		dataReadyToTop := mem.NewDataReadyRsp(
			now, c.ToCU, readFromTop.Src(), readFromTop.GetID())
		dataReadyToTop.Data =
			dataReady.Data[offset : offset+readFromTop.MemByteSize]
		c.reqBuf[i].Rsp = dataReadyToTop

		c.traceMem(readFromTop, now, "data-ready",
			address, uint64(len(dataReady.Data)), dataReady.Data)
	}

	c.ToL2.Retrieve(now)
	c.pendingDownGoingRead = c.pendingDownGoingRead[1:]
	c.NeedTick = true

}

func (c *L1VCache) handleDoneRsp(now akita.VTimeInSec, rsp *mem.DoneRsp) {
	for _, trans := range c.reqBuf {
		if trans.ReqToBottom == nil {
			continue
		}

		if trans.ReqToBottom.GetID() != rsp.RespondTo {
			continue
		}

		write := trans.Req.(*mem.WriteReq)
		done := mem.NewDoneRsp(now, c.ToCU, write.Src(), write.GetID())
		trans.Rsp = done

		c.traceMem(trans.Req, now, "write-done",
			write.Address, uint64(len(write.Data)), write.Data)
	}

	c.ToL2.Retrieve(now)
	c.pendingDownGoingWrite = c.pendingDownGoingWrite[1:]

	c.NeedTick = true

}

func (c *L1VCache) doReadWrite(now akita.VTimeInSec) {

	for _, s := range c.inPipeline {
		s.CycleLeft--
		c.NeedTick = true

		if s.CycleLeft <= 0 {
			c.inPipeline = c.inPipeline[1:]

			switch req := s.Req.(type) {
			case *mem.ReadReq:
				c.finishLocalRead(now, req)
			default:
				log.Panicf("cannot handle request of type %s",
					reflect.TypeOf(req))
			}

			break
		}
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
	traceInfo.Req = req
	traceInfo.Where = c.Name()
	traceInfo.When = time
	traceInfo.What = what
	traceInfo.Address = address
	traceInfo.ByteSize = byteSize
	traceInfo.Data = data
	c.InvokeHook(nil, c, akita.AnyHookPos, traceInfo)
}

// NewL1VCache creates a new L1VCache
func NewL1VCache(name string, engine akita.Engine, freq akita.Freq) *L1VCache {
	c := new(L1VCache)
	c.TickingComponent = akita.NewTickingComponent(name, engine, freq, c)

	c.reqBufCapacity = 256
	c.reqIDToTransactionMap = make(map[string]*cacheTransaction)

	c.pipeline = pipelines.NewPipeline()
	c.pipeline.SetStageLatency(2)
	c.pipeline.SetNumStages(50)
	c.pipeline.SetFrequency(freq)
	c.pipeline.SetNumLines(4)

	c.ToCU = akita.NewPort(c)
	c.ToCU.BufCapacity = 4
	c.ToCP = akita.NewPort(c)
	c.ToCP.BufCapacity = 4
	c.ToL2 = akita.NewPort(c)
	c.ToL2.BufCapacity = 4
	return c
}

// BuildL1VCache configures an L1VCache with specified parameters.
func BuildL1VCache(
	name string,
	engine akita.Engine, freq akita.Freq,
	latency int,
	blockSizeAsPowerOf2, way, sizeAsPowerOf2 uint64,
	l2Finder cache.LowModuleFinder,
) *L1VCache {
	c := NewL1VCache(name, engine, freq)

	blockSize := uint64(1 << blockSizeAsPowerOf2)
	totalSize := uint64(1 << sizeAsPowerOf2)

	lruEvictor := cache.NewLRUEvictor()
	directory := cache.NewDirectory(
		int(totalSize/way/blockSize),
		int(way), int(blockSize), lruEvictor)
	storage := mem.NewStorage(totalSize)

	c.Directory = directory
	c.Storage = storage
	c.L2Finder = l2Finder

	c.BlockSizeAsPowerOf2 = blockSizeAsPowerOf2
	c.Latency = latency
	c.InvalidationLatency = int(totalSize / way / blockSize)

	return c
}
