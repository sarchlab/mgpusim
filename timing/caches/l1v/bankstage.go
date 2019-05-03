package l1v

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/util"
)

type bankStage struct {
	name              string
	inBuf             util.Buffer
	storage           *mem.Storage
	postCTransactions *[]*transaction
	latency           int
	log2BlockSize     uint64

	cycleLeft int
	currTrans *transaction
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
	item := s.inBuf.Peek()
	if item == nil {
		return false
	}

	s.currTrans = item.(*transaction)
	s.cycleLeft = s.latency
	s.inBuf.Pop()
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

	data, err := s.storage.Read(block.CacheAddress, trans.read.MemByteSize)
	if err != nil {
		panic(err)
	}
	block.ReadCount--

	for _, t := range trans.preCoalesceTransactions {
		offset := t.read.Address - block.Tag
		t.data = data[offset : offset+t.read.MemByteSize]
		t.done = true
	}

	s.removeTransaction(s.currTrans)
	s.currTrans = nil
	return true
}

func (s *bankStage) finalizeWriteTrans(now akita.VTimeInSec) bool {
	trans := s.currTrans
	write := trans.write
	block := trans.block
	blockSize := 1 << s.log2BlockSize

	data, err := s.storage.Read(block.CacheAddress, uint64(blockSize))

	offset := write.Address - block.Tag
	for i := 0; i < len(write.Data); i++ {
		if write.DirtyMask[i] {
			data[offset+uint64(i)] = write.Data[i]
		}
	}

	err = s.storage.Write(block.CacheAddress, data)
	if err != nil {
		panic(err)
	}
	block.DirtyMask = write.DirtyMask
	block.IsLocked = false

	s.currTrans = nil
	return true
}

func (s *bankStage) finalizeWriteFetchedTrans(now akita.VTimeInSec) bool {
	trans := s.currTrans
	block := trans.block

	err := s.storage.Write(block.CacheAddress, trans.data)
	if err != nil {
		panic(err)
	}

	block.DirtyMask = trans.writeFetchedDirtyMask
	block.IsLocked = false

	s.currTrans = nil

	trace(now, s.name, "write fetched", block.Tag, trans.data)
	return true
}

func (s *bankStage) removeTransaction(trans *transaction) {
	for i, t := range *s.postCTransactions {
		if t == trans {
			trace(0, s.name, fmt.Sprintf("remove trans %p", trans), 0, nil)
			*s.postCTransactions = append(
				(*s.postCTransactions)[:i],
				(*s.postCTransactions)[i+1:]...)
			return
		}
	}
}
