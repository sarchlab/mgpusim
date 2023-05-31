package writeback

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v3/pipelining"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

type bankStage struct {
	cache  *Cache
	bankID int

	pipeline                   pipelining.Pipeline
	pipelineWidth              int
	postPipelineBuf            *bufferImpl
	inflightTransCount         int
	downwardInflightTransCount int // Count the trans that needs to be sent to the write buffer.
}

type bufferImpl struct {
	sim.HookableBase

	name     string
	capacity int
	elements []interface{}
}

func (b *bufferImpl) Name() string {
	return b.name
}

func (b *bufferImpl) CanPush() bool {
	return len(b.elements) < b.capacity
}

func (b *bufferImpl) Push(e interface{}) {
	if len(b.elements) >= b.capacity {
		log.Panic("buffer overflow")
	}
	b.elements = append(b.elements, e)

	if b.NumHooks() > 0 {
		b.InvokeHook(sim.HookCtx{
			Domain: b,
			Pos:    sim.HookPosBufPush,
			Item:   e,
			Detail: nil,
		})
	}
}

func (b *bufferImpl) Pop() interface{} {
	if len(b.elements) == 0 {
		return nil
	}

	e := b.elements[0]
	b.elements = b.elements[1:]

	if b.NumHooks() > 0 {
		b.InvokeHook(sim.HookCtx{
			Domain: b,
			Pos:    sim.HookPosBufPush,
			Item:   e,
			Detail: nil,
		})
	}

	return e
}

func (b *bufferImpl) Peek() interface{} {
	if len(b.elements) == 0 {
		return nil
	}

	return b.elements[0]
}

func (b *bufferImpl) Capacity() int {
	return b.capacity
}

func (b *bufferImpl) Size() int {
	return len(b.elements)
}

func (b *bufferImpl) Clear() {
	b.elements = nil
}

func (b *bufferImpl) Get(i int) interface{} {
	return b.elements[i]
}

func (b *bufferImpl) Remove(i int) {
	element := b.elements[i]

	b.elements = append(b.elements[:i], b.elements[i+1:]...)

	if b.NumHooks() > 0 {
		b.InvokeHook(sim.HookCtx{
			Domain: b,
			Pos:    sim.HookPosBufPush,
			Item:   element,
			Detail: nil,
		})
	}
}

type bankPipelineElem struct {
	trans *transaction
}

func (e bankPipelineElem) TaskID() string {
	return e.trans.req().Meta().ID + "_write_back_bank_pipeline"
}

func (s *bankStage) Tick(now sim.VTimeInSec) (madeProgress bool) {
	for i := 0; i < s.cache.numReqPerCycle; i++ {
		madeProgress = s.finalizeTrans(now) || madeProgress
	}

	madeProgress = s.pipeline.Tick(now) || madeProgress

	for i := 0; i < s.cache.numReqPerCycle; i++ {
		madeProgress = s.pullFromBuf(now) || madeProgress
	}

	return madeProgress
}

func (s *bankStage) Reset(now sim.VTimeInSec) {
	s.cache.dirToBankBuffers[s.bankID].Clear()
	s.pipeline.Clear()
	s.postPipelineBuf.Clear()
	s.inflightTransCount = 0
	// s.currentTrans = nil
}

func (s *bankStage) pullFromBuf(now sim.VTimeInSec) bool {
	if !s.pipeline.CanAccept() {
		return false
	}

	inBuf := s.cache.writeBufferToBankBuffers[s.bankID]
	trans := inBuf.Pop()
	if trans != nil {
		s.pipeline.Accept(now, bankPipelineElem{trans: trans.(*transaction)})
		s.inflightTransCount++
		return true
	}

	// Do not jam the writeBufferBuffer
	if !s.cache.writeBufferBuffer.CanPush() {
		return false
	}

	// Always reserve one lane for up-going transactions
	if s.downwardInflightTransCount >= s.pipelineWidth-1 {
		return false
	}

	inBuf = s.cache.dirToBankBuffers[s.bankID]
	trans = inBuf.Pop()
	if trans != nil {
		t := trans.(*transaction)

		if t.action == writeBufferFetch {
			s.cache.writeBufferBuffer.Push(trans)
			return true
		}

		s.pipeline.Accept(now, bankPipelineElem{trans: trans.(*transaction)})
		s.inflightTransCount++

		switch t.action {
		case bankEvict, bankEvictAndFetch, bankEvictAndWrite:
			s.downwardInflightTransCount++
		}

		return true
	}

	return false
}

func (s *bankStage) finalizeTrans(now sim.VTimeInSec) bool {
	for i := 0; i < s.postPipelineBuf.Size(); i++ {
		trans := s.postPipelineBuf.Get(i).(bankPipelineElem).trans

		done := false

		switch trans.action {
		case bankReadHit:
			done = s.finalizeReadHit(now, trans)
		case bankWriteHit:
			done = s.finalizeWriteHit(now, trans)
		case bankWriteFetched:
			done = s.finalizeBankWriteFetched(now, trans)
		case bankEvictAndFetch, bankEvictAndWrite, bankEvict:
			done = s.finalizeBankEviction(now, trans)
		default:
			panic("bank action not supported")
		}

		if done {
			s.postPipelineBuf.Remove(i)

			return true
		}
	}

	return false
}

