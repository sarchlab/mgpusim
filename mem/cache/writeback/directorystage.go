package writeback

import (
	"fmt"

	"github.com/sarchlab/akita/v3/pipelining"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
	"github.com/sarchlab/mgpusim/v3/mem/vm"
)

type dirPipelineItem struct {
	trans *transaction
}

func (i dirPipelineItem) TaskID() string {
	return i.trans.id + "_dir_pipeline"
}

type directoryStage struct {
	cache    *Cache
	pipeline pipelining.Pipeline
	buf      sim.Buffer
}

func (ds *directoryStage) Tick(now sim.VTimeInSec) (madeProgress bool) {
	madeProgress = ds.acceptNewTransaction(now) || madeProgress

	madeProgress = ds.pipeline.Tick(now) || madeProgress

	madeProgress = ds.processTransaction(now) || madeProgress

	return madeProgress
}

func (ds *directoryStage) processTransaction(
	now sim.VTimeInSec,
) bool {
	madeProgress := false

	for i := 0; i < ds.cache.numReqPerCycle; i++ {
		item := ds.buf.Peek()
		if item == nil {
			break
		}

		trans := item.(dirPipelineItem).trans

		addr := trans.accessReq().GetAddress()
		cacheLineID, _ := getCacheLineID(addr, ds.cache.log2BlockSize)
		if _, evicting := ds.cache.evictingList[cacheLineID]; evicting {
			break
		}

		if trans.read != nil {
			madeProgress = ds.doRead(now, trans) || madeProgress
			continue
		}

		madeProgress = ds.doWrite(now, trans) || madeProgress
	}

	return madeProgress
}

func (ds *directoryStage) acceptNewTransaction(now sim.VTimeInSec) bool {
	madeProgress := false

	for i := 0; i < ds.cache.numReqPerCycle; i++ {
		if !ds.pipeline.CanAccept() {
			break
		}

		item := ds.cache.dirStageBuffer.Peek()
		if item == nil {
			break
		}

		trans := item.(*transaction)
		ds.pipeline.Accept(now, dirPipelineItem{trans})
		ds.cache.dirStageBuffer.Pop()

		madeProgress = true
	}

	return madeProgress
}

func (ds *directoryStage) Reset(now sim.VTimeInSec) {
	ds.pipeline.Clear()
	ds.buf.Clear()
	ds.cache.dirStageBuffer.Clear()
}

func (ds *directoryStage) doRead(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	cachelineID, _ := getCacheLineID(
		trans.read.Address, ds.cache.log2BlockSize)

	mshrEntry := ds.cache.mshr.Query(trans.read.PID, cachelineID)
	if mshrEntry != nil {
		return ds.handleReadMSHRHit(now, trans, mshrEntry)
	}

	block := ds.cache.directory.Lookup(
		trans.read.PID, cachelineID)
	if block != nil {
		return ds.handleReadHit(now, trans, block)
	}

	return ds.handleReadMiss(now, trans)
}

func (ds *directoryStage) handleReadMSHRHit(
	now sim.VTimeInSec,
	trans *transaction,
	mshrEntry *cache.MSHREntry,
) bool {
	trans.mshrEntry = mshrEntry
	mshrEntry.Requests = append(mshrEntry.Requests, trans)
	ds.buf.Pop()

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(trans.read, ds.cache),
		ds.cache,
		"read-mshr-hit",
	)

	return true
}

func (ds *directoryStage) handleReadHit(
	now sim.VTimeInSec,
	trans *transaction,
	block *cache.Block,
) bool {
	if block.IsLocked {
		return false
	}

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(trans.read, ds.cache),
		ds.cache,
		"read-hit",
	)

	// log.Printf("%.10f, %s, dir read hit， %s, %04X, %04X, (%d, %d), %v\n",
	// 	now, ds.cache.Name(),
	// 	trans.read.ID,
	// 	trans.read.Address,
	// 	(trans.read.GetAddress()>>ds.cache.log2BlockSize)<<ds.cache.log2BlockSize,
	// 	block.SetID, block.WayID,
	// 	nil,
	// )

	return ds.readFromBank(trans, block)
}

func (ds *directoryStage) handleReadMiss(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	req := trans.read
	cacheLineID, _ := getCacheLineID(req.Address, ds.cache.log2BlockSize)

	if ds.cache.mshr.IsFull() {
		return false
	}

	victim := ds.cache.directory.FindVictim(cacheLineID)
	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	// log.Printf("%.10f, %s, dir read miss， %s, %04X, %04X, (%d, %d), %v\n",
	// 	now, ds.cache.Name(),
	// 	trans.read.ID,
	// 	trans.read.Address,
	// 	(trans.read.GetAddress()>>ds.cache.log2BlockSize)<<ds.cache.log2BlockSize,
	// 	victim.SetID, victim.WayID,
	// 	nil,
	// )

	if ds.needEviction(victim) {
		ok := ds.evict(now, trans, victim)
		if ok {
			tracing.AddTaskStep(
				tracing.MsgIDAtReceiver(trans.read, ds.cache),
				ds.cache,
				"read-miss",
			)
		}

		return ok
	}

	ok := ds.fetch(now, trans, victim)
	if ok {
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(trans.read, ds.cache),
			ds.cache,
			"read-miss",
		)
	}

	return ok
}

