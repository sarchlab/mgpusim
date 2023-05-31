package writeevict

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
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

	tracing.TraceReqComplete(read, s.cache)

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

	tracing.TraceReqComplete(write, s.cache)

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
