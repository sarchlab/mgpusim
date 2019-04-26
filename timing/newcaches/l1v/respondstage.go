package l1v

import "gitlab.com/akita/akita"

type respondStage struct {
	topPort      akita.Port
	transactions *[]*transaction
}

func (s *respondStage) Tick(now akita.VTimeInSec) bool {
	if len(*s.transactions) == 0 {
		return false
	}

	trans := (*s.transactions)[0]
	if trans.read != nil {
		return s.respondReadTrans(now, trans)
	}

	panic("not implemented")
}

func (s *respondStage) respondReadTrans(
	now akita.VTimeInSec,
	trans *transaction,
) bool {
	if trans.dataReadyFromBottom == nil {
		return false
	}

	panic("not implemnted")
}
