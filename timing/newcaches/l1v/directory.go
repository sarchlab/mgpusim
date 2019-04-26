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

	bankNum := getBankNum(block, d.dir.WayAssociativity(), len(d.bankBufs))
	bankBuf := d.bankBufs[bankNum]
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

	bottomModule := d.lowModuleFinder.Find(cacheLineID)
	readToBottom := mem.NewReadReq(now, d.bottomPort, bottomModule,
		cacheLineID, 1<<d.log2BlockSize)
	readToBottom.PID = read.PID
	err := d.bottomPort.Send(readToBottom)
	if err != nil {
		return false
	}

	mshrEntry := d.mshr.Add(cacheLineID)
	mshrEntry.Requests = append(mshrEntry.Requests, trans)
	mshrEntry.ReadReq = readToBottom
	mshrEntry.Block = victim

	victim.Tag = cacheLineID
	victim.IsValid = true
	victim.IsLocked = true
	d.dir.Visit(victim)

	d.inBuf.Pop()

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
		return d.processMSHRHit(trans, mshrEntry)
	}

	block := d.dir.Lookup(cacheLineID)
	if block != nil && block.IsValid {
		return d.processWriteHit(trans, block)
	}

	block = d.dir.FindVictim(cacheLineID)
	return d.processWriteHit(trans, block)
}

func (d *directory) processWriteHit(
	trans *transaction,
	block *cache.Block,
) bool {
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	bankNum := getBankNum(block, d.dir.WayAssociativity(), len(d.bankBufs))
	bankBuf := d.bankBufs[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	block.IsLocked = true

	d.dir.Visit(block)

	trans.bankAction = bankActionWrite
	trans.block = block
	bankBuf.Push(trans)

	d.inBuf.Pop()
	return true
}

func getBankNum(block *cache.Block, wayAssociativity, numBanks int) int {
	return (block.SetID*wayAssociativity + block.WayID) % numBanks
}
