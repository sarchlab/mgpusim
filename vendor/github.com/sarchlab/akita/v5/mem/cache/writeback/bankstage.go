package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type bankStage struct {
	cache  *pipelineMW
	bankID int

	pipelineWidth int
}

func (s *bankStage) Tick() (madeProgress bool) {
	spec := s.cache.comp.GetSpec()

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = s.finalizeTrans() || madeProgress
	}

	madeProgress = s.tickPipeline() || madeProgress

	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = s.pullFromBuf() || madeProgress
	}

	return madeProgress
}

func (s *bankStage) tickPipeline() bool {
	next := s.cache.comp.GetNextState()
	bankPipeline := &next.BankPipelines[s.bankID]
	bankPostBuf := &next.BankPostPipelineBufs[s.bankID]
	return bankPipeline.Tick(bankPostBuf)
}

func (s *bankStage) Reset() {
	next := s.cache.comp.GetNextState()
	next.DirToBankBufs[s.bankID].Clear()
	next.BankPipelines[s.bankID].Stages = nil
	next.BankPostPipelineBufs[s.bankID].Clear()
	next.BankInflightTransCounts[s.bankID] = 0
}

func (s *bankStage) pullFromBuf() bool {
	next := s.cache.comp.GetNextState()
	spec := s.cache.comp.GetSpec()

	if !s.canAcceptIntoPipeline(*next) {
		return false
	}

	// Check write buffer to bank buffer first
	wbBuf := &next.WriteBufferToBankBufs[s.bankID]
	if len(wbBuf.Elements) > 0 {
		transIdx := wbBuf.Elements[0]
		wbBuf.Elements = wbBuf.Elements[1:]
		s.acceptIntoPipeline(next, spec, transIdx)
		next.BankInflightTransCounts[s.bankID]++
		return true
	}

	// Do not jam the writeBufferBuffer
	if !next.WriteBufferBuf.CanPush() {
		return false
	}

	// Always reserve one lane for up-going transactions
	if next.BankDownwardInflightTransCounts[s.bankID] >= s.pipelineWidth-1 {
		return false
	}

	return s.pullFromDirBuffer(next, spec)
}

func (s *bankStage) canAcceptIntoPipeline(cur State) bool {
	spec := s.cache.comp.GetSpec()

	if spec.BankLatency > 0 {
		return cur.BankPipelines[s.bankID].CanAccept()
	}

	// No pipeline - check post-buf capacity
	return cur.BankPostPipelineBufs[s.bankID].CanPush()
}

func (s *bankStage) pullFromDirBuffer(next *State, spec Spec) bool {
	dirBuf := &next.DirToBankBufs[s.bankID]
	if len(dirBuf.Elements) == 0 {
		return false
	}

	transIdx := dirBuf.Elements[0]
	dirBuf.Elements = dirBuf.Elements[1:]
	t := &next.Transactions[transIdx]

	if t.Action == writeBufferFetch {
		next.WriteBufferBuf.PushTyped(transIdx)
		return true
	}

	s.acceptIntoPipeline(next, spec, transIdx)
	next.BankInflightTransCounts[s.bankID]++

	switch t.Action {
	case bankEvict, bankEvictAndFetch, bankEvictAndWrite:
		next.BankDownwardInflightTransCounts[s.bankID]++
	}

	return true
}

func (s *bankStage) acceptIntoPipeline(next *State, spec Spec, transIdx int) {
	if spec.BankLatency > 0 {
		next.BankPipelines[s.bankID].Accept(transIdx)
	} else {
		// Bypass pipeline: put directly in post-pipeline buffer
		next.BankPostPipelineBufs[s.bankID].PushTyped(transIdx)
	}
}

func (s *bankStage) finalizeTrans() bool {
	next := s.cache.comp.GetNextState()
	postBuf := &next.BankPostPipelineBufs[s.bankID]

	for i, idx := range postBuf.Elements {
		trans := &next.Transactions[idx]

		done := false

		switch trans.Action {
		case bankReadHit:
			done = s.finalizeReadHit(idx, trans)
		case bankWriteHit:
			done = s.finalizeWriteHit(idx, trans)
		case bankWriteFetched:
			done = s.finalizeBankWriteFetched(idx, trans)
		case bankEvictAndFetch, bankEvictAndWrite, bankEvict:
			done = s.finalizeBankEviction(idx, trans)
		default:
			panic("bank action not supported")
		}

		if done {
			postBuf.Elements = append(postBuf.Elements[:i], postBuf.Elements[i+1:]...)
			return true
		}
	}

	return false
}

