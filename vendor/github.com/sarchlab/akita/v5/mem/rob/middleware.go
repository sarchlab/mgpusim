package rob

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type middleware struct {
	comp *Comp
}

func (m *middleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *middleware) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *middleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

// Tick advances the reorder buffer by one cycle. The control port is
// serviced first so Reset or Pause can quiesce the pipeline before any
// new traffic moves. While paused the pipeline is frozen entirely.
// Drain completion is handled inside processControlMsg.
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
	width := m.comp.Spec().NumReqPerCycle

	for i := 0; i < width; i++ {
		if !m.bottomUp() {
			break
		}
		madeProgress = true
	}

	for i := 0; i < width; i++ {
		if !m.parseBottom() {
			break
		}
		madeProgress = true
	}

	for i := 0; i < width; i++ {
		if !m.topDown() {
			break
		}
		madeProgress = true
	}

	return madeProgress
}

// topDown pulls a request from the top, forwards a shadow request to the
// bottom, and records the transaction in FIFO order. Skipped while not
// Enabled so Drain and Pause stop accepting new traffic.
func (m *middleware) topDown() bool {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateEnabled {
		return false
	}
	if len(state.Transactions) >= m.comp.Spec().BufferSize {
		return false
	}

	msg := m.topPort().PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(memprotocol.AccessReq)
	if !ok {
		panic("rob: unsupported top-port message type")
	}

	shadow, isRead := m.buildShadowReq(
		req, m.bottomPort().AsRemote(), m.comp.Spec().BottomUnit)

	if !m.bottomPort().CanSend() {
		return false
	}

	m.bottomPort().Send(shadow)

	state.Transactions = append(state.Transactions, transactionState{
		ReqFromTopID:  req.Meta().ID,
		ReqFromTopSrc: req.Meta().Src,
		ReqToBottomID: shadow.Meta().ID,
		IsRead:        isRead,
	})
	m.topPort().RetrieveIncoming()

	tracing.TraceReqReceive(m.comp, req)
	tracing.TraceReqInitiate(m.comp, shadow,
		tracing.MsgIDAtReceiver(req, m.comp))

	return true
}

// parseBottom records the response that came back from the bottom unit on its
// matching transaction. Unmatched responses (e.g. left over after a flush) are
// dropped.
func (m *middleware) parseBottom() bool {
	msg := m.bottomPort().PeekIncoming()
	if msg == nil {
		return false
	}

	switch dataRsp := msg.(type) {
	case memprotocol.DataReadyRsp:
		idx := m.findTransactionByBottomID(dataRsp.RspTo)
		m.bottomPort().RetrieveIncoming()

		if idx < 0 {
			return true
		}

		trans := &m.comp.State.Transactions[idx]
		trans.HasRsp = true
		trans.RspData = dataRsp.Data
		tracing.TraceReqFinalize(m.comp, m.shadowReqTraceMsg(*trans))
		return true
	case memprotocol.WriteDoneRsp:
		idx := m.findTransactionByBottomID(dataRsp.RspTo)
		m.bottomPort().RetrieveIncoming()

		if idx < 0 {
			return true
		}

		trans := &m.comp.State.Transactions[idx]
		trans.HasRsp = true
		tracing.TraceReqFinalize(m.comp, m.shadowReqTraceMsg(*trans))
		return true
	default:
		m.bottomPort().RetrieveIncoming()
		return true
	}
}

// bottomUp releases the head-of-line transaction once its bottom-unit response
// has arrived, forwarding a response to the original requester.
func (m *middleware) bottomUp() bool {
	state := &m.comp.State
	if len(state.Transactions) == 0 {
		return false
	}

	head := state.Transactions[0]
	if !head.HasRsp {
		return false
	}

	rsp := m.buildTopRsp(head, m.topPort().AsRemote())

	if !m.topPort().CanSend() {
		return false
	}

	m.topPort().Send(rsp)

	state.Transactions = state.Transactions[1:]

	tracing.TraceReqComplete(m.comp, m.topReqTraceMsg(head))

	return true
}

func (m *middleware) findTransactionByBottomID(id uint64) int {
	for i := range m.comp.State.Transactions {
		if m.comp.State.Transactions[i].ReqToBottomID == id {
			return i
		}
	}
	return -1
}

// buildShadowReq mirrors the incoming request as a fresh request the bottom
// unit will see. The returned bool is true when the source request is a read.
func (m *middleware) buildShadowReq(
	req memprotocol.AccessReq, src, dst messaging.RemotePort,
) (memprotocol.AccessReq, bool) {
	switch r := req.(type) {
	case memprotocol.ReadReq:
		shadow := memprotocol.ReadReq{
			Address:        r.Address,
			AccessByteSize: r.AccessByteSize,
			PID:            r.PID,
		}
		shadow.ID = timing.GetIDGenerator().Generate()
		shadow.Src = src
		shadow.Dst = dst
		shadow.TrafficBytes = r.TrafficBytes
		shadow.TrafficClass = r.TrafficClass
		return shadow, true
	case memprotocol.WriteReq:
		shadow := memprotocol.WriteReq{
			Address:   r.Address,
			Data:      r.Data,
			DirtyMask: r.DirtyMask,
			PID:       r.PID,
		}
		shadow.ID = timing.GetIDGenerator().Generate()
		shadow.Src = src
		shadow.Dst = dst
		shadow.TrafficBytes = r.TrafficBytes
		shadow.TrafficClass = r.TrafficClass
		return shadow, false
	default:
		panic("rob: unsupported request type")
	}
}

