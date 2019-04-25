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

	switch req := req.(type) {
	case *mem.ReadReq:
		return c.processReadReq(now, req)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}

	panic("not implemented")
}

func (c *coalescer) processReadReq(
	now akita.VTimeInSec,
	read *mem.ReadReq,
) bool {
	trans := &transaction{
		read: read,
	}

	if len(c.toCoalesce) == 0 || c.canReqCoalesce(read) {
		c.waitForCoaleasing(now, trans)
		return true
	}

	succeed := c.coaleasePirorTransactionsAndSendToDir()
	if succeed {
		c.waitForCoaleasing(now, trans)
		return true
	}

	return false
}

func (c *coalescer) canReqCoalesce(req mem.AccessReq) bool {
	blockSize := uint64(1 << c.log2BlockSize)
	if req.GetAddress()/blockSize == c.toCoalesce[0].Address()/blockSize {
		return true
	}
	return false
}

func (c *coalescer) waitForCoaleasing(
	now akita.VTimeInSec,
	trans *transaction,
) {
	*c.transactions = append(*c.transactions, trans)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.topPort.Retrieve(now)
}

func (c *coalescer) coaleasePirorTransactionsAndSendToDir() bool {
	if !c.dirBuf.CanPush() {
		return false
	}

	postCoaleascingTrans := &transaction{
		read: mem.NewReadReq(0, nil, nil,
			c.toCoalesce[0].Address(), 1<<c.log2BlockSize),
		preCoalesceTransactions: c.toCoalesce,
	}
	c.dirBuf.Push(postCoaleascingTrans)
	c.toCoalesce = nil
	return true
}
