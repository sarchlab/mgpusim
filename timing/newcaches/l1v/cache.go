package l1v

import (
	"gitlab.com/akita/akita"
)

// A Cache is a customized L1 cache the for R9nano GPUs.
type Cache struct {
	*util.TickingComponent

	TopPort     akita.Port
	BottomPort  akita.Port
	ControlPort akita.Port
}

// Tick update the state of the cache
func (c *Cache) Tick(now akita.VTimeInSec) bool {
	return false
}

// NewCache returns a newly created cache
func NewCache(name string, engine akita.Engine, freq akita.Freq) *Cache {
	c := &Cache{}
	c.TickingComponent = util.NewTickingComponent()
	return c
}
