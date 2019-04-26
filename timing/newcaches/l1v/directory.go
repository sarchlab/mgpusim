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
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	mshrEntry := d.mshr.Query(cacheLineID)
	if mshrEntry != nil {
		return d.processMSHRHit(trans, mshrEntry)
	}

	block := d.dir.Lookup(cacheLineID)
	if block != nil && block.IsValid {
		return d.processReadHit(trans, block)
	}

	return d.processReadMiss(now, trans)

}

func (d *directory) processMSHRHit(
	trans *transaction,
	mshrEntry *cache.MSHREntry,
) bool {
	mshrEntry.Requests = append(mshrEntry.Requests, trans)
	d.inBuf.Pop()
	return true
}

func (d *directory) processReadHit(
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

	victim.Tag = cacheLineID
	victim.IsValid = true
	victim.IsLocked = true
	d.dir.Visit(victim)

	d.inBuf.Pop()

	return true
}

func (d *directory) fetchFromBottom(
	now akita.VTimeInSec,
	trans *transaction,
	victim *cache.Block,
) bool {
	read := trans.read
	addr := read.Address
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	bottomModule := d.lowModuleFinder.Find(cacheLineID)
	readToBottom := mem.NewReadReq(now, d.bottomPort, bottomModule,
		cacheLineID, 1<<d.log2BlockSize)
	readToBottom.PID = read.PID
	err := d.bottomPort.Send(readToBottom)
	if err != nil {
		return false
	}

	trans.readToBottom = readToBottom

	mshrEntry := d.mshr.Add(cacheLineID)
	mshrEntry.Requests = append(mshrEntry.Requests, trans)
	mshrEntry.ReadReq = readToBottom
	mshrEntry.Block = victim
	return true
}

func (d *directory) processWrite(
	now akita.VTimeInSec,
	trans *transaction,
) bool {
	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	mshrEntry := d.mshr.Query(cacheLineID)
	if mshrEntry != nil {
		ok := d.writeBottom(now, trans)
		if ok {
			return d.processMSHRHit(trans, mshrEntry)
		}
		return false
	}

	block := d.dir.Lookup(cacheLineID)
	if block != nil && block.IsValid {
		return d.processWriteHit(now, trans, block)
	}

	block = d.dir.FindVictim(cacheLineID)
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

	ok := d.writeBottom(now, trans)
	if !ok {
		return false
	}

	write := trans.write
	addr := write.Address
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	block.IsLocked = true
	if block.Tag != cacheLineID {
		if len(write.Data) == 1<<d.log2BlockSize {
			block.IsValid = true
		} else {
			block.IsValid = false
		}
	}
	block.Tag = cacheLineID
	d.dir.Visit(block)

	trans.bankAction = bankActionWrite
	trans.block = block
	bankBuf.Push(trans)

	d.inBuf.Pop()
	return true
}

func (d *directory) getBankBuf(block *cache.Block) util.Buffer {
	numWaysPerSet := d.dir.WayAssociativity()
	blockID := block.SetID*numWaysPerSet + block.WayID
	bankID := blockID % len(d.bankBufs)
	return d.bankBufs[bankID]
}
