package l1v

import "gitlab.com/akita/akita"

type respondStage struct {
	topPort      akita.Port
	transactions *[]*transaction
}

func (s *respondStage) Tick(now akita.VTimeInSec) bool {
	panic("not implemented")
}
