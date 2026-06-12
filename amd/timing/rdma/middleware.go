package rdma

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// forwardMiddleware forwards memory requests and responses between the
// inside (local GPU) and outside (remote GPU) ports.
type forwardMiddleware struct {
	comp *Comp
}

func (m *forwardMiddleware) port(name string) messaging.Port {
	return m.comp.GetPortByName(name)
}

// Tick forwards as many messages as the per-cycle limits allow.
func (m *forwardMiddleware) Tick() bool {
	madeProgress := false
	spec := m.comp.Spec()

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
	item := inPort.PeekIncoming()
	if item == nil {
		return false
	}

	req, ok := item.(memprotocol.AccessReq)
	if !ok {
		log.Panicf("cannot process request of type %s", reflect.TypeOf(item))
	}

	outPort := m.port("RDMARequestOutside")
	if !outPort.CanSend() {
		return false
	}

	dst := m.comp.Resources().RemoteRDMAAddressTable.Find(req.GetAddress())
	cloned := cloneReq(req, outPort.AsRemote(), dst)

	outPort.Send(cloned)
	inPort.RetrieveIncoming()

	m.comp.State.TransactionsFromInside = append(
		m.comp.State.TransactionsFromInside,
		m.startTransaction(req, cloned))

	return true
}

// processIncomingRsp forwards a response from a remote GPU back to the local
// requester that initiated the transaction.
func (m *forwardMiddleware) processIncomingRsp() bool {
	outPort := m.port("RDMARequestOutside")
	item := outPort.PeekIncoming()
	if item == nil {
		return false
	}

	rsp, ok := item.(memprotocol.AccessRsp)
	if !ok {
		log.Panicf("cannot process request of type %s", reflect.TypeOf(item))
	}

	state := &m.comp.State
	index := findTransactionByRspTo(
		rsp.Meta().RspTo, state.TransactionsFromInside)
	trans := state.TransactionsFromInside[index]

	inPort := m.port("RDMARequestInside")
	if !inPort.CanSend() {
		return false
	}

	inPort.Send(cloneRsp(
		rsp, inPort.AsRemote(), trans.OriginalSrc, trans.OriginalReqID))
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
	item := outPort.PeekIncoming()
	if item == nil {
		return false
	}

	req, ok := item.(memprotocol.AccessReq)
	if !ok {
		log.Panicf("cannot process request of type %s", reflect.TypeOf(item))
	}

	inPort := m.port("RDMADataInside")
	if !inPort.CanSend() {
		return false
	}

	dst := m.comp.Resources().LocalModules.Find(req.GetAddress())
	cloned := cloneReq(req, inPort.AsRemote(), dst)

	inPort.Send(cloned)
	outPort.RetrieveIncoming()

	m.comp.State.TransactionsFromOutside = append(
		m.comp.State.TransactionsFromOutside,
		m.startTransaction(req, cloned))

	return true
}

// processFromL2 forwards a response from the local memory modules back to the
// remote GPU that initiated the transaction.
func (m *forwardMiddleware) processFromL2() bool {
	inPort := m.port("RDMADataInside")
	item := inPort.PeekIncoming()
	if item == nil {
		return false
	}

	rsp, ok := item.(memprotocol.AccessRsp)
	if !ok {
		log.Panicf("cannot process request of type %s", reflect.TypeOf(item))
	}

	state := &m.comp.State
	index := findTransactionByRspTo(
		rsp.Meta().RspTo, state.TransactionsFromOutside)
	trans := state.TransactionsFromOutside[index]

	outPort := m.port("RDMADataOutside")
	if !outPort.CanSend() {
		return false
	}

	outPort.Send(cloneRsp(
		rsp, outPort.AsRemote(), trans.OriginalSrc, trans.OriginalReqID))
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
	req, cloned memprotocol.AccessReq,
) transaction {
	recvTaskID := tracing.MsgIDAtReceiver(req, m.comp)
	tracing.TraceReqReceive(m.comp, req)
	tracing.TraceReqInitiate(m.comp, cloned, recvTaskID)

	return transaction{
		OriginalReqID:  req.Meta().ID,
		OriginalSrc:    req.Meta().Src,
		ForwardedReqID: cloned.Meta().ID,
		RecvTaskID:     recvTaskID,
	}
}

// endTransaction ends the tracing tasks for a completed transaction.
func (m *forwardMiddleware) endTransaction(trans transaction) {
	if m.comp.NumHooks() == 0 {
		return
	}

	tracing.EndTask(m.comp, tracing.TaskEnd{ID: trans.ForwardedReqID})
	tracing.EndTask(m.comp, tracing.TaskEnd{ID: trans.RecvTaskID})
	tracing.ForgetMsgIDAtReceiver(trans.OriginalReqID, m.comp)
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

func cloneReq(
	origin memprotocol.AccessReq,
	src, dst messaging.RemotePort,
) memprotocol.AccessReq {
	meta := messaging.MsgMeta{
		ID:           timing.GetIDGenerator().Generate(),
		Src:          src,
		Dst:          dst,
		TrafficClass: origin.Meta().TrafficClass,
		TrafficBytes: origin.Meta().TrafficBytes,
	}

	switch origin := origin.(type) {
	case memprotocol.ReadReq:
		return memprotocol.ReadReq{
			MsgMeta:        meta,
			Address:        origin.Address,
			AccessByteSize: origin.AccessByteSize,
			PID:            origin.PID,
		}
	case memprotocol.WriteReq:
		return memprotocol.WriteReq{
			MsgMeta:   meta,
			Address:   origin.Address,
			Data:      origin.Data,
			DirtyMask: origin.DirtyMask,
			PID:       origin.PID,
		}
	default:
		log.Panicf("cannot clone request of type %s", reflect.TypeOf(origin))
		return nil
	}
}

func cloneRsp(
	origin memprotocol.AccessRsp,
	src, dst messaging.RemotePort,
	rspTo uint64,
) memprotocol.AccessRsp {
	meta := messaging.MsgMeta{
		ID:           timing.GetIDGenerator().Generate(),
		Src:          src,
		Dst:          dst,
		RspTo:        rspTo,
		TrafficClass: origin.Meta().TrafficClass,
		TrafficBytes: origin.Meta().TrafficBytes,
	}

	switch origin := origin.(type) {
	case memprotocol.DataReadyRsp:
		return memprotocol.DataReadyRsp{
			MsgMeta: meta,
			Data:    origin.Data,
		}
	case memprotocol.WriteDoneRsp:
		return memprotocol.WriteDoneRsp{
			MsgMeta: meta,
		}
	default:
		log.Panicf("cannot clone response of type %s", reflect.TypeOf(origin))
		return nil
	}
}
