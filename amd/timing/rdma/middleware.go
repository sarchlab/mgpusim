package rdma

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
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
	madeProgress := m.sendNotifications()
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

	for i := 0; i < spec.MigrationRspPerCycle; i++ {
		madeProgress = m.processMigrationRsp() || madeProgress
	}

	for i := 0; i < spec.MigrationReqPerCycle; i++ {
		madeProgress = m.processMigrationReq() || madeProgress
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
	m.countAccess(requestAddress(req))

	return true
}

// countAccess counts a remote demand access of this GPU to the physical page
// frame of addr. When the frame reaches the threshold, its counter is cleared
// and a notification with the address of the frame is queued. A count
// belongs to the frame, not to the page that it holds: it survives the frame
// being freed and allocated again, and the driver resolves the frame when it
// services the notification.
func (m *forwardMiddleware) countAccess(addr uint64) {
	spec := m.comp.Spec
	if spec.AccessCounterHandler == "" || spec.AccessCounterThreshold == 0 {
		return
	}

	state := &m.comp.State
	if state.AccessCounts == nil {
		state.AccessCounts = make(map[uint64]uint64)
	}

	frame := addr >> spec.Log2PageSize
	state.AccessCounts[frame]++

	if state.AccessCounts[frame] < spec.AccessCounterThreshold {
		return
	}

	delete(state.AccessCounts, frame)
	state.PendingNotifications = append(state.PendingNotifications,
		messaging.Msg{
			ID:  m.comp.NewID(),
			Src: m.comp.Ports.AccessCounter.AsRemote(),
			Dst: spec.AccessCounterHandler,
			Payload: protocol.AccessCounterNotification{
				PAddr:    frame << spec.Log2PageSize,
				DeviceID: spec.DeviceID,
			},
		})
}

// sendNotifications sends the queued access-counter notifications. Requests
// never wait for them.
func (m *forwardMiddleware) sendNotifications() bool {
	state := &m.comp.State
	port := m.comp.Ports.AccessCounter
	madeProgress := false

	for len(state.PendingNotifications) > 0 && port.CanSend() {
		port.Send(state.PendingNotifications[0])
		state.PendingNotifications = state.PendingNotifications[1:]
		madeProgress = true
	}

	return madeProgress
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
	return m.forwardReqInward(
		m.port("RDMADataOutside"), m.port("RDMADataInside"),
		m.comp.Spec.LocalModules, &m.comp.State.TransactionsFromOutside)
}

// processFromL2 forwards a response from the local memory modules back to the
// remote GPU that initiated the transaction.
func (m *forwardMiddleware) processFromL2() bool {
	return m.forwardRspOutward(
		m.port("RDMADataInside"), m.port("RDMADataOutside"),
		&m.comp.State.TransactionsFromOutside)
}

// processMigrationReq forwards a read or a write of another GPU's data mover
// to the local memory below the L2 caches.
func (m *forwardMiddleware) processMigrationReq() bool {
	return m.forwardReqInward(
		m.comp.Ports.MigrationOutside, m.comp.Ports.MigrationInside,
		m.comp.Spec.MigrationModules, &m.comp.State.TransactionsFromMigration)
}

// processMigrationRsp forwards a response of the memory below the L2 caches
// back to the data mover.
func (m *forwardMiddleware) processMigrationRsp() bool {
	return m.forwardRspOutward(
		m.comp.Ports.MigrationInside, m.comp.Ports.MigrationOutside,
		&m.comp.State.TransactionsFromMigration)
}

// forwardReqInward forwards a request that arrived on outPort to the local
// module that modules selects, through inPort, and records it in
// transactions.
func (m *forwardMiddleware) forwardReqInward(
	outPort, inPort messaging.Port,
	modules mem.AddressMapper,
	transactions *[]transaction,
) bool {
	req, ok := outPort.PeekIncoming()
	if !ok {
		return false
	}

	if !inPort.CanSend() {
		return false
	}

	dst := modules.Find(requestAddress(req))
	cloned := cloneReq(m.comp.NewID(), req, inPort.AsRemote(), dst)

	trans := m.startTransaction(req, cloned)
	inPort.Send(cloned)
	outPort.RetrieveIncoming()

	*transactions = append(*transactions, trans)

	return true
}

// forwardRspOutward forwards a response that arrived on inPort back to the
// requester of its transaction, through outPort.
func (m *forwardMiddleware) forwardRspOutward(
	inPort, outPort messaging.Port,
	transactions *[]transaction,
) bool {
	rsp, ok := inPort.PeekIncoming()
	if !ok {
		return false
	}

	index := findTransactionByRspTo(rsp.RspTo, *transactions)
	trans := (*transactions)[index]

	// The remote response has arrived: end the remote round-trip wait now,
	// before the egress-port backpressure below, so the wait to forward the
	// response is not charged to the remote interval. Guard so an egress-full
	// retry does not re-emit.
	if !trans.RemoteMilestoneEmitted {
		m.markRemoteRoundTripDone(trans)
		(*transactions)[index].RemoteMilestoneEmitted = true
	}

	if !outPort.CanSend() {
		return false
	}

	outPort.Send(cloneRsp(
		m.comp.NewID(), rsp, outPort.AsRemote(), trans.OriginalSrc, trans.OriginalReqID))
	inPort.RetrieveIncoming()

	m.endTransaction(trans)
	*transactions = append((*transactions)[:index], (*transactions)[index+1:]...)

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
