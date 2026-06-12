package rob

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// middleware implements the reorder buffer behavior: the data pipeline
// (topDown, parseBottom, bottomUp) and the control protocol.
type middleware struct {
	comp *Comp

	// Cached port lookups. The Top port additionally carries the milestone
	// port hook (see porthook.go), attached on first access.
	top    messaging.Port
	bottom messaging.Port
	ctrl   messaging.Port
}

func (m *middleware) topPort() messaging.Port {
	if m.top == nil {
		m.top = m.comp.GetPortByName("Top")
		// The v4 builder attached the milestone hook when it created the
		// top port. In v5 ports are assigned externally after Build, so the
		// hook is attached lazily on first use instead.
		m.top.AcceptHook(&portHook{})
	}

	return m.top
}

func (m *middleware) bottomPort() messaging.Port {
	if m.bottom == nil {
		m.bottom = m.comp.GetPortByName("Bottom")
	}

	return m.bottom
}

func (m *middleware) ctrlPort() messaging.Port {
	if m.ctrl == nil {
		m.ctrl = m.comp.GetPortByName("Control")
	}

	return m.ctrl
}

// Tick updates the status of the ReorderBuffer. The control port is serviced
// before the data path so a synchronous control verb takes effect in the same
// tick. The pipeline runs while Enabled or Draining (draining lets in-flight
// transactions retire); it is frozen while Paused.
func (m *middleware) Tick() bool {
	madeProgress := false

	madeProgress = m.processControlMsg() || madeProgress

	switch m.comp.State.ControlState {
	case memcontrolprotocol.StateEnabled, memcontrolprotocol.StateDraining:
		madeProgress = m.runPipeline() || madeProgress
	}

	return madeProgress
}

func (m *middleware) runPipeline() bool {
	madeProgress := false
	numReqPerCycle := m.comp.Spec().NumReqPerCycle

	for i := 0; i < numReqPerCycle; i++ {
		madeProgress = m.bottomUp() || madeProgress
	}

	for i := 0; i < numReqPerCycle; i++ {
		madeProgress = m.parseBottom() || madeProgress
	}

	for i := 0; i < numReqPerCycle; i++ {
		madeProgress = m.topDown() || madeProgress
	}

	return madeProgress
}

// topDown accepts a request from the Top port, forwards a duplicated request
// to the bottom unit, and records the transaction in FIFO order. New traffic
// is only accepted while Enabled.
func (m *middleware) topDown() bool {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateEnabled {
		return false
	}

	item := m.topPort().PeekIncoming()
	if item == nil {
		return false
	}

	if m.isFull() {
		return false
	}

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(item, m.comp),
		Kind:   tracing.MilestoneKindHardwareResource,
		What:   m.comp.Name() + ".Buffer",
	})

	req, ok := item.(memprotocol.AccessReq)
	if !ok {
		panic("rob: unsupported top-port message type")
	}

	reqToBottom, isRead := m.duplicateReq(req)

	if !m.bottomPort().CanSend() {
		return false
	}

	m.bottomPort().Send(reqToBottom)

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(item, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.bottomPort().Name(),
	})

	state.Transactions = append(state.Transactions, transactionState{
		ReqFromTopID:  req.Meta().ID,
		ReqFromTopSrc: req.Meta().Src,
		ReqToBottomID: reqToBottom.Meta().ID,
		IsRead:        isRead,
	})
	m.topPort().RetrieveIncoming()

	tracing.TraceReqReceive(m.comp, req)
	tracing.TraceReqInitiate(m.comp, reqToBottom,
		tracing.MsgIDAtReceiver(req, m.comp))

	return true
}

// parseBottom records the response that came back from the bottom unit on its
// matching transaction. Responses that match no transaction (e.g., left over
// after a flush) are dropped.
func (m *middleware) parseBottom() bool {
	item := m.bottomPort().PeekIncoming()
	if item == nil {
		return false
	}

	var (
		rspTo   uint64
		isRead  bool
		rspData []byte
	)

	switch rsp := item.(type) {
	case memprotocol.DataReadyRsp:
		rspTo = rsp.RspTo
		isRead = true
		rspData = rsp.Data
	case memprotocol.WriteDoneRsp:
		rspTo = rsp.RspTo
	default:
		panic("rob: unsupported bottom-port message type")
	}

	idx := m.findTransactionByBottomID(rspTo)
	if idx >= 0 {
		trans := &m.comp.State.Transactions[idx]
		trans.HasRsp = true
		if isRead {
			trans.RspData = rspData
		}

		tracing.TraceReqFinalize(m.comp, m.bottomReqTraceMsg(*trans))
	}

	m.bottomPort().RetrieveIncoming()

	return true
}

