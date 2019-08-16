package l1v

import (
	"log"
	"reflect"

	"github.com/rs/xid"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/util/tracing"
)

type coalescer struct {
	cache      *Cache
	toCoalesce []*transaction
}

func (c *coalescer) Tick(now akita.VTimeInSec) bool {
	req := c.cache.TopPort.Peek()
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
	trans := c.createTransaction(req, now)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.cache.TopPort.Retrieve(now)

	tracing.TraceReqReceive(req, now, c.cache)
	return true
}

func (c *coalescer) processReqNoncoalescable(
	now akita.VTimeInSec,
	req mem.AccessReq,
) bool {
	if !c.cache.dirBuf.CanPush() {
		return false
	}

	c.coalesceAndSend(now)

	trans := c.createTransaction(req, now)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.cache.TopPort.Retrieve(now)

	tracing.TraceReqReceive(req, now, c.cache)
	return true
}

func (c *coalescer) processReqLastInWaveCoalescable(
	now akita.VTimeInSec,
	req mem.AccessReq,
) bool {
	if !c.cache.dirBuf.CanPush() {
		return false
	}

	trans := c.createTransaction(req, now)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.coalesceAndSend(now)
	c.cache.TopPort.Retrieve(now)

	tracing.TraceReqReceive(req, now, c.cache)
	return true
}

func (c *coalescer) processReqLastInWaveNoncoalescable(
	now akita.VTimeInSec,
	req mem.AccessReq,
) bool {
	if !c.cache.dirBuf.CanPush() {
		return false
	}
	c.coalesceAndSend(now)

	if !c.cache.dirBuf.CanPush() {
		return true
	}

	trans := c.createTransaction(req, now)
	c.toCoalesce = append(c.toCoalesce, trans)
	c.cache.transactions = append(c.cache.transactions, trans)
	c.coalesceAndSend(now)
	c.cache.TopPort.Retrieve(now)

	tracing.TraceReqReceive(req, now, c.cache)
	return true
}

func (c *coalescer) createTransaction(req mem.AccessReq, now akita.VTimeInSec) *transaction {
	switch req := req.(type) {
	case *mem.ReadReq:
		t := &transaction{
			id:   xid.New().String(),
			read: req,
		}
		tracing.StartTask(t.id, tracing.ReqIDAtReceiver(req, c.cache),
			now, c.cache, "l1_transaction", "read", nil)
		return t
	case *mem.WriteReq:
		t := &transaction{
			id:    xid.New().String(),
			write: req,
		}
		tracing.StartTask(t.id, tracing.ReqIDAtReceiver(req, c.cache),
			now, c.cache, "l1_transaction", "write", nil)
		return t
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
	blockSize := uint64(1 << c.cache.log2BlockSize)
	if req.GetAddress()/blockSize == c.toCoalesce[0].Address()/blockSize {
		return true
	}
	return false
}

func (c *coalescer) coalesceAndSend(now akita.VTimeInSec) bool {
	var trans *transaction
	if c.toCoalesce[0].read != nil {
		trans = c.coalesceRead()
	} else {
		trans = c.coalesceWrite()
	}
	c.cache.dirBuf.Push(trans)
	c.cache.postCoalesceTransactions =
		append(c.cache.postCoalesceTransactions, trans)
	c.toCoalesce = nil
	return true
}

func (c *coalescer) coalesceRead() *transaction {
	blockSize := uint64(1 << c.cache.log2BlockSize)
	cachelineID := c.toCoalesce[0].Address() / blockSize * blockSize
	coalescedRead := mem.NewReadReq(0, nil, nil, cachelineID, blockSize)
	coalescedRead.PID = c.toCoalesce[0].PID()
	return &transaction{
		read:                    coalescedRead,
		preCoalesceTransactions: c.toCoalesce,
	}
}

func (c *coalescer) coalesceWrite() *transaction {
	blockSize := uint64(1 << c.cache.log2BlockSize)
	cachelineID := c.toCoalesce[0].Address() / blockSize * blockSize
	write := mem.NewWriteReq(0, nil, nil, cachelineID)
	write.Data = make([]byte, blockSize)
	write.DirtyMask = make([]bool, blockSize)
	write.PID = c.toCoalesce[0].PID()

	for _, t := range c.toCoalesce {
		w := t.write
		offset := int(w.Address - cachelineID)
		for i := 0; i < len(w.Data); i++ {
			if w.DirtyMask == nil || w.DirtyMask[i] == true {
				write.Data[i+offset] = w.Data[i]
				write.DirtyMask[i+offset] = true
			}
		}
	}
	return &transaction{
		write:                   write,
		preCoalesceTransactions: c.toCoalesce,
	}
}
