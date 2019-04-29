package l1v

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util"
)

type directory struct {
	inBuf           util.Buffer
	dir             cache.Directory
	mshr            cache.MSHR
	bankBufs        []util.Buffer
	bottomPort      akita.Port
	lowModuleFinder cache.LowModuleFinder
	log2BlockSize   uint64
}

func (d *directory) Tick(now akita.VTimeInSec) bool {
	item := d.inBuf.Peek()
	if item == nil {
		return false
	}

	trans := item.(*transaction)
	if trans.read != nil {
		return d.processRead(now, trans)
	}

	return d.processWrite(now, trans)
}

func (d *directory) processRead(now akita.VTimeInSec, trans *transaction) bool {
	read := trans.read
	addr := read.Address
	pid := read.PID
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	mshrEntry := d.mshr.Query(pid, cacheLineID)
	if mshrEntry != nil {
		return d.processMSHRHit(now, trans, mshrEntry)
	}

	block := d.dir.Lookup(pid, cacheLineID)
	if block != nil && block.IsValid {
		return d.processReadHit(now, trans, block)
	}

	return d.processReadMiss(now, trans)

}

func (d *directory) processMSHRHit(
	now akita.VTimeInSec,
	trans *transaction,
	mshrEntry *cache.MSHREntry,
) bool {
	mshrEntry.Requests = append(mshrEntry.Requests, trans)
	d.inBuf.Pop()

	if trans.read != nil {
		trace(now, "r-mshr-hit", trans.Address(), nil)
	} else {
		trace(now, "w-mshr-hit", trans.Address(), trans.write.Data)
	}

	return true
}

func (d *directory) processReadHit(
	now akita.VTimeInSec,
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
	d.dir.Visit(block)
	bankBuf.Push(trans)

	d.inBuf.Pop()

	trace(now, "r-hit", block.Tag, nil)

	return true
}

func (d *directory) processReadMiss(
	now akita.VTimeInSec,
	trans *transaction,
) bool {
	read := trans.read
	addr := read.Address
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	victim := d.dir.FindVictim(cacheLineID)
	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if d.mshr.IsFull() {
		return false
	}

	if !d.fetchFromBottom(now, trans, victim) {
		return false
	}

	d.inBuf.Pop()

	trace(now, "r-miss", addr, nil)

	return true
}

func (d *directory) processWrite(
	now akita.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	pid := write.PID
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	mshrEntry := d.mshr.Query(pid, cacheLineID)
	if mshrEntry != nil {
		ok := d.writeBottom(now, trans)
		if ok {
			return d.processMSHRHit(now, trans, mshrEntry)
		}
		return false
	}

	block := d.dir.Lookup(pid, cacheLineID)
	if block != nil && block.IsValid {
		return d.processWriteHit(now, trans, block)
	}

	if d.isPartialWrite(write) {
		return d.partialWriteMiss(now, trans)
	}
	return d.fullLineWriteMiss(now, trans)
}

func (d *directory) isPartialWrite(write *mem.WriteReq) bool {
	if len(write.Data) < (1 << d.log2BlockSize) {
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
	now akita.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	sentThisCycle := false
	if trans.writeToBottom == nil {
		ok := d.writeBottom(now, trans)
		if !ok {
			return false
		}
		sentThisCycle = true
	}

	if d.mshr.IsFull() {
		if sentThisCycle {
			return true
		}
		return false
	}

	victim := d.dir.FindVictim(cacheLineID)
	ok := d.fetchFromBottom(now, trans, victim)
	if !ok {
		if sentThisCycle {
			return true
		}
		return false
	}
	trans.fetchAndWrite = true

	d.inBuf.Pop()
	trace(now, "w-miss-partial", addr, write.Data)
	return true
}

func (d *directory) fullLineWriteMiss(
	now akita.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	block := d.dir.FindVictim(cacheLineID)
	return d.processWriteHit(now, trans, block)
}

func (d *directory) writeBottom(now akita.VTimeInSec, trans *transaction) bool {
	write := trans.write
	addr := write.Address

	writeToBottom := mem.NewWriteReq(
		now,
		d.bottomPort,
		d.lowModuleFinder.Find(addr),
		addr,
	)
	writeToBottom.Data = write.Data
	writeToBottom.PID = write.PID

	err := d.bottomPort.Send(writeToBottom)
	if err != nil {
		return false
	}

	trans.writeToBottom = writeToBottom
	return true
}

func (d *directory) processWriteHit(
	now akita.VTimeInSec,
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
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	block.IsLocked = true
	block.IsValid = true
	block.Tag = cacheLineID
	d.dir.Visit(block)

	trans.bankAction = bankActionWrite
	trans.block = block
	bankBuf.Push(trans)

	d.inBuf.Pop()

	trace(now, "w", addr, write.Data)

	return true
}

func (d *directory) fetchFromBottom(
	now akita.VTimeInSec,
	trans *transaction,
	victim *cache.Block,
) bool {
	addr := trans.Address()
	pid := trans.PID()
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	bottomModule := d.lowModuleFinder.Find(cacheLineID)
	readToBottom := mem.NewReadReq(now, d.bottomPort, bottomModule,
		cacheLineID, 1<<d.log2BlockSize)
	readToBottom.PID = pid
	err := d.bottomPort.Send(readToBottom)
	if err != nil {
		return false
	}

	trans.readToBottom = readToBottom
	trans.block = victim

	mshrEntry := d.mshr.Add(pid, cacheLineID)
	mshrEntry.Requests = append(mshrEntry.Requests, trans)
	mshrEntry.ReadReq = readToBottom
	mshrEntry.Block = victim

	victim.Tag = cacheLineID
	victim.PID = pid
	victim.IsValid = true
	victim.IsLocked = true
	d.dir.Visit(victim)

	return true
}

func (d *directory) getBankBuf(block *cache.Block) util.Buffer {
	numWaysPerSet := d.dir.WayAssociativity()
	blockID := block.SetID*numWaysPerSet + block.WayID
	bankID := blockID % len(d.bankBufs)
	return d.bankBufs[bankID]
}
