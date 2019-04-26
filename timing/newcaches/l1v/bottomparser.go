package l1v

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/util"
)

type bottomParser struct {
	bottomPort   akita.Port
	bankBufs     []util.Buffer
	transactions *[]*transaction
}

func (p *bottomParser) Tick(now akita.VTimeInSec) bool {
	panic("not implemented")
}
