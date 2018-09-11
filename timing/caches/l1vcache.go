package caches

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

type L1VCache struct {
	*akita.TickingComponent

	ToCU *akita.Port
	ToL2 *akita.Port

	L2Finder cache.LowModuleFinder

	Directory cache.Directory
	Storage   *mem.Storage

	BlockSizeAsPowerOf2 uint64
	Latency             int

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
	c.parseFromL2(now)
	c.parseFromCU(now)

	if c.NeedTick {
		c.TickLater(now)
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
}

func (c *L1VCache) handleReadHit(now akita.VTimeInSec, req *mem.ReadReq, block *cache.Block) {
	c.cycleLeft = c.Latency
	c.busyBlock = block
	c.isStorageBusy = true
}

func (c *L1VCache) handleWriteReq(now akita.VTimeInSec, req *mem.WriteReq) {
	c.isBusy = true
	c.writing = req
	l2 := c.L2Finder.Find(req.Address)
	writeBottom := mem.NewWriteReq(now, c.ToL2, l2, req.Address)
	writeBottom.Data = req.Data

	c.toL2Buffer = append(c.toL2Buffer, writeBottom)
	c.pendingDownGoingWrite = append(c.pendingDownGoingWrite, writeBottom)

	if len(req.Data) == 1<<c.BlockSizeAsPowerOf2 {
		block := c.Directory.Evict(req.Address)
		c.Storage.Write(block.CacheAddress, writeBottom.Data)
		block.IsValid = true
		block.Tag = req.Address
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
	readBottom := c.pendingDownGoingRead[0]
	readTop := c.reading
	address := readTop.Address
	_, offset := cache.GetCacheLineID(address, c.BlockSizeAsPowerOf2)

	block := c.Directory.Evict(readBottom.Address)
	block.IsValid = true
	block.IsDirty = false
	c.Storage.Write(block.CacheAddress, dataReady.Data)

	dataReadyToTop := mem.NewDataReadyRsp(
		now, c.ToCU, c.reading.Src(), c.reading.GetID())
	dataReadyToTop.Data = dataReady.Data[offset : offset+readTop.MemByteSize]
	c.toCUBuffer = append(c.toCUBuffer, dataReadyToTop)

	c.ToL2.Retrieve(now)
	c.pendingDownGoingRead = nil
	c.isBusy = false
	c.NeedTick = true
}

func (c *L1VCache) handleDoneRsp(now akita.VTimeInSec, rsp *mem.DoneRsp) {
	done := mem.NewDoneRsp(now, c.ToCU, c.writing.Src(), c.writing.ID)
	c.toCUBuffer = append(c.toCUBuffer, done)

	c.ToL2.Retrieve(now)
	c.pendingDownGoingWrite = nil
	c.writing = nil
	c.isBusy = false
	c.NeedTick = true
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

func NewL1VCache(name string, engine akita.Engine, freq akita.Freq) *L1VCache {
	c := new(L1VCache)
	c.TickingComponent = akita.NewTickingComponent(name, engine, freq, c)

	c.ToCU = akita.NewPort(c)
	c.ToL2 = akita.NewPort(c)
	return c
}
