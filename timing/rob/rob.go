// Package rob implemented an reorder buffer for memory requests.
package rob

import (
	"container/list"
	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"fmt"
	"encoding/csv"
	"sync"
    "os"
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

type Milestone struct {
	ID string
	TaskID string
	BlockingCategory string
	BlockingReason string
	BlockingLocation string
	Timestamp  sim.VTimeInSec
}

func (m *MilestoneManager) AddMilestone(
	taskID           string,
    blockingCategory string,
    blockingReason   string,
    blockingLocation string,
    timestamp        sim.VTimeInSec,
) {
	m.mutex.Lock()
    defer m.mutex.Unlock()
	milestone := Milestone {
		ID:               fmt.Sprintf("milestone_%d", len(m.milestones)+1),
        TaskID:           taskID,
        BlockingCategory: blockingCategory,
        BlockingReason:   blockingReason,
        BlockingLocation: blockingLocation,
        Timestamp:        timestamp,
	}
	m.milestones = append(m.milestones, milestone)
	fmt.Printf("Added milestone: %+v\n", milestone)
}

type MilestoneManager struct {
    milestones []Milestone
    mutex      sync.Mutex
}

var GlobalMilestoneManager = &MilestoneManager{
	milestones: make([]Milestone, 0),
}

func (m *MilestoneManager) GetMilestones() []Milestone {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    return m.milestones
}

func (b *ReorderBuffer) getTaskID() string {
    if b.transactions.Len() == 0 {
        return ""
    }

    trans := b.transactions.Front().Value.(*transaction)

    if trans.reqFromTop != nil {
        return trans.reqFromTop.Meta().ID
    }

    if trans.reqToBottom != nil {
        return trans.reqFromTop.Meta().ID
    }

    return ""
}


// Tick updates the status of the ReorderBuffer.
func (b *ReorderBuffer) Tick(now sim.VTimeInSec) (madeProgress bool) {
	madeProgress = b.processControlMsg(now) || madeProgress

	if !b.isFlushing {
		madeProgress = b.runPipeline(now) || madeProgress
	}
	// b.ExportMilestonesToCSV("../samples/fir/milestones.csv")
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

func (b *ReorderBuffer) topDown(now sim.VTimeInSec) bool {
    if b.isFull() {
		GlobalMilestoneManager.AddMilestone(
			b.getTaskID(),
			"Hardware Occupancy",
			"Buffer full",
			"topDown",
			now,
		)
        return false
    }

	item := b.topPort.Peek()
	if item == nil {
		return false
	}

	req := item.(mem.AccessReq)
	trans := b.createTransaction(req)

	trans.reqToBottom.Meta().Src = b.bottomPort
	trans.reqToBottom.Meta().SendTime = now
	err := b.bottomPort.Send(trans.reqToBottom)
    if err != nil {
        GlobalMilestoneManager.AddMilestone(
            b.getTaskID(),
            "Network Error",
            "Unable to send request to bottom port",
            "topDown",
            now,
        )
        return false
    }
	
	b.addTransaction(trans)
	b.topPort.Retrieve(now)

	tracing.TraceReqReceive(req, b)
	tracing.TraceReqInitiate(trans.reqToBottom, b,
		tracing.MsgIDAtReceiver(req, b))

	return true
}

func (b *ReorderBuffer) parseBottom(now sim.VTimeInSec) bool {
	item := b.bottomPort.Peek()
    if item == nil {
        GlobalMilestoneManager.AddMilestone(
            b.getTaskID(),
            "Dependency",
            "Waiting for bottom response",
            "parseBottom",
            now,
        )
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

	b.bottomPort.Retrieve(now)

	return true
}

func (b *ReorderBuffer) bottomUp(now sim.VTimeInSec) bool {
	elem := b.transactions.Front()
    if elem == nil {
        GlobalMilestoneManager.AddMilestone(
            b.getTaskID(),
            "Dependency",
            "No transactions to process",
            "bottomUp",
            now,
        )
        return false
    }

	trans := elem.Value.(*transaction)
	if trans.rspFromBottom == nil {
		GlobalMilestoneManager.AddMilestone(
            b.getTaskID(),
            "Dependency",
            "Waiting for bottom response",
            "bottomUp",
            now,
        )
		return false
	}

	rsp := b.duplicateRsp(trans.rspFromBottom, trans.reqFromTop.Meta().ID)
	rsp.Meta().Dst = trans.reqFromTop.Meta().Src
	rsp.Meta().Src = b.topPort
	rsp.Meta().SendTime = now

	err := b.topPort.Send(rsp)
    if err != nil {
        GlobalMilestoneManager.AddMilestone(
            b.getTaskID(),
            "Network Error",
            "Unable to send request to bottom port",
            "bottomUp",
            now,
        )
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

func (m *MilestoneManager) ExportMilestonesToCSV(filename string) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    for _, milestone := range m.milestones {
        fmt.Printf("ID: %s, TaskID: %s, BlockingCategory: %s, BlockingReason: %s, BlockingLocation: %s, Timestamp: %v\n",
            milestone.ID, milestone.TaskID, milestone.BlockingCategory, milestone.BlockingReason, milestone.BlockingLocation, milestone.Timestamp)
    }

    file, err := os.Create(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    writer := csv.NewWriter(file)
    defer writer.Flush()

    headers := []string{"ID", "TaskID", "BlockingCategory", "BlockingReason", "BlockingLocation", "Timestamp"}
    if err := writer.Write(headers); err != nil {
        return err
    }

    for _, m := range m.milestones {
        record := []string{
            m.ID,
            m.TaskID,
            m.BlockingCategory,
            m.BlockingReason,
            m.BlockingLocation,
            fmt.Sprintf("%v", m.Timestamp),
        }
        if err := writer.Write(record); err != nil {
            return err
        }
    }

    return nil
}
