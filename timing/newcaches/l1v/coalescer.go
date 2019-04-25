package l1v

import "gitlab.com/akita/akita"

type coalescer struct {
	topPort akita.Port
}

func (c *coalescer) Tick(now akita.VTimeInSec) bool {

}
