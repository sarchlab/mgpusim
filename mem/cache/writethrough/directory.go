package writethrough

import (
	"github.com/sarchlab/akita/v3/pipelining"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

type dirPipelineItem struct {
	trans *transaction
}

func (i dirPipelineItem) TaskID() string {
	return i.trans.id + "_dir_pipeline"
}

type directory struct {
	cache    *Cache
	pipeline pipelining.Pipeline
	buf      sim.Buffer
}

func (d *directory) Tick(now sim.VTimeInSec) (madeProgress bool) {
	for i := 0; i < d.cache.numReqPerCycle; i++ {
		if !d.pipeline.CanAccept() {
			break
		}

		item := d.cache.dirBuf.Peek()
		if item == nil {
			break
		}

		trans := item.(*transaction)
		d.pipeline.Accept(now, dirPipelineItem{trans})
		d.cache.dirBuf.Pop()

		madeProgress = true
	}

	madeProgress = d.pipeline.Tick(now) || madeProgress

	for i := 0; i < d.cache.numReqPerCycle; i++ {
		item := d.buf.Peek()
		if item == nil {
			break
		}

		trans := item.(dirPipelineItem).trans

		if trans.read != nil {
			madeProgress = d.processRead(now, trans) || madeProgress
			continue
		}

		madeProgress = d.processWrite(now, trans) || madeProgress
	}

	return madeProgress
}

func (d *directory) processRead(now sim.VTimeInSec, trans *transaction) bool {
	read := trans.read
	addr := read.Address
	pid := read.PID
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	mshrEntry := d.cache.mshr.Query(pid, cacheLineID)
	if mshrEntry != nil {
		return d.processMSHRHit(now, trans, mshrEntry)
	}

	block := d.cache.directory.Lookup(pid, cacheLineID)
	if block != nil && block.IsValid {
		return d.processReadHit(now, trans, block)
	}

	return d.processReadMiss(now, trans)
}

func (d *directory) processMSHRHit(
	now sim.VTimeInSec,
	trans *transaction,
	mshrEntry *cache.MSHREntry,
) bool {
	mshrEntry.Requests = append(mshrEntry.Requests, trans)

	d.buf.Pop()

	if trans.read != nil {
		tracing.AddTaskStep(trans.id, d.cache, "read-mshr-hit")
	} else {
		tracing.AddTaskStep(trans.id, d.cache, "write-mshr-hit")
	}

	return true
}

func (d *directory) processReadHit(
	now sim.VTimeInSec,
	trans *transaction,
	block *cache.Block,
) bool {
	if block.IsLocked {
		return false
	}

	bankBuf := d.getBankBuf(block)
	if !bankBuf.CanPush() {
		return false
	}

	trans.block = block
	trans.bankAction = bankActionReadHit
	block.ReadCount++
	d.cache.directory.Visit(block)
	bankBuf.Push(trans)

	d.buf.Pop()
	tracing.AddTaskStep(trans.id, d.cache, "read-hit")

	return true
}

func (d *directory) processReadMiss(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	read := trans.read
	addr := read.Address
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	victim := d.cache.directory.FindVictim(cacheLineID)
	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if d.cache.mshr.IsFull() {
		return false
	}

	if !d.fetchFromBottom(now, trans, victim) {
		return false
	}

	d.buf.Pop()
	tracing.AddTaskStep(trans.id, d.cache, "read-miss")

	return true
}

func (d *directory) processWrite(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	pid := write.PID
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	mshrEntry := d.cache.mshr.Query(pid, cacheLineID)
	if mshrEntry != nil {
		ok := d.writeBottom(now, trans)
		if ok {
			return d.processMSHRHit(now, trans, mshrEntry)
		}
		return false
	}

	block := d.cache.directory.Lookup(pid, cacheLineID)
	if block != nil && block.IsValid {
		ok := d.processWriteHit(now, trans, block)
		if ok {
			tracing.AddTaskStep(trans.id, d.cache, "write-hit")
		}

		return ok
	}

	if d.isPartialWrite(write) {
		return d.partialWriteMiss(now, trans)
	}

	ok := d.fullLineWriteMiss(now, trans)
	if ok {
		tracing.AddTaskStep(trans.id, d.cache, "write-miss")
	}

	return ok
}

func (d *directory) isPartialWrite(write *mem.WriteReq) bool {
	if len(write.Data) < (1 << d.cache.log2BlockSize) {
		return true
	}

	if write.DirtyMask != nil {
		for _, byteDirty := range write.DirtyMask {
			if !byteDirty {
				return true
			}
		}
	}

	return false
}

func (d *directory) partialWriteMiss(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	trans.fetchAndWrite = true

	if d.cache.mshr.IsFull() {
		return false
	}

	victim := d.cache.directory.FindVictim(cacheLineID)
	if victim.ReadCount > 0 || victim.IsLocked {
		return false
	}

	sentThisCycle := false
	if trans.writeToBottom == nil {
		ok := d.writeBottom(now, trans)
		if !ok {
			return false
		}
		sentThisCycle = true
	}

	ok := d.fetchFromBottom(now, trans, victim)
	if !ok {
		if sentThisCycle {
			return true
		}
		return false
	}

	d.buf.Pop()
	tracing.AddTaskStep(trans.id, d.cache, "write-miss")

	return true
}

func (d *directory) fullLineWriteMiss(
	now sim.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	block := d.cache.directory.FindVictim(cacheLineID)
	return d.processWriteHit(now, trans, block)
}

func (d *directory) writeBottom(now sim.VTimeInSec, trans *transaction) bool {
	write := trans.write
	addr := write.Address

	writeToBottom := mem.WriteReqBuilder{}.
		WithSendTime(now).
		WithSrc(d.cache.bottomPort).
		WithDst(d.cache.lowModuleFinder.Find(addr)).
		WithAddress(addr).
		WithPID(write.PID).
		WithData(write.Data).
		WithDirtyMask(write.DirtyMask).
		Build()

	err := d.cache.bottomPort.Send(writeToBottom)
	if err != nil {
		return false
	}

	trans.writeToBottom = writeToBottom

	tracing.TraceReqInitiate(writeToBottom, d.cache, trans.id)

	return true
}

func (d *directory) processWriteHit(
	now sim.VTimeInSec,
	trans *transaction,
	block *cache.Block,
) bool {
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	bankBuf := d.getBankBuf(block)
	if !bankBuf.CanPush() {
		return false
	}

	if trans.writeToBottom == nil {
		ok := d.writeBottom(now, trans)
		if !ok {
			return false
		}
	}

	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	block.IsLocked = true
	block.IsValid = true
	block.Tag = cacheLineID
	d.cache.directory.Visit(block)

	trans.bankAction = bankActionWrite
	trans.block = block
	bankBuf.Push(trans)

	d.buf.Pop()

	return true
}

func (d *directory) fetchFromBottom(
	now sim.VTimeInSec,
	trans *transaction,
	victim *cache.Block,
) bool {
	addr := trans.Address()
	pid := trans.PID()
	blockSize := uint64(1 << d.cache.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	bottomModule := d.cache.lowModuleFinder.Find(cacheLineID)
	readToBottom := mem.ReadReqBuilder{}.
		WithSendTime(now).
		WithSrc(d.cache.bottomPort).
		WithDst(bottomModule).
		WithAddress(cacheLineID).
		WithPID(pid).
		WithByteSize(blockSize).
		Build()
	err := d.cache.bottomPort.Send(readToBottom)
	if err != nil {
		return false
	}

	tracing.TraceReqInitiate(readToBottom, d.cache, trans.id)
	trans.readToBottom = readToBottom
	trans.block = victim

	mshrEntry := d.cache.mshr.Add(pid, cacheLineID)
	mshrEntry.Requests = append(mshrEntry.Requests, trans)
	mshrEntry.ReadReq = readToBottom
	mshrEntry.Block = victim

	victim.Tag = cacheLineID
	victim.PID = pid
	victim.IsValid = true
	victim.IsLocked = true
	d.cache.directory.Visit(victim)

	return true
}

func (d *directory) getBankBuf(block *cache.Block) sim.Buffer {
	numWaysPerSet := d.cache.directory.WayAssociativity()
	blockID := block.SetID*numWaysPerSet + block.WayID
	bankID := blockID % len(d.cache.bankBufs)
	return d.cache.bankBufs[bankID]
}
