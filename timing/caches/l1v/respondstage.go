package l1v

import (
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mem/v2/mem"
	"gitlab.com/akita/util/v2/tracing"
)

type respondStage struct {
	cache *Cache
}

func (s *respondStage) Tick(now sim.VTimeInSec) bool {
	if len(s.cache.transactions) == 0 {
		return false
	}

	for _, trans := range s.cache.transactions {
		if !trans.done {
			continue
		}

		if trans.read != nil {
			return s.respondReadTrans(now, trans)
		}
		return s.respondWriteTrans(now, trans)
	}

	return false
}

func (s *respondStage) respondReadTrans(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	if !trans.done {
		return false
	}

	read := trans.read
	dr := mem.DataReadyRspBuilder{}.
		WithSendTime(now).
		WithSrc(s.cache.topPort).
		WithDst(read.Src).
		WithRspTo(read.ID).
		WithData(trans.data).
		Build()
	err := s.cache.topPort.Send(dr)
	if err != nil {
		return false
	}

	s.removeTransaction(trans)

	tracing.TraceReqComplete(read, now, s.cache)

	return true
}

func (s *respondStage) respondWriteTrans(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	if !trans.done {
		return false
	}

	write := trans.write
	done := mem.WriteDoneRspBuilder{}.
		WithSendTime(now).
		WithSrc(s.cache.topPort).
		WithDst(write.Src).
		WithRspTo(write.ID).
		Build()
	err := s.cache.topPort.Send(done)
	if err != nil {
		return false
	}

	s.removeTransaction(trans)

	tracing.TraceReqComplete(write, now, s.cache)

	return true
}

func (s *respondStage) removeTransaction(trans *transaction) {
	for i, t := range s.cache.transactions {
		if t == trans {
			s.cache.transactions = append(s.cache.transactions[:i],
				s.cache.transactions[i+1:]...)
			return
		}
	}

	panic("not found")
}
