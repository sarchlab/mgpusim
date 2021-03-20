package l1v

import (
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/util/v2/buffering"
	"gitlab.com/akita/util/v2/pipelining"
	"gitlab.com/akita/util/v2/tracing"
)

type bankTransaction struct {
	*transaction
}

func (t *bankTransaction) TaskID() string {
	return t.transaction.id
}

type bankStage struct {
	cache          *Cache
	bankID         int
	numReqPerCycle int

	pipeline        pipelining.Pipeline
	postPipelineBuf buffering.Buffer
}

func (s *bankStage) Reset() {
	s.postPipelineBuf.Clear()
	s.pipeline.Clear()
}

func (s *bankStage) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.finalizeTrans(now) || madeProgress
	}

	madeProgress = s.pipeline.Tick(now) || madeProgress

	for i := 0; i < s.numReqPerCycle; i++ {
		madeProgress = s.extractFromBuf(now) || madeProgress
	}

	return madeProgress
}

func (s *bankStage) extractFromBuf(now sim.VTimeInSec) bool {
	item := s.cache.bankBufs[s.bankID].Peek()
	if item == nil {
		return false
	}

	if !s.pipeline.CanAccept() {
		return false
	}

	s.pipeline.Accept(now, &bankTransaction{
		transaction: item.(*transaction),
	})
	s.cache.bankBufs[s.bankID].Pop()
	return true
}

func (s *bankStage) finalizeTrans(now sim.VTimeInSec) bool {
	item := s.postPipelineBuf.Peek()
	if item == nil {
		return false
	}

	trans := item.(*bankTransaction).transaction

	switch trans.bankAction {
	case bankActionReadHit:
		return s.finalizeReadHitTrans(now, trans)
	case bankActionWrite:
		return s.finalizeWriteTrans(now, trans)
	case bankActionWriteFetched:
		return s.finalizeWriteFetchedTrans(now, trans)
	default:
		panic("cannot handle trans bank action")
	}
}

func (s *bankStage) finalizeReadHitTrans(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	block := trans.block

	data, err := s.cache.storage.Read(
		block.CacheAddress, trans.read.AccessByteSize)
	if err != nil {
		panic(err)
	}
	block.ReadCount--

	for _, t := range trans.preCoalesceTransactions {
		offset := t.read.Address - block.Tag
		t.data = data[offset : offset+t.read.AccessByteSize]
		t.done = true
	}

	s.removeTransaction(trans)
	s.postPipelineBuf.Pop()

	tracing.EndTask(trans.id, now, s.cache)
	return true
}

func (s *bankStage) finalizeWriteTrans(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	block := trans.block
	blockSize := 1 << s.cache.log2BlockSize

	data, err := s.cache.storage.Read(block.CacheAddress, uint64(blockSize))

	offset := write.Address - block.Tag
	for i := 0; i < len(write.Data); i++ {
		if write.DirtyMask[i] {
			data[offset+uint64(i)] = write.Data[i]
		}
	}

	err = s.cache.storage.Write(block.CacheAddress, data)
	if err != nil {
		panic(err)
	}
	block.DirtyMask = write.DirtyMask
	block.IsLocked = false

	s.postPipelineBuf.Pop()

	tracing.EndTask(trans.id, now, s.cache)
	return true
}

func (s *bankStage) finalizeWriteFetchedTrans(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	block := trans.block

	err := s.cache.storage.Write(block.CacheAddress, trans.data)
	if err != nil {
		panic(err)
	}

	block.DirtyMask = trans.writeFetchedDirtyMask
	block.IsLocked = false

	s.postPipelineBuf.Pop()

	return true
}

func (s *bankStage) removeTransaction(trans *transaction) {
	for i, t := range s.cache.postCoalesceTransactions {
		if t == trans {
			s.cache.postCoalesceTransactions = append(
				s.cache.postCoalesceTransactions[:i],
				s.cache.postCoalesceTransactions[i+1:]...)
			return
		}
	}
}
