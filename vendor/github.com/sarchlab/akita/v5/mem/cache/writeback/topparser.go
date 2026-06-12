package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type topParser struct {
	cache *pipelineMW
}

func (p *topParser) Tick() bool {
	next := p.cache.comp.GetNextState()

	if cacheState(next.CacheState) != cacheStateRunning {
		return false
	}

	msg := p.cache.topPort.PeekIncoming()
	if msg == nil {
		return false
	}

	if !next.DirStageBuf.CanPush() {
		return false
	}

	trans := transactionState{
		ID: sim.GetIDGenerator().Generate(),
	}

	switch msg := msg.(type) {
	case *mem.ReadReq:
		trans.HasRead = true
		trans.ReadMeta = msg.MsgMeta
		trans.ReadAddress = msg.Address
		trans.ReadAccessByteSize = msg.AccessByteSize
		trans.ReadPID = msg.PID
	case *mem.WriteReq:
		trans.HasWrite = true
		trans.WriteMeta = msg.MsgMeta
		trans.WriteAddress = msg.Address
		trans.WriteData = msg.Data
		trans.WriteDirtyMask = msg.DirtyMask
		trans.WritePID = msg.PID
	}

	next.Transactions = append(next.Transactions, trans)

	idx := len(next.Transactions) - 1
	next.DirStageBuf.PushTyped(idx)

	tracing.TraceReqReceive(msg, p.cache.comp)

	p.cache.topPort.RetrieveIncoming()

	return true
}
