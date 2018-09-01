package caches

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
	"gitlab.com/yaotsu/mem/cache"
)

type directoryBusy struct {
	req       mem.AccessReq
	cycleLeft int
}

type cacheRamBusy struct {
	req       mem.AccessReq
	cycleLeft int
	block     *cache.Block
}

type L1VCache struct {
	*core.TickingComponent

	ToCU *core.Port
	ToL2 *core.Port

	Directory cache.Directory

	DirectoryBusy []directoryBusy
	CacheRamBusy  []cacheRamBusy

	NumBank                uint64
	BankInterleaving       uint64
	BlockSizeAsPowerOf2    uint64
	DirectoryLookupLatency int
	ReadLatency            int
	WriteLatency           int
}

func (c *L1VCache) Handle(e core.Event) error {
	switch e := e.(type) {
	case *core.TickEvent:
		c.handleTickEvent(e)
	default:
		log.Panicf("cannot handle event of type %s", reflect.TypeOf(e))
	}
	return nil
}

func (c *L1VCache) handleTickEvent(e *core.TickEvent) {
	now := e.Time()
	c.NeedTick = false

	c.parseFromCU(now)

	if c.NeedTick {
		c.TickLater(now)
	}
}

func (c *L1VCache) parseFromCU(now core.VTimeInSec) {
	req := c.ToCU.Peek()
	switch req := req.(type) {
	case *mem.ReadReq:
		bankID := req.GetAddress() / c.BankInterleaving % c.NumBank
		if c.DirectoryBusy[bankID].req == nil {
			c.DirectoryBusy[bankID].cycleLeft = c.DirectoryLookupLatency
			c.DirectoryBusy[bankID].req = req
			c.ToCU.Retrieve(now)
			c.NeedTick = true
		}
	default:
		log.Panicf("cannot process request of type %s",
			reflect.TypeOf(req))
	}
}

func (c *L1VCache) lookupDirectory(now core.VTimeInSec, bankID uint64) {
	if c.DirectoryBusy[bankID].req == nil {
		return
	}

	if c.DirectoryBusy[bankID].cycleLeft <= 0 {
		req := c.DirectoryBusy[bankID].req
		block := c.Directory.Lookup(req.GetAddress())

		switch req := req.(type) {
		case *mem.ReadReq:
			if block != nil {
				c.doReadHit(now, bankID, req, block)
			} else {
				c.doReadMiss(now, bankID, req)
			}

		case *mem.WriteReq:
		default:
			log.Panicf("cannot process request of type %s",
				reflect.TypeOf(req))
		}
	}
}

func (c *L1VCache) doReadHit(
	now core.VTimeInSec,
	bankID uint64,
	req *mem.ReadReq,
	block *cache.Block,
) {
	c.DirectoryBusy[bankID].req = nil
	c.CacheRamBusy[bankID].req = req
	c.CacheRamBusy[bankID].cycleLeft = c.ReadLatency
	c.NeedTick = true
}

func (c *L1VCache) doReadMiss(
	now core.VTimeInSec,
	bankID uint64,
	req *mem.ReadReq,
) {

}

func NewL1VCache(name string, engine core.Engine, freq core.Freq) *L1VCache {
	c := new(L1VCache)
	c.TickingComponent = core.NewTickingComponent(name, engine, freq, c)

	c.ToCU = core.NewPort(c)
	c.ToL2 = core.NewPort(c)
	return c
}
