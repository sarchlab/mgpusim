package rdma

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// forwardMiddleware forwards memory requests and responses between the
// inside (local GPU) and outside (remote GPU) ports.
type forwardMiddleware struct {
	comp *Comp
}

func (m *forwardMiddleware) port(name string) messaging.Port {
	switch name {
	case "RDMARequestInside":
		return m.comp.Ports.RDMARequestInside
	case "RDMARequestOutside":
		return m.comp.Ports.RDMARequestOutside
	case "RDMADataInside":
		return m.comp.Ports.RDMADataInside
	case "RDMADataOutside":
		return m.comp.Ports.RDMADataOutside
	default:
		panic("unknown RDMA port: " + name)
	}
}

// Tick forwards as many messages as the per-cycle limits allow.
func (m *forwardMiddleware) Tick() bool {
	madeProgress := false
	spec := m.comp.Spec

	for i := 0; i < spec.OutgoingReqPerCycle; i++ {
		madeProgress = m.processFromL1() || madeProgress
	}

	for i := 0; i < spec.OutgoingRspPerCycle; i++ {
		madeProgress = m.processFromL2() || madeProgress
	}

	for i := 0; i < spec.IncomingReqPerCycle; i++ {
		madeProgress = m.processIncomingReq() || madeProgress
	}

	for i := 0; i < spec.IncomingRspPerCycle; i++ {
		madeProgress = m.processIncomingRsp() || madeProgress
	}

	return madeProgress
}

// processFromL1 forwards a request from the local L1 caches to the RDMA
// engine of the GPU that owns the address.
func (m *forwardMiddleware) processFromL1() bool {
	if m.comp.State.PauseIncomingReqsFromL1 {
		return false
	}

	inPort := m.port("RDMARequestInside")
	item, ok := inPort.PeekIncoming()
	if !ok {
		return false
	}

	req := item

	outPort := m.port("RDMARequestOutside")
	if !outPort.CanSend() {
		return false
	}

	dst := m.comp.Spec.RemoteRDMAAddressTable.Find(requestAddress(req))
	cloned := cloneReq(m.comp.NewID(), req, outPort.AsRemote(), dst)

	trans := m.startTransaction(req, cloned)
	outPort.Send(cloned)
	inPort.RetrieveIncoming()

	m.comp.State.TransactionsFromInside = append(
		m.comp.State.TransactionsFromInside,
		trans)

	return true
}

// processIncomingRsp forwards a response from a remote GPU back to the local
// requester that initiated the transaction.
func (m *forwardMiddleware) processIncomingRsp() bool {
	outPort := m.port("RDMARequestOutside")
	item, ok := outPort.PeekIncoming()
	if !ok {
		return false
	}

	rsp := item

	state := &m.comp.State
	index := findTransactionByRspTo(
		rsp.RspTo, state.TransactionsFromInside)
	trans := state.TransactionsFromInside[index]

	// The remote response has arrived: end the remote round-trip wait now,
	// before the egress-port backpressure below, so the wait to forward the
	// response is not charged to the remote interval. Guard so an egress-full
	// retry does not re-emit.
	if !trans.RemoteMilestoneEmitted {
		m.markRemoteRoundTripDone(trans)
		state.TransactionsFromInside[index].RemoteMilestoneEmitted = true
	}

	inPort := m.port("RDMARequestInside")
	if !inPort.CanSend() {
		return false
	}

	inPort.Send(cloneRsp(
		m.comp.NewID(), rsp, inPort.AsRemote(), trans.OriginalSrc, trans.OriginalReqID))
	outPort.RetrieveIncoming()

	m.endTransaction(trans)
	state.TransactionsFromInside = append(
		state.TransactionsFromInside[:index],
		state.TransactionsFromInside[index+1:]...)

	return true
}

// processIncomingReq forwards a request from a remote GPU to the local module
// that owns the address.
func (m *forwardMiddleware) processIncomingReq() bool {
	outPort := m.port("RDMADataOutside")
	item, ok := outPort.PeekIncoming()
	if !ok {
		return false
	}

	req := item

	inPort := m.port("RDMADataInside")
	if !inPort.CanSend() {
		return false
	}

	dst := m.comp.Spec.LocalModules.Find(requestAddress(req))
	cloned := cloneReq(m.comp.NewID(), req, inPort.AsRemote(), dst)

	trans := m.startTransaction(req, cloned)
	inPort.Send(cloned)
	outPort.RetrieveIncoming()

	m.comp.State.TransactionsFromOutside = append(
		m.comp.State.TransactionsFromOutside,
		trans)

	return true
}

