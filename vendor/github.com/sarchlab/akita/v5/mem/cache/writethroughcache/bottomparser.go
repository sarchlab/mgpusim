package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/tracing"
)

type bottomParser struct {
	cache *pipelineMW
}

func (p *bottomParser) Tick() bool {
	itemI := p.cache.bottomPort.PeekIncoming()
	if itemI == nil {
		return false
	}

	switch itemI.(type) {
	case *mem.WriteDoneRsp:
		return p.processDoneRsp(itemI)
	case *mem.DataReadyRsp:
		return p.processDataReady(itemI)
	default:
		panic("cannot process response")
	}
}

func (p *bottomParser) processDoneRsp(msg sim.Msg) bool {
	next := p.cache.comp.GetNextState()
	transIdx := p.findTransactionByWriteToBottomID(msg.Meta().RspTo)
	if transIdx < 0 {
		p.cache.bottomPort.RetrieveIncoming()
		return true
	}

	trans := &next.Transactions[transIdx]
	if trans.FetchAndWrite {
		p.cache.bottomPort.RetrieveIncoming()
		return true
	}

	trans.Done = true

	spec := p.cache.GetSpec()
	if needsDualCompletion(spec.WritePolicyType) {
		trans.BottomWriteDone = true

		if trans.BankDone {
			tracing.EndTask(trans.ID, p.cache.comp)
		}
	} else {
		tracing.EndTask(trans.ID, p.cache.comp)
	}

	p.cache.bottomPort.RetrieveIncoming()

	// Reconstruct writeToBottom for tracing
	writeToBottom := &mem.WriteReq{
		MsgMeta:   trans.WriteToBottomMeta,
		Data:      trans.WriteToBottomData,
		DirtyMask: trans.WriteToBottomDirtyMask,
		PID:       trans.WriteToBottomPID,
	}
	tracing.TraceReqFinalize(writeToBottom, p.cache.comp)

	return true
}

func (p *bottomParser) processDataReady(msg sim.Msg) bool {
	next := p.cache.comp.GetNextState()
	transIdx := p.findTransactionByReadToBottomID(msg.Meta().RspTo)
	if transIdx < 0 {
		p.cache.bottomPort.RetrieveIncoming()
		return true
	}

	trans := &next.Transactions[transIdx]

	bankBuf := p.getBankBuf(trans.BlockSetID, trans.BlockWayID)
	if !bankBuf.CanPush() {
		return false
	}

	pid := trans.ReadToBottomPID
	addr := trans.Address()
	spec := p.cache.GetSpec()
	cachelineID := (addr >> spec.Log2BlockSize) << spec.Log2BlockSize
	drMsg := msg.(*mem.DataReadyRsp)
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

	// Count how many bank slots are needed: 1 for fetcher + coalesced reads
	numBankSlots := 1
	for _, idx := range entryTransIdxs {
		if idx != transIdx && next.Transactions[idx].HasRead {
			numBankSlots++
		}
	}

	// Check bankBuf has enough free space for all slots
	if bankBuf.Cap-len(bankBuf.Elements) < numBankSlots {
		return false
	}

	// Set up trans for bank processing and push BEFORE any removals
	trans.BankAction = bankActionWriteFetched
	trans.Data = data
	trans.WriteFetchedDirtyMask = dirtyMask

	bankBuf.PushTyped(transIdx)

	// Finalize MSHR transactions (marks transactions as done, removes
	// from active processing). Skip the fetcher trans — it's been
	// pushed to the bank buffer and will be removed after bank processing.
	p.finalizeMSHRTransExcept(next, entryTransIdxs, blockTag, data, transIdx, bankBuf)
	cache.MSHRRemove(&next.MSHRState, pid, cachelineID)

	p.cache.bottomPort.RetrieveIncoming()

	// Reconstruct readToBottom for tracing
	readToBottom := &mem.ReadReq{
		MsgMeta: trans.ReadToBottomMeta,
		PID:     trans.ReadToBottomPID,
	}
	tracing.TraceReqFinalize(readToBottom, p.cache.comp)

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
	bankBuf *queueing.Buffer[int],
) {
	fetcherTrans := &next.Transactions[exceptIdx]

	for _, idx := range entryTransIdxs {
		trans := &next.Transactions[idx]

		if idx == exceptIdx {
			// The fetcher transaction — don't overwrite Data (the bank
			// stage needs the full block data) and don't mark Done yet.
			// The bank stage will restore the correct read slice and mark
			// Done after writing to storage.
		} else if trans.HasRead {
			offset := trans.ReadAddress - blockTag
			trans.Data = data[offset : offset+trans.ReadAccessByteSize]
			trans.BankAction = bankActionReadMissFill
			trans.MissFillDelay = p.cache.GetSpec().MissFillExtraLatency
			trans.BlockSetID = fetcherTrans.BlockSetID
			trans.BlockWayID = fetcherTrans.BlockWayID
			bankBuf.PushTyped(idx)
			// Do NOT mark Done or call tracing.EndTask — bank stage will do it
			continue
		} else {
			trans.Done = true
		}

		tracing.EndTask(trans.ID, p.cache.comp)
	}
}

func (p *bottomParser) findTransactionByWriteToBottomID(
	id uint64,
) int {
	next := p.cache.comp.GetNextState()

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
	next := p.cache.comp.GetNextState()

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
	next := p.cache.comp.GetNextState()
	numWaysPerSet := p.cache.GetSpec().WayAssociativity
	blockID := setID*numWaysPerSet + wayID
	bankID := blockID % len(next.BankBufs)

	return &next.BankBufs[bankID]
}
