package l1v

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/util"
	"gitlab.com/akita/util/akitaext"
)

// A Cache is a customized L1 cache the for R9nano GPUs.
type Cache struct {
	*akitaext.TickingComponent

	TopPort     akita.Port
	BottomPort  akita.Port
	ControlPort akita.Port

	dirBuf   util.Buffer
	bankBufs []util.Buffer

	coalesceStage    *coalescer
	directoryStage   *directory
	parseBottomStage *bottomParser
	respondStage     *respondStage

	transactions             []*transaction
	postCoalesceTransactions []*transaction
}

// Tick update the state of the cache
func (c *Cache) Tick(now akita.VTimeInSec) bool {
	madeProgress := false

	madeProgress = c.respondStage.Tick(now) || madeProgress
	madeProgress = c.parseBottomStage.Tick(now) || madeProgress
	madeProgress = c.directoryStage.Tick(now) || madeProgress
	madeProgress = c.coalesceStage.Tick(now) || madeProgress

	return madeProgress
}

// NewCache returns a newly created cache
func NewCache(
	name string,
	engine akita.Engine,
	freq akita.Freq,
	log2BlockSize uint64,
	wayAssocitivity int,
) *Cache {
	c := &Cache{}
	c.TickingComponent = akitaext.NewTickingComponent(name, engine, freq, c)

	c.TopPort = akita.NewLimitNumReqPort(c, 4)
	c.BottomPort = akita.NewLimitNumReqPort(c, 4)
	c.ControlPort = akita.NewLimitNumReqPort(c, 4)

	c.dirBuf = util.NewBuffer(4)

	c.coalesceStage = &coalescer{
		topPort:                  c.TopPort,
		dirBuf:                   c.dirBuf,
		transactions:             &c.transactions,
		postCoalesceTransactions: &c.postCoalesceTransactions,
	}
	c.directoryStage = &directory{
		inBuf: c.dirBuf,
	}

	return c
}
