package caches

import (
	"log"
	"reflect"

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

	cycleLeft int
	isBusy    bool
	reading   *mem.ReadReq
	writing   *mem.WriteReq

	isStorageBusy bool
	busyBlock     *cache.Block

	toCUBuffer            []akita.Req
	toL2Buffer            []akita.Req
	pendingDownGoingRead  []*mem.ReadReq
	pendingDownGoingWrite []*mem.WriteReq
}

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

func (c *L1VCache) handleTickEvent(e *akita.TickEvent) {
	now := e.Time()
	c.NeedTick = false

	c.sendToCU(now)
	c.sendToL2(now)
	c.doReadWrite(now)
	c.parseFromCP(now)
	c.parseFromL2(now)
	c.parseFromCU(now)

	if c.NeedTick {
		c.TickLater(now)
	}
}

func (c *L1VCache) parseFromCP(now akita.VTimeInSec) {
	if c.isBusy {
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
	if c.isBusy {
		return
	}

	req := c.ToCU.Retrieve(now)
	if req == nil {
		return
	}

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
	c.isBusy = true
	c.reading = req

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
	l2 := c.L2Finder.Find(cacheLineID)
	readBottom := mem.NewReadReq(now, c.ToL2, l2, cacheLineID, 1<<c.BlockSizeAsPowerOf2)
	c.pendingDownGoingRead = append(c.pendingDownGoingRead, readBottom)
	c.toL2Buffer = append(c.toL2Buffer, readBottom)

	c.traceMem(now, "read-miss", address, req.MemByteSize, nil)
}

func (c *L1VCache) handleReadHit(now akita.VTimeInSec, req *mem.ReadReq, block *cache.Block) {
	c.cycleLeft = c.Latency
	c.busyBlock = block
	c.isStorageBusy = true

	c.traceMem(now, "read-hit", req.Address, req.MemByteSize, nil)
}

func (c *L1VCache) handleWriteReq(now akita.VTimeInSec, req *mem.WriteReq) {
	c.isBusy = true
	c.writing = req

	c.writeToLowModule(now, req)
	c.writeToLocalStorage(now, req)

	c.traceMem(now, "write", req.Address, uint64(len(req.Data)),
		req.Data)
}

func (c *L1VCache) writeToLowModule(now akita.VTimeInSec, req *mem.WriteReq) {
	l2 := c.L2Finder.Find(req.Address)
	writeBottom := mem.NewWriteReq(now, c.ToL2, l2, req.Address)
	writeBottom.Data = req.Data

	c.toL2Buffer = append(c.toL2Buffer, writeBottom)
	c.pendingDownGoingWrite = append(c.pendingDownGoingWrite, writeBottom)
}

func (c *L1VCache) writeToLocalStorage(now akita.VTimeInSec, req *mem.WriteReq) {
	cacheLineID, _ := cache.GetCacheLineID(req.Address, c.BlockSizeAsPowerOf2)
	block := c.Directory.Lookup(cacheLineID)
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
	// Do no do partial write when write miss
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
	readTop := c.reading
	address := readTop.Address
	_, offset := cache.GetCacheLineID(address, c.BlockSizeAsPowerOf2)

	block := c.Directory.Evict(readBottom.Address)
	block.IsValid = true
	block.IsDirty = false
	block.Tag = address
	c.Storage.Write(block.CacheAddress, dataReady.Data)

	dataReadyToTop := mem.NewDataReadyRsp(
		now, c.ToCU, c.reading.Src(), c.reading.GetID())
	dataReadyToTop.Data = dataReady.Data[offset : offset+readTop.MemByteSize]
	c.toCUBuffer = append(c.toCUBuffer, dataReadyToTop)

	c.ToL2.Retrieve(now)
	c.pendingDownGoingRead = nil
	c.isBusy = false
	c.NeedTick = true

	c.traceMem(now, "data-ready", address, uint64(len(dataReady.Data)),
		dataReady.Data)
}

func (c *L1VCache) handleDoneRsp(now akita.VTimeInSec, rsp *mem.DoneRsp) {
	done := mem.NewDoneRsp(now, c.ToCU, c.writing.Src(), c.writing.ID)
	c.toCUBuffer = append(c.toCUBuffer, done)
	write := c.pendingDownGoingWrite[0]

	c.ToL2.Retrieve(now)
	c.pendingDownGoingWrite = nil
	c.writing = nil
	c.isBusy = false
	c.NeedTick = true

	c.traceMem(now, "write-done", write.Address, uint64(len(write.Data)), write.Data)
}

func (c *L1VCache) doReadWrite(now akita.VTimeInSec) {
	if !c.isStorageBusy {
		return
	}

	c.cycleLeft--
	c.NeedTick = true

	if c.cycleLeft <= 0 {
		if c.reading != nil {
			c.finishLocalRead(now)
		}
	}
}

func (c *L1VCache) finishLocalRead(now akita.VTimeInSec) {
	c.isStorageBusy = false
	c.isBusy = false

	_, offset := cache.GetCacheLineID(c.reading.Address, c.BlockSizeAsPowerOf2)
	data, err := c.Storage.Read(
		c.busyBlock.CacheAddress+offset, c.reading.MemByteSize)
	if err != nil {
		log.Panic(err)
	}

	dataReady := mem.NewDataReadyRsp(now, c.ToCU, c.reading.Src(), c.reading.ID)
	dataReady.Data = data
	c.toCUBuffer = append(c.toCUBuffer, dataReady)

	c.traceMem(now, "local_read_done", c.reading.Address,
		uint64(len(dataReady.Data)), data)
}

func (c *L1VCache) sendToCU(now akita.VTimeInSec) {
	if len(c.toCUBuffer) > 0 {
		req := c.toCUBuffer[0]
		req.SetSendTime(now)
		err := c.ToCU.Send(req)
		if err == nil {
			c.toCUBuffer = c.toCUBuffer[1:]
			c.NeedTick = true
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
	c.InvokeHook(nil, c, akita.AnyHookPos, traceInfo)
}

func NewL1VCache(name string, engine akita.Engine, freq akita.Freq) *L1VCache {
	c := new(L1VCache)
	c.TickingComponent = akita.NewTickingComponent(name, engine, freq, c)

	c.ToCU = akita.NewPort(c)
	c.ToCP = akita.NewPort(c)
	c.ToL2 = akita.NewPort(c)
	return c
}

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

	return c
}
