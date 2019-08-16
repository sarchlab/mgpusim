package l1v

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
)

type respondStage struct {
	name         string
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
	dr := mem.NewDataReadyRsp(now, s.topPort, read.Src(), read.GetID())
	dr.Data = trans.data
	err := s.topPort.Send(dr)
	if err != nil {
		return false
	}

	*s.transactions = (*s.transactions)[1:]

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
	done := mem.NewDoneRsp(now, s.topPort, write.Src(), write.GetID())
	err := s.topPort.Send(done)
	if err != nil {
		return false
	}

	*s.transactions = (*s.transactions)[1:]

	return true
}
