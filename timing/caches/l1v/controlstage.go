package l1v

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem/cache"
)

type controlStage struct {
	ctrlPort     akita.Port
	transactions *[]*transaction
	directory    cache.Directory

	currReq *cache.FlushReq
}

func (s *controlStage) Tick(now akita.VTimeInSec) bool {
	if s.currReq == nil {
		item := s.ctrlPort.Peek()
		if item == nil {
			return false
		}

		s.currReq = item.(*cache.FlushReq)
		s.ctrlPort.Retrieve(now)
	}

	if len(*s.transactions) > 0 {
		return false
	}

	rsp := cache.FlushRspBuilder{}.
		WithSendTime(now).
		WithSrc(s.ctrlPort).
		WithDst(s.currReq.Src).
		WithRspTo(s.currReq.ID).
		Build()
	err := s.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	s.directory.Reset()
	s.currReq = nil
	return true
}
