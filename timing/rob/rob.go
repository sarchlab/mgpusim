// Package rob implemented an reorder buffer for memory requests.
package rob

import (
	"fmt"
	// "time"
	"container/list"

	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
)

type transaction struct {
	reqFromTop    mem.AccessReq
	reqToBottom   mem.AccessReq
	rspFromBottom mem.AccessRsp
	reqInTaskID   string
	reqOutTaskID  string
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

	traceWriter *tracing.SQLiteTraceWriter
}

// Function to print all transactions in the ReorderBuffer
func (b *ReorderBuffer) PrintTransactions() {
	fmt.Println("Printing transactions:")
	for element := b.transactions.Front(); element != nil; element = element.Next() {
		fmt.Println(element.Value)
	}
}
func (b *ReorderBuffer) PrintBottomUpTransactions() []string {
	fmt.Println("Printing transactions:")
	waitingTaskIDs := []string{}
	for element := b.transactions.Front(); element != nil; element = element.Next() {
		if transaction, ok := element.Value.(*transaction); ok {
			waitingTaskIDs = append(waitingTaskIDs, transaction.reqFromTop.Meta().ID)
		}
	}
	return waitingTaskIDs;
}

func (b *ReorderBuffer) PrintParseBottomTransactions() []string {
	fmt.Println("Printing transactions:")
	waitingTaskIDs := []string{}
	for element := b.transactions.Front(); element != nil; element = element.Next() {
		if transaction, ok := element.Value.(*transaction); ok {
			waitingTaskIDs = append(waitingTaskIDs, transaction.reqToBottom.Meta().ID)
		}
	}
	return waitingTaskIDs;
}


// Tick updates the status of the ReorderBuffer.
func (b *ReorderBuffer) Tick(now sim.VTimeInSec) (madeProgress bool) {
	madeProgress = b.processControlMsg(now) || madeProgress

	if !b.isFlushing {
		madeProgress = b.runPipeline(now) || madeProgress
	}

	return madeProgress
}

func (b *ReorderBuffer) processControlMsg(
	now sim.VTimeInSec,
) (madeProgress bool) {
	item := b.controlPort.Peek()
	if item == nil {
		return false
	}

	msg := item.(*mem.ControlMsg)
	if msg.DiscardTransations {
		return b.discardTransactions(now, msg)
	} else if msg.Restart {
		return b.restart(now, msg)
	}

	panic("never")
}

func (b *ReorderBuffer) discardTransactions(
	now sim.VTimeInSec,
	msg *mem.ControlMsg,
) (madeProgress bool) {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(b.controlPort).
		WithDst(msg.Src).
		WithSendTime(now).
		ToNotifyDone().
		Build()

	err := b.controlPort.Send(rsp)
	if err != nil {
		return false
	}

	b.isFlushing = true
	b.toBottomReqIDToTransactionTable = make(map[string]*list.Element)
	b.transactions.Init()
	b.controlPort.Retrieve(now)
	return true
}

func (b *ReorderBuffer) restart(
	now sim.VTimeInSec,
	msg *mem.ControlMsg,
) (madeProgress bool) {
	rsp := mem.ControlMsgBuilder{}.
		WithSrc(b.controlPort).
		WithDst(msg.Src).
		WithSendTime(now).
		ToNotifyDone().
		Build()

	err := b.controlPort.Send(rsp)
	if err != nil {
		// tracing.TraceDelay(now, b, "rob")
		// fmt.Printf("Delay, no available control port: %.20f\n", now)
		return false
	}

	b.isFlushing = false
	b.toBottomReqIDToTransactionTable = make(map[string]*list.Element)
	b.transactions.Init()

	for b.topPort.Retrieve(now) != nil {
	}

	for b.bottomPort.Retrieve(now) != nil {
	}

	b.controlPort.Retrieve(now)

	// fmt.Printf("%.10f, %s, rob restarted\n", now, b.Name())

	return true
}

func (b *ReorderBuffer) runPipeline(now sim.VTimeInSec) (madeProgress bool) {
	for i := 0; i < b.numReqPerCycle; i++ {
		madeProgress = b.bottomUp(now) || madeProgress
	}

	for i := 0; i < b.numReqPerCycle; i++ {
		madeProgress = b.parseBottom(now) || madeProgress
	}

	for i := 0; i < b.numReqPerCycle; i++ {
		madeProgress = b.topDown(now) || madeProgress
	}

	return madeProgress
}
func extractIDs(bufferElements []interface{}, portName string) []string {
    ids := []string{}
    for _, elem := range bufferElements {
        if req, ok := elem.(*mem.ReadReq); ok {
            ids = append(ids, req.MsgMeta.ID+"_"+portName)
        }
    }
    return ids
}