// bottomUp releases the head-of-line transaction once its bottom-unit
// response has arrived, forwarding a duplicated response to the original
// requester.
func (m *middleware) bottomUp() bool {
	state := &m.comp.State
	if len(state.Transactions) == 0 {
		return false
	}

	head := state.Transactions[0]
	if !head.HasRsp {
		return false
	}

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(m.topReqTraceMsg(head), m.comp),
		Kind:   tracing.MilestoneKindData,
		What:   "data",
	})

	rsp := m.duplicateRsp(head)

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)

	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(m.topReqTraceMsg(head), m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.topPort().Name(),
	})

	state.Transactions = state.Transactions[1:]

	tracing.TraceReqComplete(m.comp, m.topReqTraceMsg(head))

	return true
}

func (m *middleware) isFull() bool {
	return len(m.comp.State.Transactions) >= m.comp.Spec().BufferSize
}

func (m *middleware) findTransactionByBottomID(id uint64) int {
	for i := range m.comp.State.Transactions {
		if m.comp.State.Transactions[i].ReqToBottomID == id {
			return i
		}
	}

	return -1
}

// duplicateReq mirrors the incoming request as a fresh request addressed to
// the bottom unit. The returned bool is true when the request is a read.
func (m *middleware) duplicateReq(
	req memprotocol.AccessReq,
) (memprotocol.AccessReq, bool) {
	src := m.bottomPort().AsRemote()
	dst := m.comp.Spec().BottomUnit

	switch req := req.(type) {
	case memprotocol.ReadReq:
		dup := memprotocol.ReadReq{
			Address:        req.Address,
			AccessByteSize: req.AccessByteSize,
			PID:            req.PID,
		}
		dup.ID = timing.GetIDGenerator().Generate()
		dup.Src = src
		dup.Dst = dst
		dup.TrafficClass = req.TrafficClass
		dup.TrafficBytes = req.TrafficBytes

		return dup, true
	case memprotocol.WriteReq:
		dup := memprotocol.WriteReq{
			Address:   req.Address,
			Data:      req.Data,
			DirtyMask: req.DirtyMask,
			PID:       req.PID,
		}
		dup.ID = timing.GetIDGenerator().Generate()
		dup.Src = src
		dup.Dst = dst
		dup.TrafficClass = req.TrafficClass
		dup.TrafficBytes = req.TrafficBytes

		return dup, false
	default:
		panic("rob: unsupported request type")
	}
}

// duplicateRsp builds the response that is sent to the original requester,
// answering the original top-side request.
func (m *middleware) duplicateRsp(trans transactionState) messaging.Msg {
	if trans.IsRead {
		rsp := memprotocol.DataReadyRsp{Data: trans.RspData}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = m.topPort().AsRemote()
		rsp.Dst = trans.ReqFromTopSrc
		rsp.RspTo = trans.ReqFromTopID
		rsp.TrafficClass = "memprotocol.DataReadyRsp"
		rsp.TrafficBytes = len(trans.RspData) + 4

		return rsp
	}

	rsp := memprotocol.WriteDoneRsp{}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.topPort().AsRemote()
	rsp.Dst = trans.ReqFromTopSrc
	rsp.RspTo = trans.ReqFromTopID
	rsp.TrafficClass = "memprotocol.WriteDoneRsp"
	rsp.TrafficBytes = 4

	return rsp
}

// bottomReqTraceMsg reconstructs a minimal request to use as the subject of a
// trace event for the duplicated request the reorder buffer issued.
func (m *middleware) bottomReqTraceMsg(trans transactionState) messaging.Msg {
	if trans.IsRead {
		req := memprotocol.ReadReq{}
		req.ID = trans.ReqToBottomID
		req.Src = m.bottomPort().AsRemote()
		req.Dst = m.comp.Spec().BottomUnit

		return req
	}

	req := memprotocol.WriteReq{}
	req.ID = trans.ReqToBottomID
	req.Src = m.bottomPort().AsRemote()
	req.Dst = m.comp.Spec().BottomUnit

	return req
}

// topReqTraceMsg reconstructs a minimal request to use as the subject of a
// trace event for the original top-side request.
func (m *middleware) topReqTraceMsg(trans transactionState) messaging.Msg {
	if trans.IsRead {
		req := memprotocol.ReadReq{}
		req.ID = trans.ReqFromTopID
		req.Src = trans.ReqFromTopSrc
		req.Dst = m.topPort().AsRemote()

		return req
	}

	req := memprotocol.WriteReq{}
	req.ID = trans.ReqFromTopID
	req.Src = trans.ReqFromTopSrc
	req.Dst = m.topPort().AsRemote()

	return req
}

var _ modeling.Middleware = (*middleware)(nil)