func (s *bankStage) finalizeReadHit(transIdx int, trans *transactionState) bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	spec := s.cache.comp.GetSpec()
	next := s.cache.comp.GetNextState()

	addr := trans.ReadAddress
	_, offset := getCacheLineID(addr, spec.Log2BlockSize)
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]

	data, err := s.cache.storage.Read(
		nextBlock.CacheAddress+offset, trans.ReadAccessByteSize)
	if err != nil {
		panic(err)
	}

	trans.Removed = true

	next.BankInflightTransCounts[s.bankID]--
	next.BankDownwardInflightTransCounts[s.bankID]--

	nextBlock.ReadCount--

	dataReady := &mem.DataReadyRsp{}
	dataReady.ID = sim.GetIDGenerator().Generate()
	dataReady.Src = s.cache.topPort.AsRemote()
	dataReady.Dst = trans.ReadMeta.Src
	dataReady.RspTo = trans.ReadMeta.ID
	dataReady.Data = data
	dataReady.TrafficBytes = len(data) + 4
	dataReady.TrafficClass = "mem.DataReadyRsp"
	s.cache.topPort.Send(dataReady)

	tracing.TraceReqComplete(&trans.ReadMeta, s.cache.comp)

	return true
}

func (s *bankStage) finalizeWriteHit(transIdx int, trans *transactionState) bool {
	if !s.cache.topPort.CanSend() {
		return false
	}

	spec := s.cache.comp.GetSpec()
	next := s.cache.comp.GetNextState()

	addr := trans.WriteAddress
	_, offset := getCacheLineID(addr, spec.Log2BlockSize)
	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]

	dirtyMask := s.writeData(nextBlock, trans, offset, spec.Log2BlockSize)

	nextBlock.IsValid = true
	nextBlock.IsLocked = false
	nextBlock.IsDirty = true
	nextBlock.DirtyMask = dirtyMask

	trans.Removed = true

	next.BankInflightTransCounts[s.bankID]--
	next.BankDownwardInflightTransCounts[s.bankID]--

	done := &mem.WriteDoneRsp{}
	done.ID = sim.GetIDGenerator().Generate()
	done.Src = s.cache.topPort.AsRemote()
	done.Dst = trans.WriteMeta.Src
	done.RspTo = trans.WriteMeta.ID
	done.TrafficBytes = 4
	done.TrafficClass = "mem.WriteDoneRsp"
	s.cache.topPort.Send(done)

	tracing.TraceReqComplete(&trans.WriteMeta, s.cache.comp)

	return true
}

func (s *bankStage) writeData(
	block *cache.BlockState,
	trans *transactionState,
	offset uint64,
	log2BlockSize uint64,
) []bool {
	data, err := s.cache.storage.Read(
		block.CacheAddress, 1<<log2BlockSize)
	if err != nil {
		panic(err)
	}

	dirtyMask := block.DirtyMask
	if dirtyMask == nil {
		dirtyMask = make([]bool, 1<<log2BlockSize)
	}

	for i := 0; i < len(trans.WriteData); i++ {
		if trans.WriteDirtyMask == nil || trans.WriteDirtyMask[i] {
			index := offset + uint64(i)
			data[index] = trans.WriteData[i]
			dirtyMask[index] = true
		}
	}

	err = s.cache.storage.Write(block.CacheAddress, data)
	if err != nil {
		panic(err)
	}

	return dirtyMask
}

func (s *bankStage) finalizeBankWriteFetched(
	transIdx int,
	trans *transactionState,
) bool {
	next := s.cache.comp.GetNextState()
	mshrBuf := &next.MSHRStageBuf

	if !mshrBuf.CanPush() {
		return false
	}

	nextBlock := &next.DirectoryState.Sets[trans.BlockSetID].Blocks[trans.BlockWayID]

	mshrBuf.PushTyped(transIdx)

	err := s.cache.storage.Write(nextBlock.CacheAddress, trans.MSHRData)
	if err != nil {
		panic(err)
	}
	nextBlock.IsLocked = false
	nextBlock.IsValid = true

	next.BankInflightTransCounts[s.bankID]--

	return true
}

func (s *bankStage) finalizeBankEviction(
	transIdx int,
	trans *transactionState,
) bool {
	spec := s.cache.comp.GetSpec()
	next := s.cache.comp.GetNextState()
	wbBuf := &next.WriteBufferBuf

	if !wbBuf.CanPush() {
		return false
	}

	data, err := s.cache.storage.Read(
		trans.VictimCacheAddress, 1<<spec.Log2BlockSize)
	if err != nil {
		panic(err)
	}

	trans.EvictingData = data

	switch trans.Action {
	case bankEvict:
		trans.Action = writeBufferFlush
	case bankEvictAndFetch:
		trans.Action = writeBufferEvictAndFetch
	case bankEvictAndWrite:
		trans.Action = writeBufferEvictAndWrite
	default:
		panic("unsupported action")
	}

	delete(next.EvictingList, trans.EvictingAddr)

	wbBuf.PushTyped(transIdx)

	next.BankInflightTransCounts[s.bankID]--
	next.BankDownwardInflightTransCounts[s.bankID]--

	return true
}