func (b *ReorderBuffer) topDown(now sim.VTimeInSec) bool {
	item := b.topPort.Peek()
	if item == nil {
		tracing.TraceDelay(nil, b, b.topPort.Name(), now, "Delay", "idle", "rob")
		fmt.Printf("Delay, no available top port: %.20f\n", now)
		return false
	}

	req := item.(mem.AccessReq)
	if b.isFull() {
		tracing.TraceDelay(req, b, b.topPort.Name(), now, "Delay", "Resources/storage not available", "rob")
		return false
	}
	trans := b.createTransaction(req)

	trans.reqToBottom.Meta().Src = b.bottomPort
	trans.reqToBottom.Meta().SendTime = now
	err := b.bottomPort.Send(trans.reqToBottom)
	if err != nil {
		tracing.TraceDelay(req, b, b.topPort.Name(), now, "Delay", "Port network not available", "rob")
		return false
	}

	b.topPort.Retrieve(now)
	tracing.TraceReqReceive(req, b)
	trans.reqOutTaskID = req.Meta().ID+"_req_out";

	currentProgressID := req.Meta().ID+"_"+b.topPort.Name();
	tracing.TraceProgress(currentProgressID, trans.reqOutTaskID, b, now, "rob", "Port network not available"); // network
	dependentIDs := extractIDs(b.topPort.GetAllBufferElements(), b.topPort.Name());
	tracing.TraceDependency(currentProgressID, b, dependentIDs)


	parentTaskID := tracing.MsgIDAtReceiver(req, b);
	tracing.TraceReqInitiate(trans.reqToBottom, b,
		parentTaskID)
	trans.reqInTaskID = parentTaskID;
	b.addTransaction(trans)

	return true
}

func (b *ReorderBuffer) parseBottom(now sim.VTimeInSec) bool {
	item := b.bottomPort.Peek()
	if item == nil {
		tracing.TraceDelay(nil, b, b.topPort.Name(), now, "Delay", "idle", "rob")
		return false
	}

	rsp := item.(mem.AccessRsp)
	rspTo := rsp.GetRspTo()
	transElement, found := b.toBottomReqIDToTransactionTable[rspTo]

	if found {
		trans := transElement.Value.(*transaction)
		trans.rspFromBottom = rsp
		tracing.TraceReqFinalize(trans.reqToBottom, b) 
		
		transactionProgressID := sim.GetIDGenerator().Generate()
		tracing.TraceProgress(transactionProgressID, trans.reqInTaskID, b, now, "rob", "Data not available") //0506
		dependentIDs := b.PrintParseBottomTransactions();
		tracing.TraceDependency(transactionProgressID, b, dependentIDs);
	}

	b.bottomPort.Retrieve(now)

	return true
}

func (b *ReorderBuffer) bottomUp(now sim.VTimeInSec) bool {
	elem := b.transactions.Front()
	if elem == nil {
		tracing.TraceDelay(nil, b, "", now, "Delay", "idle", "rob")
		fmt.Printf("Delay, no more transactions: %.20f\n", now)
		return false
	}

	trans := elem.Value.(*transaction)
	if trans.rspFromBottom == nil {
		tracing.TraceDelay(trans.reqFromTop, b, b.bottomPort.Name(), now, "Delay", "data not available", "rob")
		// tracing.TraceDelay(now, b, "rob") //TODO: add id, data not available
		return false
	}
	transactionProgressID := sim.GetIDGenerator().Generate();
	tracing.TraceProgress(transactionProgressID, trans.reqInTaskID, b, now, "rob", "Data not available") 


	rsp := b.duplicateRsp(trans.rspFromBottom, trans.reqFromTop.Meta().ID)
	rsp.Meta().Dst = trans.reqFromTop.Meta().Src
	rsp.Meta().Src = b.topPort
	rsp.Meta().SendTime = now

	err := b.topPort.Send(rsp)
	if err != nil {
		// Port network not available
		tracing.TraceDelay(trans.rspFromBottom, b, b.topPort.Name(), now, "Delay", "Port network not available", "rob")
		fmt.Printf("Delay, no available top port: %.20f\n", now)
		return false
	}
	transactionProgressID = sim.GetIDGenerator().Generate();
	tracing.TraceProgress(transactionProgressID, trans.reqInTaskID, b, now, "rob", "Port network not available")
	dependentIDs := b.PrintBottomUpTransactions();
	tracing.TraceDependency(transactionProgressID, b, dependentIDs)
	

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
		WithDst(b.BottomUnit).
		Build()
}

func (b *ReorderBuffer) duplicateWriteReq(req *mem.WriteReq) *mem.WriteReq {
	return mem.WriteReqBuilder{}.
		WithAddress(req.Address).
		WithPID(req.PID).
		WithData(req.Data).
		WithDirtyMask(req.DirtyMask).
		WithDst(b.BottomUnit).
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
