package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type bank struct {
	pipeline        pipelining.Pipeline
	postPipelineBuf sim.Buffer
	lastRowAddr     uint64
	rowValid        bool
	delayQueue      []delayedItem
}

type delayedItem struct {
	item       *bankPipelineItem
	cyclesLeft int
}

type bankPipelineItem struct {
	req       mem.AccessReq
	committed bool
	readData  []byte
}

func (i *bankPipelineItem) TaskID() string {
	return i.req.Meta().ID + "_pl"
}

// Comp models a banked memory with configurable banking and pipeline behavior.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort sim.Port

	Storage              *mem.Storage
	AddressConverter     mem.AddressConverter
	BankAddressConverter mem.AddressConverter // Used ONLY for bank selection

	banks        []bank
	bankSelector bankSelector

	rowBufferSizeLog2  uint64
	rowMissDelay       int
	log2InterleaveSize uint64
	numBanks           int
}

// Tick updates the component state cycle by cycle.
func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type middleware struct {
	*Comp
	pendingReqs []mem.AccessReq
}

func (m *middleware) Tick() (madeProgress bool) {
	madeProgress = m.finalizeBanks() || madeProgress
	madeProgress = m.tickPipelines() || madeProgress
	madeProgress = m.tickDelayQueues() || madeProgress
	madeProgress = m.dispatchPending() || madeProgress
	madeProgress = m.drainTopPort() || madeProgress
	return madeProgress
}

// drainTopPort reads all incoming messages into the pending buffer.
func (m *middleware) drainTopPort() bool {
	madeProgress := false
	for {
		msg := m.topPort.PeekIncoming()
		if msg == nil {
			break
		}
		req, ok := msg.(mem.AccessReq)
		if !ok {
			log.Panicf("simplebankedmemory: unsupported message type %T", msg)
		}
		m.topPort.RetrieveIncoming()
		tracing.TraceReqReceive(req, m.Comp)
		m.pendingReqs = append(m.pendingReqs, req)
		madeProgress = true
	}
	return madeProgress
}

// dispatchPending tries to dispatch pending requests to banks, skipping
// blocked banks to avoid head-of-line blocking.
func (m *middleware) dispatchPending() bool {
	madeProgress := false
	remaining := make([]mem.AccessReq, 0, len(m.pendingReqs))

	for _, req := range m.pendingReqs {
		addr := req.GetAddress()
		if m.BankAddressConverter != nil {
			addr = m.BankAddressConverter.ConvertExternalToInternal(addr)
		} else if m.AddressConverter != nil {
			addr = m.AddressConverter.ConvertExternalToInternal(addr)
		}

		bankID := m.bankSelector.Select(addr, len(m.banks))
		b := &m.banks[bankID]

		item := &bankPipelineItem{req: req}

		if m.rowBufferSizeLog2 > 0 && m.rowMissDelay > 0 {
			interleaveSize := uint64(1) << m.log2InterleaveSize
			bankBlockIndex := addr / interleaveSize
			bankLocalBlock := bankBlockIndex / uint64(len(m.banks))
			offset := addr & (interleaveSize - 1)
			bankLocalAddr := bankLocalBlock*interleaveSize + offset
			rowAddr := bankLocalAddr >> m.rowBufferSizeLog2

			if b.rowValid && b.lastRowAddr == rowAddr {
				// ROW HIT — accept into pipeline directly
				if !b.pipeline.CanAccept() {
					remaining = append(remaining, req)
					continue
				}
				b.pipeline.Accept(item)
			} else {
				// ROW MISS — add to delay queue
				b.delayQueue = append(b.delayQueue, delayedItem{
					item:       item,
					cyclesLeft: m.rowMissDelay,
				})
			}

			b.lastRowAddr = rowAddr
			b.rowValid = true
		} else {
			// No row buffer tracking — old behavior
			if !b.pipeline.CanAccept() {
				remaining = append(remaining, req)
				continue
			}
			b.pipeline.Accept(item)
		}

		madeProgress = true
	}

	m.pendingReqs = remaining
	return madeProgress
}

