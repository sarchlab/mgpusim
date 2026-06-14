package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type respondStage struct {
	cache *pipelineMW
}

func (s *respondStage) Tick() bool {
	next := &s.cache.comp.State
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
	dr := memprotocol.DataReadyRsp{}
	dr.ID = timing.GetIDGenerator().Generate()
	dr.Src = s.cache.topPort().AsRemote()
	dr.Dst = trans.ReadMeta.Src
	dr.RspTo = trans.ReadMeta.ID
	dr.Data = trans.Data
	dr.TrafficBytes = len(trans.Data) + 4
	dr.TrafficClass = "rsp"

	if !s.cache.topPort().CanSend() {
		return false
	}

	s.cache.topPort().Send(dr)

	trans.Removed = true

	// Reconstruct read for tracing
	read := memprotocol.ReadReq{
		MsgMeta:        trans.ReadMeta,
		Address:        trans.ReadAddress,
		AccessByteSize: trans.ReadAccessByteSize,
		PID:            trans.ReadPID,
	}
	tracing.TraceReqComplete(s.cache.comp, read)

	return true
}

func (s *respondStage) respondWriteTrans(trans *transactionState) bool {
	done := memprotocol.WriteDoneRsp{}
	done.ID = timing.GetIDGenerator().Generate()
	done.Src = s.cache.topPort().AsRemote()
	done.Dst = trans.WriteMeta.Src
	done.RspTo = trans.WriteMeta.ID
	done.TrafficBytes = 4
	done.TrafficClass = "rsp"

	if !s.cache.topPort().CanSend() {
		return false
	}

	s.cache.topPort().Send(done)

	trans.Removed = true

	// Reconstruct write for tracing
	write := memprotocol.WriteReq{
		MsgMeta:   trans.WriteMeta,
		Address:   trans.WriteAddress,
		Data:      trans.WriteData,
		DirtyMask: trans.WriteDirtyMask,
		PID:       trans.WritePID,
	}
	tracing.TraceReqComplete(s.cache.comp, write)

	return true
}