func (ds *directoryStage) doWrite(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	cachelineID, _ := getCacheLineID(write.Address, ds.cache.log2BlockSize)

	mshrEntry := ds.cache.mshr.Query(write.PID, cachelineID)
	if mshrEntry != nil {
		ok := ds.doWriteMSHRHit(now, trans, mshrEntry)
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(trans.write, ds.cache),
			ds.cache,
			"write-mshr-hit",
		)

		return ok
	}

	block := ds.cache.directory.Lookup(trans.write.PID, cachelineID)
	if block != nil {
		ok := ds.doWriteHit(trans, block)
		if ok {
			tracing.AddTaskStep(
				tracing.MsgIDAtReceiver(trans.write, ds.cache),
				ds.cache,
				"write-hit",
			)
		}

		return ok
	}

	ok := ds.doWriteMiss(now, trans)
	if ok {
		tracing.AddTaskStep(
			tracing.MsgIDAtReceiver(trans.write, ds.cache),
			ds.cache,
			"write-miss",
		)
	}

	return ok
}

func (ds *directoryStage) doWriteMSHRHit(
	now sim.VTimeInSec,
	trans *transaction,
	mshrEntry *cache.MSHREntry,
) bool {
	trans.mshrEntry = mshrEntry
	mshrEntry.Requests = append(mshrEntry.Requests, trans)
	ds.buf.Pop()

	return true
}

func (ds *directoryStage) doWriteHit(
	trans *transaction,
	block *cache.Block,
) bool {
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	return ds.writeToBank(trans, block)
}

func (ds *directoryStage) doWriteMiss(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write

	if ds.isWritingFullLine(write) {
		return ds.writeFullLineMiss(now, trans)
	}
	return ds.writePartialLineMiss(now, trans)
}

func (ds *directoryStage) writeFullLineMiss(now sim.VTimeInSec, trans *transaction) bool {
	write := trans.write
	cachelineID, _ := getCacheLineID(write.Address, ds.cache.log2BlockSize)

	victim := ds.cache.directory.FindVictim(cachelineID)
	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if ds.needEviction(victim) {
		return ds.evict(now, trans, victim)
	}

	return ds.writeToBank(trans, victim)
}

func (ds *directoryStage) writePartialLineMiss(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	cachelineID, _ := getCacheLineID(write.Address, ds.cache.log2BlockSize)

	if ds.cache.mshr.IsFull() {
		return false
	}

	victim := ds.cache.directory.FindVictim(cachelineID)
	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	// log.Printf("%.10f, %s, write partial line ， %s, %04X, %04X, (%d, %d), %v\n",
	// 	now, ds.cache.Name(),
	// 	trans.write.ID,
	// 	trans.write.Address, cachelineID,
	// 	victim.SetID, victim.WayID,
	// 	write.Data,
	// )

	if ds.needEviction(victim) {
		return ds.evict(now, trans, victim)
	}

	return ds.fetch(now, trans, victim)
}

func (ds *directoryStage) readFromBank(
	trans *transaction,
	block *cache.Block,
) bool {
	numBanks := len(ds.cache.dirToBankBuffers)
	bank := bankID(block, ds.cache.directory.WayAssociativity(), numBanks)
	bankBuf := ds.cache.dirToBankBuffers[bank]

	if !bankBuf.CanPush() {
		return false
	}

	ds.cache.directory.Visit(block)
	block.ReadCount++
	trans.block = block
	trans.action = bankReadHit
	ds.buf.Pop()
	bankBuf.Push(trans)
	return true
}

func (ds *directoryStage) writeToBank(
	trans *transaction,
	block *cache.Block,
) bool {
	numBanks := len(ds.cache.dirToBankBuffers)
	bank := bankID(block, ds.cache.directory.WayAssociativity(), numBanks)
	bankBuf := ds.cache.dirToBankBuffers[bank]

	if !bankBuf.CanPush() {
		return false
	}

	addr := trans.write.Address
	cachelineID, _ := getCacheLineID(addr, ds.cache.log2BlockSize)

	ds.cache.directory.Visit(block)
	block.IsLocked = true
	block.Tag = cachelineID
	block.IsValid = true
	block.PID = trans.write.PID
	trans.block = block
	trans.action = bankWriteHit
	ds.buf.Pop()
	bankBuf.Push(trans)

	return true
}