func (m *middleware) finalizeBanks() bool {
	madeProgress := false

	for i := range m.banks {
		for {
			progress := m.finalizeSingle(&m.banks[i])
			if !progress {
				break
			}

			madeProgress = true
		}
	}

	return madeProgress
}

func (m *middleware) finalizeSingle(b *bank) bool {
	itemIfc := b.postPipelineBuf.Peek()
	if itemIfc == nil {
		return false
	}

	item := itemIfc.(*bankPipelineItem)

	switch req := item.req.(type) {
	case *mem.ReadReq:
		return m.finalizeRead(b, item, req)
	case *mem.WriteReq:
		return m.finalizeWrite(b, item, req)
	default:
		log.Panicf("simplebankedmemory: unsupported request type %T", req)
	}

	return false
}

func (m *middleware) finalizeRead(
	b *bank,
	item *bankPipelineItem,
	req *mem.ReadReq,
) bool {
	if !item.committed {
		addr := req.Address
		if m.AddressConverter != nil {
			addr = m.AddressConverter.ConvertExternalToInternal(addr)
		}

		data, err := m.Storage.Read(addr, req.AccessByteSize)
		if err != nil {
			log.Panic(err)
		}

		item.readData = data
		item.committed = true
	}

	if !m.topPort.CanSend() {
		return false
	}

	rsp := mem.DataReadyRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithData(item.readData).
		Build()

	if err := m.topPort.Send(rsp); err != nil {
		return false
	}

	tracing.TraceReqComplete(req, m.Comp)

	b.postPipelineBuf.Pop()

	return true
}

func (m *middleware) finalizeWrite(
	b *bank,
	item *bankPipelineItem,
	req *mem.WriteReq,
) bool {
	if !item.committed {
		addr := req.Address
		if m.AddressConverter != nil {
			addr = m.AddressConverter.ConvertExternalToInternal(addr)
		}

		if req.DirtyMask == nil {
			if err := m.Storage.Write(addr, req.Data); err != nil {
				log.Panic(err)
			}
		} else {
			data, err := m.Storage.Read(addr, uint64(len(req.Data)))
			if err != nil {
				log.Panic(err)
			}

			for i := range req.Data {
				if req.DirtyMask[i] {
					data[i] = req.Data[i]
				}
			}

			if err := m.Storage.Write(addr, data); err != nil {
				log.Panic(err)
			}
		}

		item.committed = true
	}

	if !m.topPort.CanSend() {
		return false
	}

	rsp := mem.WriteDoneRspBuilder{}.
		WithSrc(m.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		Build()

	if err := m.topPort.Send(rsp); err != nil {
		return false
	}

	tracing.TraceReqComplete(req, m.Comp)

	b.postPipelineBuf.Pop()

	return true
}

func (m *middleware) tickPipelines() bool {
	madeProgress := false

	for i := range m.banks {
		p := m.banks[i].pipeline
		madeProgress = p.Tick() || madeProgress
	}

	return madeProgress
}

func (m *middleware) tickDelayQueues() bool {
	madeProgress := false
	for i := range m.banks {
		b := &m.banks[i]
		if len(b.delayQueue) == 0 {
			continue
		}
		remaining := make([]delayedItem, 0, len(b.delayQueue))
		for _, di := range b.delayQueue {
			di.cyclesLeft--
			if di.cyclesLeft <= 0 {
				if b.pipeline.CanAccept() {
					b.pipeline.Accept(di.item)
				} else {
					remaining = append(remaining, di)
				}
			} else {
				remaining = append(remaining, di)
			}
		}
		b.delayQueue = remaining
		// Report progress whenever items are in the delay queue, even if
		// just counting down. This keeps the TickingComponent scheduled so
		// the delay countdown continues each cycle.
		madeProgress = true
	}
	return madeProgress
}
