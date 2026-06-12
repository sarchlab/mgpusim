package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type respondStage struct {
	cache *pipelineMW
}

func (s *respondStage) Tick() bool {
	next := s.cache.comp.GetNextState()
	if len(next.Transactions) == 0 {
		return false
	}

	for i := 0; i < len(next.Transactions); i++ {
		trans := &next.Transactions[i]
		if !trans.Done || trans.Removed {
			continue
		}

		if trans.HasRead {
			return s.respondReadTrans(trans)
		}

		return s.respondWriteTrans(trans)
	}

	return false
}

func (s *respondStage) respondReadTrans(trans *transactionState) bool {
	dr := &mem.DataReadyRsp{}
	dr.ID = sim.GetIDGenerator().Generate()
	dr.Src = s.cache.topPort.AsRemote()
	dr.Dst = trans.ReadMeta.Src
	dr.RspTo = trans.ReadMeta.ID
	dr.Data = trans.Data
	dr.TrafficBytes = len(trans.Data) + 4
	dr.TrafficClass = "rsp"

	err := s.cache.topPort.Send(dr)
	if err != nil {
		return false
	}

	trans.Removed = true

	// Reconstruct read for tracing
	read := &mem.ReadReq{
		MsgMeta:        trans.ReadMeta,
		Address:        trans.ReadAddress,
		AccessByteSize: trans.ReadAccessByteSize,
		PID:            trans.ReadPID,
	}
	tracing.TraceReqComplete(read, s.cache.comp)

	return true
}

func (s *respondStage) respondWriteTrans(trans *transactionState) bool {
	done := &mem.WriteDoneRsp{}
	done.ID = sim.GetIDGenerator().Generate()
	done.Src = s.cache.topPort.AsRemote()
	done.Dst = trans.WriteMeta.Src
	done.RspTo = trans.WriteMeta.ID
	done.TrafficBytes = 4
	done.TrafficClass = "rsp"

	err := s.cache.topPort.Send(done)
	if err != nil {
		return false
	}

	trans.Removed = true

	// Reconstruct write for tracing
	write := &mem.WriteReq{
		MsgMeta:   trans.WriteMeta,
		Address:   trans.WriteAddress,
		Data:      trans.WriteData,
		DirtyMask: trans.WriteDirtyMask,
		PID:       trans.WritePID,
	}
	tracing.TraceReqComplete(write, s.cache.comp)

	return true
}
