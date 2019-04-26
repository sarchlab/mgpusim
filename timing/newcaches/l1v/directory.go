package l1v

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util"
)

type directory struct {
	inBuf         util.Buffer
	dir           cache.Directory
	mshr          cache.MSHR
	bankBufs      []util.Buffer
	log2BlockSize uint64
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

	panic("not implemented")
}

func (d *directory) processRead(now akita.VTimeInSec, trans *transaction) bool {
	read := trans.read
	addr := read.Address
	blockSize := uint64(1 << d.log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	mshrEntry := d.mshr.Query(cacheLineID)
	if mshrEntry != nil {
		mshrEntry.Requests = append(mshrEntry.Requests, trans)
		d.inBuf.Pop()
		return true
	}

	block := d.dir.Lookup(cacheLineID)
	if block != nil && block.IsValid {
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

		return true
	}

	panic("not implemented")

}

func getBankNum(block *cache.Block, wayAssociativity, numBanks int) int {
	return (block.SetID*wayAssociativity + block.WayID) % numBanks
}
