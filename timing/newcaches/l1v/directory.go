package l1v

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/util"
)

type directory struct {
	inBuf util.Buffer
}

func (d *directory) Tick(now akita.VTimeInSec) bool {
	item := d.inBuf.Peek()
	if item == nil {
		return false
	}

	panic("not implemented")
}
