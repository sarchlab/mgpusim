package l1v

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/util"
)

type bankStage struct {
	inBuf        util.Buffer
	topPort      akita.Port
	transactions *[]*transaction
	latency      int

	cycleLeft int
	currTrans *transaction
}

func (s *bankStage) Tick(now akita.VTimeInSec) bool {
	if s.currTrans != nil {
		s.cycleLeft--
		return true
	}
	return s.extractFromBuf()
}

func (s *bankStage) extractFromBuf() bool {
	item := s.inBuf.Peek()
	if item == nil {
		return false
	}

	s.currTrans = item.(*transaction)
	s.cycleLeft = s.latency
	s.inBuf.Pop()
	return true
}
