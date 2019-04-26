package l1v

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util"
)

type bottomParser struct {
	bottomPort       akita.Port
	mshr             cache.MSHR
	bankBufs         []util.Buffer
	transactions     *[]*transaction
	log2BlockSize    uint64
	wayAssociativity int
}

func (p *bottomParser) Tick(now akita.VTimeInSec) bool {
	item := p.bottomPort.Peek()
	if item == nil {
		return false
	}

	switch rsp := item.(type) {
	case *mem.DoneRsp:
		return p.processDoneRsp(now, rsp)
	case *mem.DataReadyRsp:
		return p.processDataReady(now, rsp)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(rsp))
	}

	panic("not implemented")
}

func (p *bottomParser) processDoneRsp(
	now akita.VTimeInSec,
	done *mem.DoneRsp,
) bool {
	trans := p.findTransactionByWriteToBottomID(done.GetRespondTo())

	for _, t := range trans.preCoalesceTransactions {
		t.doneFromBottom = done
	}

	p.removeTransaction(trans)
	p.bottomPort.Retrieve(now)
	return true
}

func (p *bottomParser) processDataReady(
	now akita.VTimeInSec,
	dr *mem.DataReadyRsp,
) bool {
	trans := p.findTransactionByReadToBottomID(dr.GetRespondTo())
	bankBuf := p.getBankBuf(trans.block)
	if !bankBuf.CanPush() {
		return false
	}

	addr := trans.Address()
	cachelineID := (addr >> p.log2BlockSize) << p.log2BlockSize
	mshrEntry := p.mshr.Query(cachelineID)
	data := dr.Data
	dirtyMask := make([]bool, 1<<p.log2BlockSize)
	for _, t := range mshrEntry.Requests {
		trans = t.(*transaction)
		if trans.read != nil {
			for _, preCTrans := range trans.preCoalesceTransactions {
				preCTrans.dataReadyFromBottom = dr
			}
		} else {
			for _, preCTrans := range trans.preCoalesceTransactions {
				preCTrans.doneFromBottom = mem.NewDoneRsp(0, nil, nil, "")
			}
			write := trans.write
			offset := write.Address - cachelineID
			for i := 0; i < len(write.Data); i++ {
				if write.DirtyMask[i] == true {
					data[offset+uint64(i)] = write.Data[i]
					dirtyMask[offset+uint64(i)] = true
				}
			}
		}
		p.removeTransaction(trans)
	}
	p.mshr.Remove(cachelineID)

	trans.bankAction = bankActionWriteFetched
	trans.data = data
	trans.writeFetchedDirtyMask = dirtyMask
	bankBuf.Push(trans)

	p.bottomPort.Retrieve(now)
	return true
}

func (p *bottomParser) findTransactionByWriteToBottomID(
	id string,
) *transaction {
	for _, trans := range *p.transactions {
		if trans.writeToBottom != nil && trans.writeToBottom.ID == id {
			return trans
		}
	}
	panic("trans not found")
}

func (p *bottomParser) findTransactionByReadToBottomID(
	id string,
) *transaction {
	for _, trans := range *p.transactions {
		if trans.readToBottom != nil && trans.readToBottom.ID == id {
			return trans
		}
	}
	panic("trans not found")
}

func (p *bottomParser) removeTransaction(trans *transaction) {
	for i, t := range *p.transactions {
		if t == trans {
			*p.transactions = append(
				(*p.transactions)[:i],
				(*p.transactions)[i+1:]...)
			return
		}
	}
	panic("trans not found")
}

func (p *bottomParser) getBankBuf(block *cache.Block) util.Buffer {
	numWaysPerSet := p.wayAssociativity
	blockID := block.SetID*numWaysPerSet + block.WayID
	bankID := blockID % len(p.bankBufs)
	return p.bankBufs[bankID]
}
