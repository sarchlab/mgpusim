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

	// fmt.Printf("%.10f, %s, rob flushed\n", now, b.Name())

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
		// 0506 b.topPort.Name() -> b.Name()+".Buffer"
		return false
	}
	// 加progress 记时间
	trans := b.createTransaction(req)

	trans.reqToBottom.Meta().Src = b.bottomPort
	trans.reqToBottom.Meta().SendTime = now
	err := b.bottomPort.Send(trans.reqToBottom)
	// 0506 这是send to bottomPort的意思 ，而不是 从 bottomPort send吧？
	if err != nil {
		tracing.TraceDelay(req, b, b.topPort.Name(), now, "Delay", "Port network not available", "rob")
		return false
	}

	b.topPort.Retrieve(now)
	tracing.TraceReqReceive(req, b)
	// 0506 一个新的req_in 产生了！但它的req_out什么时候出现呢？
	trans.reqOutTaskID = req.Meta().ID+"_req_out";

	currentProgressID := req.Meta().ID+"_"+b.topPort.Name();
	tracing.TraceProgress(currentProgressID, trans.reqOutTaskID, b, now, "rob", "Port network not available"); // network
	dependentIDs := extractIDs(b.topPort.GetAllBufferElements(), b.topPort.Name());
	tracing.TraceDependency(currentProgressID, b, dependentIDs)


	parentTaskID := tracing.MsgIDAtReceiver(req, b); // 0506 这是个req_in
	tracing.TraceReqInitiate(trans.reqToBottom, b,
		parentTaskID)
	// taskID := tracing.TraceReqInitiate(trans.reqToBottom, b,
	// 	parentTaskID)
	// 它的req_out 出现了！但这其中似乎没有任何delay的空间？
	trans.reqInTaskID = parentTaskID;
	
	//0506 从 req_in发下去了，并req_out那条bar也建立了
	// progressID := req.Meta().ID+"_"+b.topPort.Name();
	// tracing.TraceProgress(progressID, trans.reqInTaskID, b, now, "rob", "Initiate request");
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
	rspTo := rsp.GetRspTo() //0506 ID of corresponding task that would receive the response
	transElement, found := b.toBottomReqIDToTransactionTable[rspTo]

	if found {
		//0506 这里found了 所以就把这个transaction完成
		trans := transElement.Value.(*transaction)
		trans.rspFromBottom = rsp
		tracing.TraceReqFinalize(trans.reqToBottom, b) 
		// taskID := tracing.TraceReqFinalize(trans.reqToBottom, b) 
		// 0506 terminates the message task。sender receive response那这算是 发出response的task还是收到response的task的step呢？
		// receiverTaskID := fmt.Sprintf("%s@%s", rspTo, b.Name()); // 0506 cannot directly use tracing.MsgIDAtReceiver(rsp, b); 因为是rspTo 在receive
		
		transactionProgressID := sim.GetIDGenerator().Generate()
		// tracing.TraceProgress(transactionProgressID, trans.reqOutTaskID, b, now, "rob") 
		tracing.TraceProgress(transactionProgressID, trans.reqInTaskID, b, now, "rob", "Data not available") //0506
		dependentIDs := b.PrintParseBottomTransactions();
		tracing.TraceDependency(transactionProgressID, b, dependentIDs);

		//????
	}
	// 0506 那如果没有 found呢？就不用end message了？因为没有message task可end？就看看bottom port里有什么就拿什么
	// 没found,就被discard (page migration相关 不用考虑)

	b.bottomPort.Retrieve(now)
	// msg := b.bottomPort.Retrieve(now)
	// 这里是说 bottomPort就紧接着那一个新的任务可以下次输出,所以这里不能算一个progress
	// 0506 这里和上面的 b.topPort.Retrieve(now)对比 为什么retrieve之后没有再建一个新的task？难道是完成了当时的req_out task?
	// 0506 为什么msg.RespondTo() 的ID和rspTo的id不一样？
	// 查询了一下trace结果，好像并没有msg.RespondTo()的ID对应的task


	// tracing.TraceDelay(rsp, b, b.bottomPort.Name(), now, "Step", "", "rob")
	// dependentIDs := extractIDs(b.bottomPort.GetAllBufferElements());
	// tracing.TraceDependency(rsp, b, dependentIDs)

	// bottomPortProgressID := sim.GetIDGenerator().Generate();
	// tracing.TraceProgress(bottomPortProgressID, "", b, now, "rob")
	// dependentIDs := extractIDs(b.bottomPort.GetAllBufferElements(), b.bottomPort.Name());
	// tracing.TraceDependency(bottomPortProgressID, b, dependentIDs)

	// dependeIDs2 := b.PrintParseBottomTransactions();
	// tracing.TraceDependency(rsp, b, dependeIDs2)


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
		tracing.TraceDelay(trans.reqFromTop, b, b.bottomPort.Name(), now, "Delay", "data not available", "rob") //???
		// tracing.TraceDelay(now, b, "rob") // Jijie: add id, data not available
		return false
	}
	transactionProgressID := sim.GetIDGenerator().Generate();
	tracing.TraceProgress(transactionProgressID, trans.reqInTaskID, b, now, "rob", "Data not available") // 改成记录第一次成功的时间， 判断有没有记录过

	//0506有来自bottomport的response了，可以往上发了

	rsp := b.duplicateRsp(trans.rspFromBottom, trans.reqFromTop.Meta().ID)
	rsp.Meta().Dst = trans.reqFromTop.Meta().Src
	rsp.Meta().Src = b.topPort
	rsp.Meta().SendTime = now

	err := b.topPort.Send(rsp)
	//0506 应该要往上发了 从topPort send出去？ 不是应该send 到topPort吗?
	if err != nil {
		// Port network not available
		tracing.TraceDelay(trans.rspFromBottom, b, b.topPort.Name(), now, "Delay", "Port network not available", "rob") //???
		fmt.Printf("Delay, no available top port: %.20f\n", now)
		return false
	}
	//0506下面往上发完了 ？ 上面收完了？上面发完了？
	// tracing.TraceDelay(trans.rspFromBottom, b, "", now, "Step", "", "rob") //  b.topPort.Name() 变成空着 0318 done
	transactionProgressID = sim.GetIDGenerator().Generate();
	// tracing.TraceProgress(transactionProgressID,  trans.reqOutTaskID, b, now, "rob")
	// receiverTaskID := rsp.GetRspTo();
	// tracing.TraceProgress(transactionProgressID, receiverTaskID, b, now, "rob")
	tracing.TraceProgress(transactionProgressID, trans.reqInTaskID, b, now, "rob", "Port network not available")//050666
	dependentIDs := b.PrintBottomUpTransactions();
	// dependentIDs := extractIDs(b.topPort.GetAllBufferElements()); // b.transactions Jijie todo 0318 遍历 并且反过来
	tracing.TraceDependency(transactionProgressID, b, dependentIDs)
	

	b.deleteTransaction(elem)
	tracing.TraceReqComplete(trans.reqFromTop, b)
	// 0506此transaction正式完结，需要一个额外的progress吗？


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
