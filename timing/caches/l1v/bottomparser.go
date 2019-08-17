package l1v

import (
	"fmt"
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util"
	"gitlab.com/akita/util/tracing"
)

type bottomParser struct {
	cache *Cache
}

func (p *bottomParser) Tick(now akita.VTimeInSec) bool {
	item := p.cache.BottomPort.Peek()
	if item == nil {
		return false
	}

	switch rsp := item.(type) {
	case *mem.DoneRsp:
		return p.processDoneRsp(now, rsp)
	case *mem.DataReadyRsp:
		return p.processDataReady(now, rsp)
	default:
		panic("cannot process response")
	}
}

func (p *bottomParser) processDoneRsp(
	now akita.VTimeInSec,
	done *mem.DoneRsp,
) bool {
	trans := p.findTransactionByWriteToBottomID(done.GetRespondTo())
	if trans == nil || trans.fetchAndWrite {
		p.cache.BottomPort.Retrieve(now)
		return true
	}

	for _, t := range trans.preCoalesceTransactions {
		t.done = true
	}

	p.removeTransaction(trans)
	p.cache.BottomPort.Retrieve(now)

	tracing.TraceReqFinalize(trans.writeToBottom, now, p.cache)
	tracing.EndTask(trans.id, now, p.cache)
	return true
}

func (p *bottomParser) processDataReady(
	now akita.VTimeInSec,
	dr *mem.DataReadyRsp,
) bool {
	trans := p.findTransactionByReadToBottomID(dr.GetRespondTo())
	if trans == nil {
		log.Panicf("cannot find trans")
	}
	pid := trans.readToBottom.PID
	bankBuf := p.getBankBuf(trans.block)
	if !bankBuf.CanPush() {
		return false
	}

	addr := trans.Address()
	cachelineID := (addr >> p.cache.log2BlockSize) << p.cache.log2BlockSize
	data := dr.Data
	dirtyMask := make([]bool, 1<<p.cache.log2BlockSize)
	mshrEntry := p.cache.mshr.Query(pid, cachelineID)
	p.mergeMSHRData(mshrEntry, data, dirtyMask)
	p.finalizeMSHRTrans(mshrEntry, data, now)
	p.cache.mshr.Remove(pid, cachelineID)

	trans.bankAction = bankActionWriteFetched
	trans.data = data
	trans.writeFetchedDirtyMask = dirtyMask
	bankBuf.Push(trans)

	p.removeTransaction(trans)
	p.cache.BottomPort.Retrieve(now)

	tracing.TraceReqFinalize(trans.readToBottom, now, p.cache)
	if p.cache.Name() == "GPU_1.L1I_00" {
		fmt.Printf("Finalize fetch task %s\n", trans.readToBottom.ID)
	}

	return true
}

func (p *bottomParser) mergeMSHRData(
	mshrEntry *cache.MSHREntry,
	data []byte,
	dirtyMask []bool,
) {
	for _, t := range mshrEntry.Requests {
		trans := t.(*transaction)

		if trans.write == nil {
			continue
		}

		write := trans.write
		offset := write.Address - mshrEntry.Block.Tag
		for i := 0; i < len(write.Data); i++ {
			if write.DirtyMask[i] == true {
				data[offset+uint64(i)] = write.Data[i]
				dirtyMask[offset+uint64(i)] = true
			}
		}
	}
}

func (p *bottomParser) finalizeMSHRTrans(
	mshrEntry *cache.MSHREntry,
	data []byte,
	now akita.VTimeInSec,
) {
	for _, t := range mshrEntry.Requests {
		trans := t.(*transaction)
		if trans.read != nil {
			for _, preCTrans := range trans.preCoalesceTransactions {
				read := preCTrans.read
				offset := read.Address - mshrEntry.Block.Tag
				preCTrans.data = data[offset : offset+read.MemByteSize]
				preCTrans.done = true
			}
		} else {
			for _, preCTrans := range trans.preCoalesceTransactions {
				preCTrans.done = true
			}
		}
		p.removeTransaction(trans)

		tracing.EndTask(trans.id, now, p.cache)
	}
}

func (p *bottomParser) findTransactionByWriteToBottomID(
	id string,
) *transaction {
	for _, trans := range p.cache.postCoalesceTransactions {
		if trans.writeToBottom != nil && trans.writeToBottom.ID == id {
			return trans
		}
	}
	return nil
}

func (p *bottomParser) findTransactionByReadToBottomID(
	id string,
) *transaction {
	for _, trans := range p.cache.postCoalesceTransactions {
		if trans.readToBottom != nil && trans.readToBottom.ID == id {
			return trans
		}
	}
	return nil
}

func (p *bottomParser) removeTransaction(trans *transaction) {
	for i, t := range p.cache.postCoalesceTransactions {
		if t == trans {
			p.cache.postCoalesceTransactions = append(
				(p.cache.postCoalesceTransactions)[:i],
				(p.cache.postCoalesceTransactions)[i+1:]...)
			return
		}
	}
}

func (p *bottomParser) getBankBuf(block *cache.Block) util.Buffer {
	numWaysPerSet := p.cache.wayAssociativity
	blockID := block.SetID*numWaysPerSet + block.WayID
	bankID := blockID % len(p.cache.bankBufs)
	return p.cache.bankBufs[bankID]
}