func (s *bankStage) finalizeReadHit(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	if !s.cache.topSender.CanSend(1) {
		return false
	}

	read := trans.read
	addr := read.Address
	_, offset := getCacheLineID(addr, s.cache.log2BlockSize)
	block := trans.block

	data, err := s.cache.storage.Read(
		block.CacheAddress+offset, read.AccessByteSize)
	if err != nil {
		panic(err)
	}

	s.removeTransaction(now, trans)
	s.inflightTransCount--
	s.downwardInflightTransCount--
	block.ReadCount--

	dataReady := mem.DataReadyRspBuilder{}.
		WithSendTime(now).
		WithSrc(s.cache.topPort).
		WithDst(read.Src).
		WithRspTo(read.ID).
		WithData(data).
		Build()
	s.cache.topSender.Send(dataReady)

	tracing.TraceReqComplete(read, s.cache)

	// log.Printf("%.10f, %s, bank read hit finalize， %s, %04X, %04X, (%d, %d), %v\n",
	// 	now, s.cache.Name(),
	// 	trans.read.ID,
	// 	trans.read.Address, block.Tag,
	// 	block.SetID, block.WayID,
	// 	dataReady.Data,
	// )

	return true
}

func (s *bankStage) finalizeWriteHit(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	if !s.cache.topSender.CanSend(1) {
		return false
	}

	write := trans.write
	addr := write.Address
	_, offset := getCacheLineID(addr, s.cache.log2BlockSize)
	block := trans.block

	dirtyMask := s.writeData(block, write, offset)

	block.IsValid = true
	block.IsLocked = false
	block.IsDirty = true
	block.DirtyMask = dirtyMask

	s.removeTransaction(now, trans)
	s.inflightTransCount--
	s.downwardInflightTransCount--

	done := mem.WriteDoneRspBuilder{}.
		WithSendTime(now).
		WithSrc(s.cache.topPort).
		WithDst(write.Src).
		WithRspTo(write.ID).
		Build()
	s.cache.topSender.Send(done)

	tracing.TraceReqComplete(write, s.cache)

	// log.Printf("%.10f, %s, bank write hit finalize， %s, %04X, %04X, (%d, %d), %v\n",
	// 	now, s.cache.Name(),
	// 	trans.write.ID,
	// 	trans.write.Address, block.Tag,
	// 	block.SetID, block.WayID,
	// 	write.Data,
	// )

	return true
}

func (s *bankStage) writeData(
	block *cache.Block,
	write *mem.WriteReq,
	offset uint64,
) []bool {
	data, err := s.cache.storage.Read(
		block.CacheAddress, 1<<s.cache.log2BlockSize)
	if err != nil {
		panic(err)
	}

	dirtyMask := block.DirtyMask
	if dirtyMask == nil {
		dirtyMask = make([]bool, 1<<s.cache.log2BlockSize)
	}

	for i := 0; i < len(write.Data); i++ {
		if write.DirtyMask == nil || write.DirtyMask[i] {
			index := offset + uint64(i)
			data[index] = write.Data[i]
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
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	if !s.cache.mshrStageBuffer.CanPush() {
		return false
	}

	mshrEntry := trans.mshrEntry
	block := mshrEntry.Block
	s.cache.mshrStageBuffer.Push(mshrEntry)

	s.cache.storage.Write(block.CacheAddress, mshrEntry.Data)
	block.IsLocked = false
	block.IsValid = true

	s.inflightTransCount--

	// if trans.accessReq() != nil {
	// 	log.Printf("%.10f, %s, write fetched, %s, %04X, %04X, (%d, %d), %v\n",
	// 		now, s.cache.Name(),
	// 		trans.accessReq().Meta().ID,
	// 		trans.accessReq().GetAddress(), block.Tag,
	// 		block.SetID, block.WayID,
	// 		mshrEntry.Data,
	// 	)
	// }

	return true
}

func (s *bankStage) removeTransaction(now sim.VTimeInSec, trans *transaction) {
	for i, t := range s.cache.inFlightTransactions {
		if trans == t {
			// fmt.Printf("%.10f, %s, trans %s removed in bank stage.\n",
			// 	now, s.cache.Name(), t.id)
			s.cache.inFlightTransactions = append(
				(s.cache.inFlightTransactions)[:i],
				(s.cache.inFlightTransactions)[i+1:]...)
			return
		}
	}

	fmt.Printf("%.10f, %s, Transaction %s not found\n",
		now, s.cache.Name(), trans.id)

	panic("transaction not found")
}

func (s *bankStage) finalizeBankEviction(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	if !s.cache.writeBufferBuffer.CanPush() {
		return false
	}

	victim := trans.victim
	data, err := s.cache.storage.Read(
		victim.CacheAddress, 1<<s.cache.log2BlockSize)
	if err != nil {
		panic(err)
	}
	trans.evictingData = data

	switch trans.action {
	case bankEvict:
		trans.action = writeBufferFlush
	case bankEvictAndFetch:
		trans.action = writeBufferEvictAndFetch
	case bankEvictAndWrite:
		trans.action = writeBufferEvictAndWrite
	default:
		panic("unsupported action")
	}

	// if trans.accessReq() != nil {
	// 	log.Printf("%.10f, %s, bank read for eviction， %s, %04X, %04X, (%d, %d), %v\n",
	// 		now, s.cache.Name(),
	// 		trans.accessReq().Meta().ID,
	// 		trans.accessReq().GetAddress(), victim.Tag,
	// 		victim.SetID, victim.WayID,
	// 		data,
	// 	)
	// }

	delete(s.cache.evictingList, trans.evictingAddr)
	s.cache.writeBufferBuffer.Push(trans)
	s.inflightTransCount--
	s.downwardInflightTransCount--

	return true
}