// processFromL2 forwards a response from the local memory modules back to the
// remote GPU that initiated the transaction.
func (m *forwardMiddleware) processFromL2() bool {
	inPort := m.port("RDMADataInside")
	item, ok := inPort.PeekIncoming()
	if !ok {
		return false
	}

	rsp := item

	state := &m.comp.State
	index := findTransactionByRspTo(
		rsp.RspTo, state.TransactionsFromOutside)
	trans := state.TransactionsFromOutside[index]

	// The remote response has arrived: end the remote round-trip wait now,
	// before the egress-port backpressure below, so the wait to forward the
	// response is not charged to the remote interval. Guard so an egress-full
	// retry does not re-emit.
	if !trans.RemoteMilestoneEmitted {
		m.markRemoteRoundTripDone(trans)
		state.TransactionsFromOutside[index].RemoteMilestoneEmitted = true
	}

	outPort := m.port("RDMADataOutside")
	if !outPort.CanSend() {
		return false
	}

	outPort.Send(cloneRsp(
		m.comp.NewID(), rsp, outPort.AsRemote(), trans.OriginalSrc, trans.OriginalReqID))
	inPort.RetrieveIncoming()

	m.endTransaction(trans)
	state.TransactionsFromOutside = append(
		state.TransactionsFromOutside[:index],
		state.TransactionsFromOutside[index+1:]...)

	return true
}

// startTransaction starts the tracing tasks for a forwarded request and
// returns the transaction that records it.
func (m *forwardMiddleware) startTransaction(
	req, cloned messaging.Msg,
) transaction {
	tracker := m.comp.TaskTracker()
	recvTaskID := tracker.StartReqIn(m.comp, req)
	sendTaskID := tracker.StartReqOut(m.comp, cloned, recvTaskID)

	return transaction{
		OriginalReqID:  req.ID,
		OriginalSrc:    req.Src,
		ForwardedReqID: cloned.ID,
		RecvTaskID:     recvTaskID,
		SendTaskID:     sendTaskID,
	}
}

// markRemoteRoundTripDone emits the "remote" data milestone for a transaction
// when its response first arrives — before any egress-port backpressure — so
// the wait to forward the response is not charged to the remote round-trip
// interval. Callers guard against re-emission via Transaction.RemoteMilestoneEmitted.
func (m *forwardMiddleware) markRemoteRoundTripDone(trans transaction) {
	m.comp.TaskTracker().
		AddMilestone(tasks.Milestone{TaskID: trans.RecvTaskID, Kind: tasks.MilestoneKindData, What: "remote"})
}

// endTransaction ends the tracing tasks for a completed transaction.
func (m *forwardMiddleware) endTransaction(trans transaction) {
	m.comp.TaskTracker().EndReqOut(trans.SendTaskID)
	m.comp.TaskTracker().EndReqIn(trans.RecvTaskID)
}

func findTransactionByRspTo(rspTo uint64, transactions []transaction) int {
	for i, trans := range transactions {
		if trans.ForwardedReqID == rspTo {
			return i
		}
	}

	log.Panicf("transaction responding to msg %d not found", rspTo)

	return 0
}

func requestAddress(req messaging.Msg) uint64 {
	switch p := req.Payload.(type) {
	case memprotocol.ReadReq:
		return p.Address
	case memprotocol.WriteReq:
		return p.Address
	default:
		panic(fmt.Sprintf("unexpected RDMA request %T", p))
	}
}
func cloneReq(id uint64, origin messaging.Msg, src, dst messaging.RemotePort) messaging.Msg {
	requestAddress(origin)
	origin.ID, origin.Src, origin.Dst, origin.RspTo = id, src, dst, 0
	return origin
}
func cloneRsp(id uint64, origin messaging.Msg, src, dst messaging.RemotePort, rspTo uint64) messaging.Msg {
	switch origin.Payload.(type) {
	case memprotocol.DataReadyRsp, memprotocol.WriteDoneRsp:
	default:
		panic(fmt.Sprintf("unexpected RDMA response %T", origin.Payload))
	}
	origin.ID, origin.Src, origin.Dst, origin.RspTo = id, src, dst, rspTo
	return origin
}
func (m *forwardMiddleware) Handle(timing.Event) bool { return m.Tick() }
