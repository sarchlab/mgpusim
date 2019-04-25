package l1v

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/util"
)

type coalescer struct {
	log2BlockSize uint64
	topPort       akita.Port
	transactions  *[]*transaction
	dirBuf        util.Buffer

	toCoalesce []*transaction
}

func (c *coalescer) Tick(now akita.VTimeInSec) bool {
	req := c.topPort.Peek()
	if req == nil {
		return false
	}

	return c.processReq(now, req.(mem.AccessReq))

}

func (c *coalescer) processReq(
	now akita.VTimeInSec,
	req mem.AccessReq,
) bool {
	if c.isReqLastInWave(req) {
		if len(c.toCoalesce) == 0 || c.canReqCoalesce(req) {
			return c.processReqLastInWaveCoalescable(now, req)
		}
		return c.processReqLastInWaveNoncoalescable(now, req)
	}

	if len(c.toCoalesce) == 0 || c.canReqCoalesce(req) {
		return c.processReqCoalescable(now, req)
	}
	return c.processReqNoncoalescable(now, req)
}

func (c *coalescer) processReqCoalescable(
	now akita.VTimeInSec,
	req mem.AccessReq,
) bool {
	trans := c.createTransaction(req)
	c.toCoalesce = append(c.toCoalesce, trans)
	*c.transactions = append(*c.transactions, trans)
	c.topPort.Retrieve(now)
	return true
}

func (c *coalescer) processReqNoncoalescable(
	now akita.VTimeInSec,
	req mem.AccessReq,
) bool {
	if !c.dirBuf.CanPush() {
		return false
	}

	c.coalesceAndSend()

	trans := c.createTransaction(req)
	c.toCoalesce = append(c.toCoalesce, trans)
	*c.transactions = append(*c.transactions, trans)
	c.topPort.Retrieve(now)

	return true
}

func (c *coalescer) processReqLastInWaveCoalescable(
	now akita.VTimeInSec,
	req mem.AccessReq,
) bool {
	if !c.dirBuf.CanPush() {
		return false
	}

	trans := c.createTransaction(req)
	c.toCoalesce = append(c.toCoalesce, trans)
	*c.transactions = append(*c.transactions, trans)
	c.coalesceAndSend()
	c.topPort.Retrieve(now)

	return true
}

func (c *coalescer) processReqLastInWaveNoncoalescable(
	now akita.VTimeInSec,
	req mem.AccessReq,
) bool {
	if !c.dirBuf.CanPush() {
		return false
	}
	c.coalesceAndSend()

	if !c.dirBuf.CanPush() {
		return true
	}

	trans := c.createTransaction(req)
	c.toCoalesce = append(c.toCoalesce, trans)
	*c.transactions = append(*c.transactions, trans)
	c.coalesceAndSend()
	c.topPort.Retrieve(now)

	return true
}

func (c *coalescer) createTransaction(req mem.AccessReq) *transaction {
	switch req := req.(type) {
	case *mem.ReadReq:
		return &transaction{read: req}
	case *mem.WriteReq:
		return &transaction{write: req}
	default:
		log.Panicf("cannot process request of type %s\n", reflect.TypeOf(req))
		return nil
	}
}

func (c *coalescer) isReqLastInWave(req mem.AccessReq) bool {
	switch req := req.(type) {
	case *mem.ReadReq:
		return req.IsLastInWave
	case *mem.WriteReq:
		return req.IsLastInWave
	default:
		panic("unknown type")
	}
}

func (c *coalescer) canReqCoalesce(req mem.AccessReq) bool {
	blockSize := uint64(1 << c.log2BlockSize)
	if req.GetAddress()/blockSize == c.toCoalesce[0].Address()/blockSize {
		return true
	}
	return false
}

func (c *coalescer) waitForCoaleasing(
	trans *transaction,
) {
	*c.transactions = append(*c.transactions, trans)
	c.toCoalesce = append(c.toCoalesce, trans)
}

func (c *coalescer) coalesceAndSend() bool {

	postCoaleascingTrans := &transaction{
		read: mem.NewReadReq(0, nil, nil,
			c.toCoalesce[0].Address(), 1<<c.log2BlockSize),
		preCoalesceTransactions: c.toCoalesce,
	}
	c.dirBuf.Push(postCoaleascingTrans)
	c.toCoalesce = nil
	return true
}
