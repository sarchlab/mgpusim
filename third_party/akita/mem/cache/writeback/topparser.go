package writeback

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type topParser struct {
	cache *pipelineMW
}

func (p *topParser) Tick() bool {
	next := &p.cache.comp.State

	if cacheState(next.CacheState) != cacheStateRunning {
		return false
	}

	msg := p.cache.topPort().PeekIncoming()
	if msg == nil {
		return false
	}

	if !next.DirStageBuf.CanPush() {
		return false
	}

	trans := transactionState{
		ID: timing.GetIDGenerator().Generate(),
	}

	switch msg := msg.(type) {
	case memprotocol.ReadReq:
		trans.HasRead = true
		trans.ReadMeta = msg.MsgMeta
		trans.ReadAddress = msg.Address
		trans.ReadAccessByteSize = msg.AccessByteSize
		trans.ReadPID = msg.PID
	case memprotocol.WriteReq:
		trans.HasWrite = true
		trans.WriteMeta = msg.MsgMeta
		trans.WriteAddress = msg.Address
		trans.WriteData = msg.Data
		trans.WriteDirtyMask = msg.DirtyMask
		trans.WritePID = msg.PID
	}

	idx := next.allocTransaction(trans)
	next.DirStageBuf.PushTyped(idx)

	tracing.TraceReqReceive(p.cache.comp, msg)

	p.cache.topPort().RetrieveIncoming()

	return true
}