func (m *middleware) buildTopRsp(
	trans transactionState, src messaging.RemotePort,
) messaging.Msg {
	if trans.IsRead {
		rsp := memprotocol.DataReadyRsp{Data: trans.RspData}
		rsp.ID = timing.GetIDGenerator().Generate()
		rsp.Src = src
		rsp.Dst = trans.ReqFromTopSrc
		rsp.RspTo = trans.ReqFromTopID
		rsp.TrafficBytes = len(trans.RspData) + 4
		rsp.TrafficClass = "memprotocol.DataReadyRsp"
		return rsp
	}

	rsp := memprotocol.WriteDoneRsp{}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = src
	rsp.Dst = trans.ReqFromTopSrc
	rsp.RspTo = trans.ReqFromTopID
	rsp.TrafficBytes = 4
	rsp.TrafficClass = "memprotocol.WriteDoneRsp"
	return rsp
}

// shadowReqTraceMsg reconstructs a minimal request to use as the subject of a
// trace event for the shadow request the reorder buffer issued.
func (m *middleware) shadowReqTraceMsg(trans transactionState) messaging.Msg {
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

// processControlMsg handles the universal control verbs and finalizes
// a pending Drain when in-flight transactions have settled.
func (m *middleware) processControlMsg() bool {
	if m.completePendingDrain() {
		return true
	}

	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not dequeued — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.ControlState == memcontrolprotocol.StateDraining {
		return false
	}

	msg := m.ctrlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(memcontrolprotocol.Req)
	if !ok {
		m.ctrlPort().RetrieveIncoming()
		return true
	}

	switch req.Command {
	case memcontrolprotocol.CmdPause:
		return m.handlePause(req)
	case memcontrolprotocol.CmdDrain:
		return m.handleDrain(req)
	case memcontrolprotocol.CmdEnable:
		return m.handleEnable(req)
	case memcontrolprotocol.CmdReset:
		return m.handleReset(req)
	default:
		return m.handleUnsupported(req)
	}
}

// completePendingDrain notices Drain has reached quiescence (no
// in-flight transactions) and acks. The component transitions to
// Paused.
func (m *middleware) completePendingDrain() bool {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateDraining {
		return false
	}
	if len(state.Transactions) != 0 {
		return false
	}
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.ControlState = memcontrolprotocol.StatePaused
	return true
}

func (m *middleware) handlePause(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = memcontrolprotocol.StatePaused
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *middleware) handleDrain(req memcontrolprotocol.Req) bool {
	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *middleware) handleEnable(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))

	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateEnabled

	// Enable resumes from Paused; it must not discard traffic queued while
	// paused (e.g. bottom responses that retire frozen in-flight
	// transactions). They are processed once the pipeline runs again.
	m.ctrlPort().RetrieveIncoming()
	return true
}

// handleReset drops every in-flight transaction, drains stale port
// traffic, and lands the ROB back in Enabled. The tracing receiver-task
// registry entries that topDown allocated for each in-flight top-side
// request are released so they do not outlive the transactions.
func (m *middleware) handleReset(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))

	state := &m.comp.State
	m.forgetInflightReceiverIDs()
	state.Transactions = state.Transactions[:0]
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = memcontrolprotocol.StateEnabled

	drainIncoming(m.topPort())
	drainIncoming(m.bottomPort())

	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *middleware) handleUnsupported(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), req.Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func makeCtrlRsp(
	port messaging.Port,
	cmd memcontrolprotocol.Command,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) memcontrolprotocol.Rsp {
	rsp := memcontrolprotocol.Rsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = port.AsRemote()
	rsp.Dst = dst
	rsp.RspTo = rspTo
	rsp.TrafficClass = "memcontrolprotocol.Rsp"
	return rsp
}

func drainIncoming(p messaging.Port) {
	for p.RetrieveIncoming() != nil {
	}
}

// forgetInflightReceiverIDs releases the tracing receiver-task registry
// entries that topDown allocated for each in-flight top-side request. Control
// paths that drop the transaction table call this so the entries do not
// outlive the transactions they describe.
func (m *middleware) forgetInflightReceiverIDs() {
	for _, trans := range m.comp.State.Transactions {
		tracing.ForgetMsgIDAtReceiver(trans.ReqFromTopID, m.comp)
	}
}

var _ modeling.Middleware = (*middleware)(nil)
