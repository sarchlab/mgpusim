package writethroughcache

import (
	"github.com/sarchlab/akita/v5/tracing"
)

type bankStage struct {
	cache          *pipelineMW
	bankID         int
	numReqPerCycle int
}

func (s *bankStage) Reset() {
	next := &s.cache.comp.State
	next.BankPostBufs[s.bankID].Clear()
	next.BankPipelines[s.bankID].Clear()
}

func (s *bankStage) Tick() bool {
	madeProgress := false

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.finalizeTrans() || madeProgress
	}

	madeProgress = s.tickPipeline() || madeProgress

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.extractFromBuf() || madeProgress
	}

	return madeProgress
}

func (s *bankStage) tickPipeline() bool {
	next := &s.cache.comp.State
	bankPipeline := &next.BankPipelines[s.bankID]
	bankPostBuf := &next.BankPostBufs[s.bankID]

	return bankPipeline.Tick(bankPostBuf)
}

func (s *bankStage) extractFromBuf() bool {
	next := &s.cache.comp.State
	bankBuf := &next.BankBufs[s.bankID]

	if bankBuf.Size() == 0 {
		return false
	}

	bankPipeline := &next.BankPipelines[s.bankID]
	if !bankPipeline.CanAccept() {
		return false
	}

	transIdx := bankBuf.Pop()
	bankPipeline.Accept(transIdx)

	return true
}

func (s *bankStage) finalizeTrans() bool {
	next := &s.cache.comp.State
	bankPostBuf := &next.BankPostBufs[s.bankID]

	if bankPostBuf.Size() == 0 {
		return false
	}

	transIdx := bankPostBuf.Peek()
	trans := &next.Transactions[transIdx]

	switch trans.BankAction {
	case bankActionReadHit:
		return s.finalizeReadHitTrans(trans, transIdx)
	case bankActionWrite:
		return s.finalizeWriteTrans(trans, transIdx)
	case bankActionWriteFetched:
		return s.finalizeWriteFetchedTrans(trans, transIdx)
	default:
		panic("cannot handle trans bank action")
	}
}

func (s *bankStage) finalizeReadHitTrans(
	trans *transactionState, transIdx int,
) bool {
	next := &s.cache.comp.State
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]
	blockSize := uint64(1 << s.cache.comp.Spec().Log2BlockSize)

	data, err := s.cache.storage.Read(
		nextBlock.CacheAddress, blockSize)
	if err != nil {
		panic(err)
	}

	nextBlock.ReadCount--

	offset := trans.ReadAddress - nextBlock.Tag
	trans.Data = data[offset : offset+trans.ReadAccessByteSize]
	trans.Done = true

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Pop()

	tracing.EndTask(s.cache.comp, tracing.TaskEnd{ID: trans.ID})

	return true
}

func (s *bankStage) finalizeWriteTrans(
	trans *transactionState, transIdx int,
) bool {
	next := &s.cache.comp.State
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]
	blockSize := 1 << s.cache.comp.Spec().Log2BlockSize

	data, err := s.cache.storage.Read(nextBlock.CacheAddress, uint64(blockSize))
	if err != nil {
		panic(err)
	}

	offset := trans.WriteAddress - nextBlock.Tag

	for i := 0; i < len(trans.WriteData); i++ {
		if trans.WriteDirtyMask[i] {
			data[offset+uint64(i)] = trans.WriteData[i]
		}
	}

	err = s.cache.storage.Write(nextBlock.CacheAddress, data)
	if err != nil {
		panic(err)
	}

	nextBlock.DirtyMask = trans.WriteDirtyMask
	nextBlock.IsLocked = false

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Pop()

	trans.BankDone = true

	if !trans.Done && writeTransIsReady(trans) {
		trans.Done = true
		tracing.EndTask(s.cache.comp, tracing.TaskEnd{ID: trans.ID})
	}

	return true
}

func (s *bankStage) finalizeWriteFetchedTrans(
	trans *transactionState, transIdx int,
) bool {
	next := &s.cache.comp.State
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]

	err := s.cache.storage.Write(nextBlock.CacheAddress, trans.Data)
	if err != nil {
		panic(err)
	}

	nextBlock.DirtyMask = trans.WriteFetchedDirtyMask
	nextBlock.IsLocked = false

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Pop()

	// The merged line is now in storage. Any MSHR-coalesced write whose
	// data was folded into this fill can be considered "fill-done"; if
	// its bottom WriteDoneRsp has also arrived, finalize it here.
	s.completeMSHRFillWaiters(transIdx)

	if trans.HasRead {
		// Read fetcher — restore the correct read slice (Data was the
		// full block for writing to storage) and finalize.
		offset := trans.ReadAddress - nextBlock.Tag
		trans.Data = trans.Data[offset : offset+trans.ReadAccessByteSize]
		trans.Done = true
		tracing.EndTask(s.cache.comp, tracing.TaskEnd{ID: trans.ID})
		return true
	}

	// Write fetcher (partial-line write miss): wait for its own
	// bottom WriteDoneRsp before reporting completion upstream.
	trans.BankDone = true
	if !trans.Done && writeTransIsReady(trans) {
		trans.Done = true
		tracing.EndTask(s.cache.comp, tracing.TaskEnd{ID: trans.ID})
	}

	return true
}

func (s *bankStage) completeMSHRFillWaiters(fetcherIdx int) {
	next := &s.cache.comp.State

	for i := range next.Transactions {
		waiter := &next.Transactions[i]
		if waiter.Removed || waiter.Done {
			continue
		}
		if !waiter.WaitForMSHRFill || waiter.MSHRFillDone {
			continue
		}
		if waiter.MSHRFillFetcherIdx != fetcherIdx {
			continue
		}

		waiter.MSHRFillDone = true

		if writeTransIsReady(waiter) {
			waiter.Done = true
			tracing.EndTask(s.cache.comp, tracing.TaskEnd{ID: waiter.ID})
		}
	}
}
