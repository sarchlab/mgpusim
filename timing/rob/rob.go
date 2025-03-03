// Package rob implemented an reorder buffer for memory requests.
package rob

import (
	"container/list"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type transaction struct {
	reqFromTop    mem.AccessReq
	reqToBottom   mem.AccessReq
	rspFromBottom mem.AccessRsp
}

// ReorderBuffer can maintain the returning order of memory transactions.
type ReorderBuffer struct {
	*sim.TickingComponent

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	BottomUnit sim.Port

	bufferSize     int
	numReqPerCycle int

	toBottomReqIDToTransactionTable map[string]*list.Element
	transactions                    *list.List
	isFlushing                      bool
}

// Tick updates the status of the ReorderBuffer.
func (b *ReorderBuffer) Tick() (madeProgress bool) {
	madeProgress = b.processControlMsg() || madeProgress

	if !b.isFlushing {
		madeProgress = b.runPipeline() || madeProgress
	}

	return madeProgress
}

func (b *ReorderBuffer) processControlMsg() (madeProgress bool) {
	item := b.controlPort.PeekIncoming()
	if item == nil {
		return false
	}

	msg := item.(*mem.ControlMsg)
	if msg.DiscardTransations {
		return b.discardTransactions(msg)
	} else if msg.Restart {
		return b.restart(msg)
	}

	panic("never")
}

func (b *ReorderBuffer) discardTransactions(
	msg *mem.ControlMsg,
) (madeProgress bool) {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(b.controlPort.AsRemote()).
		WithDst(msg.Src).
		ToNotifyDone().
		Build()

	err := b.controlPort.Send(rsp)
	if err != nil {
		return false
	}

	b.isFlushing = true
	b.toBottomReqIDToTransactionTable = make(map[string]*list.Element)
	b.transactions.Init()
	b.controlPort.RetrieveIncoming()

	// fmt.Printf("%.10f, %s, rob flushed\n", now, b.Name())

	return true
}

func (b *ReorderBuffer) restart(
	msg *mem.ControlMsg,
) (madeProgress bool) {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(b.controlPort.AsRemote()).
		WithDst(msg.Src).
		ToNotifyDone().
		Build()

	err := b.controlPort.Send(rsp)
	if err != nil {
		return false
	}

	b.isFlushing = false
	b.toBottomReqIDToTransactionTable = make(map[string]*list.Element)
	b.transactions.Init()

	for b.topPort.RetrieveIncoming() != nil {
	}

	for b.bottomPort.RetrieveIncoming() != nil {
	}

	b.controlPort.RetrieveIncoming()

	// fmt.Printf("%.10f, %s, rob restarted\n", now, b.Name())

	return true
}

func (b *ReorderBuffer) runPipeline() (madeProgress bool) {
	for i := 0; i < b.numReqPerCycle; i++ {
		madeProgress = b.bottomUp() || madeProgress
	}

	for i := 0; i < b.numReqPerCycle; i++ {
		madeProgress = b.parseBottom() || madeProgress
	}

	for i := 0; i < b.numReqPerCycle; i++ {
		madeProgress = b.topDown() || madeProgress
	}

	return madeProgress
}

func (b *ReorderBuffer) topDown() bool {
	if b.isFull() {
		return false
	}

	item := b.topPort.PeekIncoming()
	if item == nil {
		return false
	}

	req := item.(mem.AccessReq)
	trans := b.createTransaction(req)

	trans.reqToBottom.Meta().Src = b.bottomPort.AsRemote()
	err := b.bottomPort.Send(trans.reqToBottom)
	if err != nil {
		return false
	}

	b.addTransaction(trans)
	b.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(req, b)
	tracing.TraceReqInitiate(trans.reqToBottom, b,
		tracing.MsgIDAtReceiver(req, b))

	return true
}

func (b *ReorderBuffer) parseBottom() bool {
	item := b.bottomPort.PeekIncoming()
	if item == nil {
		return false
	}

	rsp := item.(mem.AccessRsp)
	rspTo := rsp.GetRspTo()
	transElement, found := b.toBottomReqIDToTransactionTable[rspTo]

	if found {
		trans := transElement.Value.(*transaction)
		trans.rspFromBottom = rsp

		tracing.TraceReqFinalize(trans.reqToBottom, b)
	}

	b.bottomPort.RetrieveIncoming()

	return true
}

func (b *ReorderBuffer) bottomUp() bool {
	elem := b.transactions.Front()
	if elem == nil {
		return false
	}

	trans := elem.Value.(*transaction)
	if trans.rspFromBottom == nil {
		return false
	}

	rsp := b.duplicateRsp(trans.rspFromBottom, trans.reqFromTop.Meta().ID)
	rsp.Meta().Dst = trans.reqFromTop.Meta().Src
	rsp.Meta().Src = b.topPort.AsRemote()

	err := b.topPort.Send(rsp)
	if err != nil {
		return false
	}

	b.deleteTransaction(elem)

	tracing.TraceReqComplete(trans.reqFromTop, b)

	return true
}

func (b *ReorderBuffer) isFull() bool {
	return b.transactions.Len() >= b.bufferSize
}

func (b *ReorderBuffer) createTransaction(req mem.AccessReq) *transaction {
	return &transaction{
		reqFromTop:  req,
		reqToBottom: b.duplicateReq(req),
	}
}

func (b *ReorderBuffer) addTransaction(trans *transaction) {
	elem := b.transactions.PushBack(trans)
	b.toBottomReqIDToTransactionTable[trans.reqToBottom.Meta().ID] = elem
}

func (b *ReorderBuffer) deleteTransaction(elem *list.Element) {
	trans := elem.Value.(*transaction)
	b.transactions.Remove(elem)
	delete(b.toBottomReqIDToTransactionTable, trans.reqToBottom.Meta().ID)
}

func (b *ReorderBuffer) duplicateReq(req mem.AccessReq) mem.AccessReq {
	switch req := req.(type) {
	case *mem.ReadReq:
		return b.duplicateReadReq(req)
	case *mem.WriteReq:
		return b.duplicateWriteReq(req)
	default:
		panic("unsupported type")
	}
}

func (b *ReorderBuffer) duplicateReadReq(req *mem.ReadReq) *mem.ReadReq {
	return mem.ReadReqBuilder{}.
		WithAddress(req.Address).
		WithByteSize(req.AccessByteSize).
		WithPID(req.PID).
		WithDst(b.BottomUnit.AsRemote()).
		Build()
}

func (b *ReorderBuffer) duplicateWriteReq(req *mem.WriteReq) *mem.WriteReq {
	return mem.WriteReqBuilder{}.
		WithAddress(req.Address).
		WithPID(req.PID).
		WithData(req.Data).
		WithDirtyMask(req.DirtyMask).
		WithDst(b.BottomUnit.AsRemote()).
		Build()
}

func (b *ReorderBuffer) duplicateRsp(
	rsp mem.AccessRsp,
	rspTo string,
) mem.AccessRsp {
	switch rsp := rsp.(type) {
	case *mem.DataReadyRsp:
		return b.duplicateDataReadyRsp(rsp, rspTo)
	case *mem.WriteDoneRsp:
		return b.duplicateWriteDoneRsp(rsp, rspTo)
	default:
		panic("type not supported")
	}
}

func (b *ReorderBuffer) duplicateDataReadyRsp(
	rsp *mem.DataReadyRsp,
	rspTo string,
) *mem.DataReadyRsp {
	return mem.DataReadyRspBuilder{}.
		WithData(rsp.Data).
		WithRspTo(rspTo).
		Build()
}

func (b *ReorderBuffer) duplicateWriteDoneRsp(
	rsp *mem.WriteDoneRsp,
	rspTo string,
) *mem.WriteDoneRsp {
	return mem.WriteDoneRspBuilder{}.
		WithRspTo(rspTo).
		Build()
}
