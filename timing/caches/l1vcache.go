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

type L1VCache struct {
	*core.TickingComponent

	ToCU *core.Port
	ToL2 *core.Port

	Directory cache.Directory

	DirectoryBusy []directoryBusy

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

func NewL1VCache(name string, engine core.Engine, freq core.Freq) *L1VCache {
	c := new(L1VCache)
	c.TickingComponent = core.NewTickingComponent(name, engine, freq, c)

	c.ToCU = core.NewPort(c)
	c.ToL2 = core.NewPort(c)
	return c
}
