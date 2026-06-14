package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/tracing"
)

type bottomParser struct {
	cache *pipelineMW
}

func (p *bottomParser) Tick() bool {
	itemI := p.cache.bottomPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	switch itemI.(type) {
	case memprotocol.WriteDoneRsp:
		return p.processDoneRsp(itemI)
	case memprotocol.DataReadyRsp:
		return p.processDataReady(itemI)
	default:
		panic("cannot process response")
	}
}

func (p *bottomParser) processDoneRsp(msg messaging.Msg) bool {
	next := &p.cache.comp.State
	transIdx := p.findTransactionByWriteToBottomID(msg.Meta().RspTo)
	if transIdx < 0 {
		p.cache.bottomPort().RetrieveIncoming()
		return true
	}

	trans := &next.Transactions[transIdx]
	trans.BottomWriteDone = true

	if !trans.Done && writeTransIsReady(trans) {
		trans.Done = true
		tracing.EndTask(p.cache.comp, tracing.TaskEnd{ID: trans.ID})
	}

	p.cache.bottomPort().RetrieveIncoming()

	// Reconstruct writeToBottom for tracing
	writeToBottom := memprotocol.WriteReq{
		MsgMeta:   trans.WriteToBottomMeta,
		Data:      trans.WriteToBottomData,
		DirtyMask: trans.WriteToBottomDirtyMask,
		PID:       trans.WriteToBottomPID,
	}
	tracing.TraceReqFinalize(p.cache.comp, writeToBottom)

	return true
}

func (p *bottomParser) processDataReady(msg messaging.Msg) bool {
	next := &p.cache.comp.State
	transIdx := p.findTransactionByReadToBottomID(msg.Meta().RspTo)
	if transIdx < 0 {
		p.cache.bottomPort().RetrieveIncoming()
		return true
	}

	trans := &next.Transactions[transIdx]

	bankBuf := p.getBankBuf(trans.BlockSetID, trans.BlockWayID)
	if !bankBuf.CanPush() {
		return false
	}

	pid := trans.ReadToBottomPID
	addr := trans.Address()
	spec := p.cache.comp.Spec()
	cachelineID := (addr >> spec.Log2BlockSize) << spec.Log2BlockSize
	drMsg := msg.(memprotocol.DataReadyRsp)
	data := drMsg.Data
	dirtyMask := make([]bool, 1<<spec.Log2BlockSize)

	entryIdx, found := cache.MSHRQuery(&next.MSHRState, pid, cachelineID)
	if !found {
		panic("MSHR entry not found for data ready response")
	}

	entry := &next.MSHRState.Entries[entryIdx]
	blockTag := next.DirectoryState.Sets[entry.BlockSetID].Blocks[entry.BlockWayID].Tag

	// Resolve entry transactions before any removals
	entryTransIdxs := append([]int(nil), entry.TransactionIndices...)
	p.mergeMSHRData(next, entryTransIdxs, blockTag, data, dirtyMask)

	// Set up trans for bank processing and push BEFORE any removals
	trans.BankAction = bankActionWriteFetched
	trans.Data = data
	trans.WriteFetchedDirtyMask = dirtyMask

	bankBuf.PushTyped(transIdx)

	// Finalize MSHR transactions (marks transactions as done, removes
	// from active processing). Skip the fetcher trans — it's been
	// pushed to the bank buffer and will be removed after bank processing.
	p.finalizeMSHRTransExcept(next, entryTransIdxs, blockTag, data, transIdx)
	cache.MSHRRemove(&next.MSHRState, pid, cachelineID)

	p.cache.bottomPort().RetrieveIncoming()

	// Reconstruct readToBottom for tracing
	readToBottom := memprotocol.ReadReq{
		MsgMeta: trans.ReadToBottomMeta,
		PID:     trans.ReadToBottomPID,
	}
	tracing.TraceReqFinalize(p.cache.comp, readToBottom)

	return true
}

func (p *bottomParser) mergeMSHRData(
	next *State,
	entryTransIdxs []int,
	blockTag uint64,
	data []byte,
	dirtyMask []bool,
) {
	for _, idx := range entryTransIdxs {
		trans := &next.Transactions[idx]
		if !trans.HasWrite {
			continue
		}

		offset := trans.WriteAddress - blockTag

		for i := 0; i < len(trans.WriteData); i++ {
			if trans.WriteDirtyMask[i] {
				data[offset+uint64(i)] = trans.WriteData[i]
				dirtyMask[offset+uint64(i)] = true
			}
		}
	}
}

func (p *bottomParser) finalizeMSHRTransExcept(
	next *State,
	entryTransIdxs []int,
	blockTag uint64,
	data []byte,
	exceptIdx int,
) {
	for _, idx := range entryTransIdxs {
		if idx == exceptIdx {
			// The fetcher transaction — bank stage finalizes it.
			continue
		}

		trans := &next.Transactions[idx]

		if trans.HasRead {
			offset := trans.ReadAddress - blockTag
			trans.Data = data[offset : offset+trans.ReadAccessByteSize]
			trans.Done = true
			tracing.EndTask(p.cache.comp, tracing.TaskEnd{ID: trans.ID})
			continue
		}

		// Coalesced write: completion now depends on its own bottom
		// WriteDoneRsp arriving. Only finalize if everything is already
		// satisfied (e.g. WriteDoneRsp landed first).
		if !trans.Done && writeTransIsReady(trans) {
			trans.Done = true
			tracing.EndTask(p.cache.comp, tracing.TaskEnd{ID: trans.ID})
		}
	}
}

func (p *bottomParser) findTransactionByWriteToBottomID(
	id uint64,
) int {
	next := &p.cache.comp.State

	for i := 0; i < len(next.Transactions); i++ {
		trans := &next.Transactions[i]
		if !trans.Removed && trans.HasWriteToBottom &&
			trans.WriteToBottomMeta.ID == id {
			return i
		}
	}

	return -1
}

func (p *bottomParser) findTransactionByReadToBottomID(
	id uint64,
) int {
	next := &p.cache.comp.State

	for i := 0; i < len(next.Transactions); i++ {
		trans := &next.Transactions[i]
		if !trans.Removed && trans.HasReadToBottom &&
			trans.ReadToBottomMeta.ID == id {
			return i
		}
	}

	return -1
}

func (p *bottomParser) getBankBuf(
	setID, wayID int,
) *queueing.Buffer[int] {
	next := &p.cache.comp.State
	numWaysPerSet := p.cache.comp.Spec().WayAssociativity
	blockID := setID*numWaysPerSet + wayID
	bankID := blockID % len(next.BankBufs)

	return &next.BankBufs[bankID]
}
