package writeback

import (
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// blockRef is a set+way pair referencing a block in the directory.
type blockRef struct {
	SetID int `json:"set_id"`
	WayID int `json:"way_id"`
}

type flusher struct {
	pipeline *pipelineMW
	ctrlPort sim.Port
}

func (f *flusher) Tick() bool {
	next := f.pipeline.comp.GetNextState()

	if next.HasProcessingFlush && cacheState(next.CacheState) == cacheStatePreFlushing {
		return f.processPreFlushing()
	}

	madeProgress := false
	if next.HasProcessingFlush && cacheState(next.CacheState) == cacheStateFlushing {
		madeProgress = f.finalizeFlushing() || madeProgress
		madeProgress = f.processFlush() || madeProgress

		return madeProgress
	}

	return f.extractFromPort()
}

func (f *flusher) processPreFlushing() bool {
	if f.existInflightTransaction() {
		return false
	}

	f.prepareBlockToFlushList()
	next := f.pipeline.comp.GetNextState()
	next.CacheState = int(cacheStateFlushing)

	return true
}

func (f *flusher) existInflightTransaction() bool {
	next := f.pipeline.comp.GetNextState()
	for _, t := range next.Transactions {
		if !t.Removed {
			return true
		}
	}
	return false
}

func (f *flusher) prepareBlockToFlushList() {
	next := f.pipeline.comp.GetNextState()
	for setID, set := range next.DirectoryState.Sets {
		for wayID, block := range set.Blocks {
			if block.ReadCount > 0 || block.IsLocked {
				panic("all the blocks should be unlocked before flushing")
			}

			if block.IsValid && block.IsDirty {
				next.FlusherBlockToEvictRefs = append(next.FlusherBlockToEvictRefs,
					blockRef{SetID: setID, WayID: wayID})
			}
		}
	}
}

func (f *flusher) processFlush() bool {
	next := f.pipeline.comp.GetNextState()

	if len(next.FlusherBlockToEvictRefs) == 0 {
		return false
	}

	spec := f.pipeline.comp.GetSpec()
	ref := next.FlusherBlockToEvictRefs[0]
	block := &next.DirectoryState.Sets[ref.SetID].Blocks[ref.WayID]
	bankNum := bankID(
		ref.SetID, ref.WayID,
		spec.WayAssociativity,
		len(next.DirToBankBufs))
	bankBuf := &next.DirToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	trans := transactionState{
		HasFlush:             true,
		FlushMeta:            next.ProcessingFlush.MsgMeta,
		FlushInvalidateAfter: next.ProcessingFlush.InvalidateAfter,
		FlushDiscardInflight: next.ProcessingFlush.DiscardInflight,
		FlushPauseAfter:      next.ProcessingFlush.PauseAfter,
		HasVictim:            true,
		VictimPID:            0,
		VictimTag:            block.Tag,
		VictimCacheAddress:   block.CacheAddress,
		Action:               bankEvict,
		EvictingAddr:         block.Tag,
		EvictingDirtyMask:    block.DirtyMask,
		BlockSetID:           ref.SetID,
		BlockWayID:           ref.WayID,
		HasBlock:             true,
	}

	next.Transactions = append(next.Transactions, trans)
	transIdx := len(next.Transactions) - 1
	bankBuf.PushTyped(transIdx)

	next.FlusherBlockToEvictRefs = next.FlusherBlockToEvictRefs[1:]

	return true
}

func (f *flusher) extractFromPort() bool {
	msg := f.ctrlPort.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *mem.ControlReq:
		switch msg.Command {
		case mem.CmdFlush:
			return f.startProcessingFlush(msg)
		case mem.CmdEnable:
			return f.handleCacheRestart(msg)
		default:
			log.Panicf("Cannot process control command %d", msg.Command)
		}
	default:
		log.Panicf("Cannot process request of type %T", msg)
	}

	return true
}

func (f *flusher) startProcessingFlush(msg *mem.ControlReq) bool {
	next := f.pipeline.comp.GetNextState()

	next.HasProcessingFlush = true
	next.ProcessingFlush = flushReqState{
		MsgMeta:         msg.MsgMeta,
		InvalidateAfter: msg.InvalidateAfter,
		DiscardInflight: msg.DiscardInflight,
		PauseAfter:      msg.PauseAfter,
	}

	if msg.DiscardInflight {
		f.pipeline.discardInflightTransactions()
	}

	next.CacheState = int(cacheStatePreFlushing)
	f.ctrlPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, f.pipeline.comp)

	return true
}

func (f *flusher) handleCacheRestart(msg *mem.ControlReq) bool {
	if !f.ctrlPort.CanSend() {
		return false
	}

	clearPort(f.pipeline.topPort)
	clearPort(f.pipeline.bottomPort)

	next := f.pipeline.comp.GetNextState()
	next.CacheState = int(cacheStateRunning)

	rsp := &mem.ControlRsp{Command: mem.CmdEnable, Success: true}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = f.ctrlPort.AsRemote()
	rsp.Dst = msg.Src
	rsp.RspTo = msg.ID
	rsp.TrafficClass = "mem.ControlRsp"
	f.ctrlPort.Send(rsp)

	f.ctrlPort.RetrieveIncoming()

	return true
}

func (f *flusher) finalizeFlushing() bool {
	next := f.pipeline.comp.GetNextState()

	if len(next.FlusherBlockToEvictRefs) > 0 {
		return false
	}

	if !f.flushCompleted() {
		return false
	}

	if !f.ctrlPort.CanSend() {
		return false
	}

	spec := f.pipeline.comp.GetSpec()

	rsp := &mem.ControlRsp{Command: mem.CmdFlush, Success: true}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = f.ctrlPort.AsRemote()
	rsp.Dst = next.ProcessingFlush.MsgMeta.Src
	rsp.RspTo = next.ProcessingFlush.MsgMeta.ID
	rsp.TrafficClass = "mem.ControlRsp"
	f.ctrlPort.Send(rsp)

	// Reset MSHR and directory state
	next.MSHRState = cache.MSHRState{}
	blockSize := 1 << spec.Log2BlockSize
	cache.DirectoryReset(
		&next.DirectoryState,
		spec.NumSets,
		spec.WayAssociativity,
		blockSize,
	)

	if next.ProcessingFlush.PauseAfter {
		next.CacheState = int(cacheStatePaused)
	} else {
		next.CacheState = int(cacheStateRunning)
	}

	tracing.TraceReqComplete(&next.ProcessingFlush.MsgMeta, f.pipeline.comp)
	next.HasProcessingFlush = false
	next.ProcessingFlush = flushReqState{}

	return true
}

func (f *flusher) flushCompleted() bool {
	next := f.pipeline.comp.GetNextState()

	for i := range next.DirToBankBufs {
		if next.DirToBankBufs[i].Size() > 0 {
			return false
		}
	}

	for i := range next.BankInflightTransCounts {
		if next.BankInflightTransCounts[i] > 0 {
			return false
		}
	}

	if next.WriteBufferBuf.Size() > 0 {
		return false
	}

	if len(next.InflightFetchIndices) > 0 ||
		len(next.InflightEvictionIndices) > 0 ||
		len(next.PendingEvictionIndices) > 0 {
		return false
	}

	return true
}