func (ds *directoryStage) evict(
	now sim.VTimeInSec,
	trans *transaction,
	victim *cache.Block,
) bool {
	bankNum := bankID(victim,
		ds.cache.directory.WayAssociativity(), len(ds.cache.dirToBankBuffers))
	bankBuf := ds.cache.dirToBankBuffers[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	var addr uint64
	var pid vm.PID
	if trans.read != nil {
		addr = trans.read.Address
		pid = trans.read.PID
	} else {
		addr = trans.write.Address
		pid = trans.write.PID
	}

	cacheLineID, _ := getCacheLineID(addr, ds.cache.log2BlockSize)

	ds.updateTransForEviction(trans, victim, pid, cacheLineID)
	ds.updateVictimBlockMetaData(victim, cacheLineID, pid)

	ds.buf.Pop()
	bankBuf.Push(trans)
	ds.cache.evictingList[trans.victim.Tag] = true

	// log.Printf("%.10f, %s, directory evict ， %s, %04X, %04X, (%d, %d), %v\n",
	// 	now, ds.cache.Name(),
	// 	trans.accessReq().Meta().ID,
	// 	trans.accessReq().GetAddress(), trans.victim.Tag,
	// 	victim.SetID, victim.WayID,
	// 	nil,
	// )

	return true
}

func (ds *directoryStage) updateVictimBlockMetaData(victim *cache.Block, cacheLineID uint64, pid vm.PID) {
	victim.Tag = cacheLineID
	victim.PID = pid
	victim.IsLocked = true
	victim.IsDirty = false
	ds.cache.directory.Visit(victim)
}

func (ds *directoryStage) updateTransForEviction(
	trans *transaction,
	victim *cache.Block,
	pid vm.PID,
	cacheLineID uint64,
) {
	trans.action = bankEvictAndFetch
	trans.victim = &cache.Block{
		PID:          victim.PID,
		Tag:          victim.Tag,
		CacheAddress: victim.CacheAddress,
		DirtyMask:    victim.DirtyMask,
	}
	trans.block = victim
	trans.evictingPID = trans.victim.PID
	trans.evictingAddr = trans.victim.Tag
	trans.evictingDirtyMask = victim.DirtyMask

	if ds.evictionNeedFetch(trans) {
		mshrEntry := ds.cache.mshr.Add(pid, cacheLineID)
		mshrEntry.Block = victim
		mshrEntry.Requests = append(mshrEntry.Requests, trans)
		trans.mshrEntry = mshrEntry
		trans.fetchPID = pid
		trans.fetchAddress = cacheLineID
		trans.action = bankEvictAndFetch
	} else {
		trans.action = bankEvictAndWrite
	}
}

func (ds *directoryStage) evictionNeedFetch(t *transaction) bool {
	if t.write == nil {
		return true
	}

	if ds.isWritingFullLine(t.write) {
		return false
	}

	return true
}

func (ds *directoryStage) fetch(
	now sim.VTimeInSec,
	trans *transaction,
	block *cache.Block,
) bool {
	var addr uint64
	var pid vm.PID
	var req mem.AccessReq
	if trans.read != nil {
		req = trans.read
		addr = trans.read.Address
		pid = trans.read.PID
	} else {
		req = trans.write
		addr = trans.write.Address
		pid = trans.write.PID
	}
	cacheLineID, _ := getCacheLineID(addr, ds.cache.log2BlockSize)

	bankNum := bankID(block,
		ds.cache.directory.WayAssociativity(), len(ds.cache.dirToBankBuffers))
	bankBuf := ds.cache.dirToBankBuffers[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	mshrEntry := ds.cache.mshr.Add(pid, cacheLineID)
	trans.mshrEntry = mshrEntry
	trans.block = block
	block.IsLocked = true
	block.Tag = cacheLineID
	block.PID = pid
	block.IsValid = true
	ds.cache.directory.Visit(block)

	tracing.AddTaskStep(
		tracing.MsgIDAtReceiver(req, ds.cache),
		ds.cache,
		fmt.Sprintf("add-mshr-entry-0x%x-0x%x", mshrEntry.Address, block.Tag),
	)

	ds.buf.Pop()

	trans.action = writeBufferFetch
	trans.fetchPID = pid
	trans.fetchAddress = cacheLineID
	bankBuf.Push(trans)

	mshrEntry.Block = block
	mshrEntry.Requests = append(mshrEntry.Requests, trans)

	return true
}

func (ds *directoryStage) isWritingFullLine(write *mem.WriteReq) bool {
	if len(write.Data) != (1 << ds.cache.log2BlockSize) {
		return false
	}

	if write.DirtyMask != nil {
		for _, dirty := range write.DirtyMask {
			if !dirty {
				return false
			}
		}
	}

	return true
}

func (ds *directoryStage) needEviction(victim *cache.Block) bool {
	return victim.IsValid && victim.IsDirty
}
