package l1v

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/util/tracing"
)

type bankStage struct {
	cache  *Cache
	bankID int

	cycleLeft int
	currTrans *transaction
}

func (s *bankStage) Reset() {
	s.cycleLeft = 0
	s.currTrans = nil
}

func (s *bankStage) Tick(now akita.VTimeInSec) bool {
	if s.currTrans != nil {
		s.cycleLeft--

		if s.cycleLeft < 0 {
			return s.finalizeTrans(now)
		}

		return true
	}
	return s.extractFromBuf()
}

func (s *bankStage) extractFromBuf() bool {
	item := s.cache.bankBufs[s.bankID].Peek()
	if item == nil {
		return false
	}

	s.currTrans = item.(*transaction)
	s.cycleLeft = s.cache.bankLatency
	s.cache.bankBufs[s.bankID].Pop()
	return true
}

func (s *bankStage) finalizeTrans(now akita.VTimeInSec) bool {
	switch s.currTrans.bankAction {
	case bankActionReadHit:
		return s.finalizeReadHitTrans(now)
	case bankActionWrite:
		return s.finalizeWriteTrans(now)
	case bankActionWriteFetched:
		return s.finalizeWriteFetchedTrans(now)
	default:
		panic("cannot handle trans bank action")
	}
}

func (s *bankStage) finalizeReadHitTrans(now akita.VTimeInSec) bool {
	trans := s.currTrans
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

	s.removeTransaction(s.currTrans)
	s.currTrans = nil

	tracing.EndTask(trans.id, now, s.cache)
	return true
}

func (s *bankStage) finalizeWriteTrans(now akita.VTimeInSec) bool {
	trans := s.currTrans
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

	s.currTrans = nil

	tracing.EndTask(trans.id, now, s.cache)
	return true
}

func (s *bankStage) finalizeWriteFetchedTrans(now akita.VTimeInSec) bool {
	trans := s.currTrans
	block := trans.block

	err := s.cache.storage.Write(block.CacheAddress, trans.data)
	if err != nil {
		panic(err)
	}

	block.DirtyMask = trans.writeFetchedDirtyMask
	block.IsLocked = false

	s.currTrans = nil

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
