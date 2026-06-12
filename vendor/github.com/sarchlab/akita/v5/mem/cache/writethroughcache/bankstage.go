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
	next := s.cache.comp.GetNextState()
	next.BankPostBufs[s.bankID].Elements = nil
	next.BankPipelines[s.bankID].Stages = nil
}

func (s *bankStage) Tick() bool {
	madeProgress := false

	madeProgress = s.countDownMissFillDelay() || madeProgress

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.finalizeTrans() || madeProgress
	}

	madeProgress = s.tickPipeline() || madeProgress

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.extractFromBuf() || madeProgress
	}

	return madeProgress
}

// countDownMissFillDelay decrements the miss-fill delay of the head
// transaction in the bank post buffer, once per tick.
func (s *bankStage) countDownMissFillDelay() bool {
	next := s.cache.comp.GetNextState()
	bankPostBuf := &next.BankPostBufs[s.bankID]
	if bankPostBuf.Size() == 0 {
		return false
	}
	transIdx := bankPostBuf.Elements[0]
	trans := &next.Transactions[transIdx]
	if trans.BankAction == bankActionReadMissFill && trans.MissFillDelay > 0 {
		trans.MissFillDelay--
		return true
	}
	return false
}

func (s *bankStage) tickPipeline() bool {
	next := s.cache.comp.GetNextState()
	bankPipeline := &next.BankPipelines[s.bankID]
	bankPostBuf := &next.BankPostBufs[s.bankID]

	return bankPipeline.Tick(bankPostBuf)
}

func (s *bankStage) extractFromBuf() bool {
	next := s.cache.comp.GetNextState()
	bankBuf := &next.BankBufs[s.bankID]

	if bankBuf.Size() == 0 {
		return false
	}

	transIdx := bankBuf.Elements[0]
	trans := &next.Transactions[transIdx]

	// Miss-fill transactions have already paid the L2 latency. Routing them
	// through the bank pipeline a second time would double-count bank latency,
	// so they bypass the pipeline and go straight to the post-buffer where the
	// miss-fill delay countdown is performed.
	if trans.BankAction == bankActionReadMissFill {
		bankPostBuf := &next.BankPostBufs[s.bankID]
		if !bankPostBuf.CanPush() {
			return false
		}
		bankPostBuf.PushTyped(transIdx)
		bankBuf.Elements = bankBuf.Elements[1:]
		return true
	}

	bankPipeline := &next.BankPipelines[s.bankID]
	if !bankPipeline.CanAccept() {
		return false
	}

	bankPipeline.Accept(transIdx)
	bankBuf.Elements = bankBuf.Elements[1:]

	return true
}

func (s *bankStage) finalizeTrans() bool {
	next := s.cache.comp.GetNextState()
	bankPostBuf := &next.BankPostBufs[s.bankID]

	if bankPostBuf.Size() == 0 {
		return false
	}

	transIdx := bankPostBuf.Elements[0]
	trans := &next.Transactions[transIdx]

	switch trans.BankAction {
	case bankActionReadHit:
		return s.finalizeReadHitTrans(trans, transIdx)
	case bankActionWrite:
		return s.finalizeWriteTrans(trans, transIdx)
	case bankActionWriteFetched:
		return s.finalizeWriteFetchedTrans(trans)
	case bankActionReadMissFill:
		return s.finalizeReadMissFillTrans(trans)
	default:
		panic("cannot handle trans bank action")
	}
}

func (s *bankStage) finalizeReadHitTrans(
	trans *transactionState, transIdx int,
) bool {
	next := s.cache.comp.GetNextState()
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]
	blockSize := uint64(1 << s.cache.GetSpec().Log2BlockSize)

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
	bankPostBuf.Elements = bankPostBuf.Elements[1:]

	tracing.EndTask(trans.ID, s.cache.comp)

	return true
}

func (s *bankStage) finalizeWriteTrans(
	trans *transactionState, transIdx int,
) bool {
	next := s.cache.comp.GetNextState()
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]
	blockSize := 1 << s.cache.GetSpec().Log2BlockSize

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
	bankPostBuf.Elements = bankPostBuf.Elements[1:]

	spec := s.cache.GetSpec()
	if needsDualCompletion(spec.WritePolicyType) {
		trans.BankDone = true

		if trans.BottomWriteDone {
			tracing.EndTask(trans.ID, s.cache.comp)
		}
	} else {
		tracing.EndTask(trans.ID, s.cache.comp)
	}

	return true
}

func (s *bankStage) finalizeReadMissFillTrans(trans *transactionState) bool {
	if trans.MissFillDelay > 0 {
		return false // still waiting — delay counted down by countDownMissFillDelay
	}
	next := s.cache.comp.GetNextState()
	trans.Done = true
	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Elements = bankPostBuf.Elements[1:]
	tracing.EndTask(trans.ID, s.cache.comp)
	return true
}

func (s *bankStage) finalizeWriteFetchedTrans(trans *transactionState) bool {
	next := s.cache.comp.GetNextState()
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]

	err := s.cache.storage.Write(nextBlock.CacheAddress, trans.Data)
	if err != nil {
		panic(err)
	}

	nextBlock.DirtyMask = trans.WriteFetchedDirtyMask
	nextBlock.IsLocked = false

	// If this transaction was a read, restore the correct read data slice
	// (Data was temporarily set to the full block for writing to storage)
	// and mark Done so the respond stage picks it up.
	if trans.HasRead {
		offset := trans.ReadAddress - nextBlock.Tag
		trans.Data = trans.Data[offset : offset+trans.ReadAccessByteSize]
	}

	// Mark transaction Done — the respond stage will send the response.
	trans.Done = true

	bankPostBuf := &next.BankPostBufs[s.bankID]
	bankPostBuf.Elements = bankPostBuf.Elements[1:]

	return true
}
