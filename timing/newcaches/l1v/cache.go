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

	dirBuf util.Buffer

	transactions []*transaction
}

// Tick update the state of the cache
func (c *Cache) Tick(now akita.VTimeInSec) bool {
	return false
}

// NewCache returns a newly created cache
func NewCache(name string, engine akita.Engine, freq akita.Freq) *Cache {
	c := &Cache{}
	c.TickingComponent = akitaext.NewTickingComponent(name, engine, freq, c)
	return c
}
