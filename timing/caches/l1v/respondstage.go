package l1v

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/util/tracing"
)

type respondStage struct {
	cache *Cache
}

func (s *respondStage) Tick(now akita.VTimeInSec) bool {
	if len(s.cache.transactions) == 0 {
		return false
	}

	trans := s.cache.transactions[0]
	if trans.read != nil {
		return s.respondReadTrans(now, trans)
	}
	return s.respondWriteTrans(now, trans)
}

func (s *respondStage) respondReadTrans(
	now akita.VTimeInSec,
	trans *transaction,
) bool {
	if !trans.done {
		return false
	}

	read := trans.read
	dr := mem.NewDataReadyRsp(now, s.cache.TopPort, read.Src(), read.GetID())
	dr.Data = trans.data
	err := s.cache.TopPort.Send(dr)
	if err != nil {
		return false
	}

	s.cache.transactions = s.cache.transactions[1:]

	tracing.TraceReqComplete(read, now, s.cache)
	if s.cache.Name() == "GPU_1.L1I_01" {
		fmt.Printf("returning\n")
	}

	return true
}

func (s *respondStage) respondWriteTrans(
	now akita.VTimeInSec,
	trans *transaction,
) bool {
	if !trans.done {
		return false
	}

	write := trans.write
	done := mem.NewDoneRsp(now, s.cache.TopPort, write.Src(), write.GetID())
	err := s.cache.TopPort.Send(done)
	if err != nil {
		return false
	}

	s.cache.transactions = s.cache.transactions[1:]

	tracing.TraceReqComplete(write, now, s.cache)

	return true
}
