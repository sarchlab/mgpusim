package rob

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// Control protocol mapping (v4 -> v5).
//
// In Akita v4 the reorder buffer accepted mem.ControlMsg on its Control port
// with two compound operations:
//
//   - "flush" (DiscardTransactions): ack immediately, drop all in-flight
//     transactions, and freeze the pipeline until restarted.
//   - "restart": ack immediately, drop all in-flight transactions, discard
//     every stale message queued on the Top and Bottom ports, and resume.
//
// In v5 the reorder buffer speaks the uniform memcontrolprotocol on its
// Control port and implements the four universal verbs (see
// akita/mem/CONTROL_PROTOCOL.md). The command processor reproduces the v4
// flows by sequencing primitives:
//
//   - v4 "flush"   -> CmdPause (sync ack; the pipeline freezes). In-flight
//     transactions stay frozen in place instead of being dropped at flush
//     time; because nothing moves while paused, deferring the drop to the
//     CmdReset that precedes re-enabling is observationally equivalent.
//   - v4 "restart" -> CmdReset (sync ack; drops all in-flight transactions,
//     discards stale Top/Bottom traffic, and lands the component back in
//     the Enabled state).
//
// CmdDrain (quiesce: let in-flight finish, end paused) and CmdEnable (resume
// from paused without discarding anything) are also supported, so the ROB
// satisfies the universal verb matrix. CmdInvalidate and CmdFlush respond
// with Success: false, Error: "unsupported" — the ROB holds no private
// cache-of-memory state.

// processControlMsg handles the universal control verbs and finalizes a
// pending Drain when in-flight transactions have settled. Control commands
// are processed serially: while an async verb (Drain) is in progress, the
// next command stays queued on the Control port until the component settles.
func (m *middleware) processControlMsg() bool {
	if m.completePendingDrain() {
		return true
	}

	if m.comp.State.ControlState == memcontrolprotocol.StateDraining {
		return false
	}

	item := m.ctrlPort().PeekIncoming()
	if item == nil {
		return false
	}

	req, ok := item.(memcontrolprotocol.Req)
	if !ok {
		panic("rob: unsupported control-port message type")
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

// completePendingDrain notices that a Drain has reached quiescence (no
// in-flight transactions) and acks it. The component lands in Paused.
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

	m.ctrlPort().Send(m.makeCtrlRsp(memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.ControlState = memcontrolprotocol.StatePaused
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""

	return true
}

// handlePause freezes the pipeline in place. This is the first half of the
// v4 "flush" flow.
func (m *middleware) handlePause(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.comp.State.ControlState = memcontrolprotocol.StatePaused
	m.ctrlPort().Send(m.makeCtrlRsp(memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()

	return true
}

// handleDrain stops accepting new traffic and lets in-flight transactions
// retire; the ack is sent asynchronously by completePendingDrain.
func (m *middleware) handleDrain(req memcontrolprotocol.Req) bool {
	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src
	m.ctrlPort().RetrieveIncoming()

	return true
}

// handleEnable resumes processing from paused. Traffic queued while paused is
// kept and processed once the pipeline runs again.
func (m *middleware) handleEnable(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(m.makeCtrlRsp(memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))
	m.comp.State.ControlState = memcontrolprotocol.StateEnabled
	m.ctrlPort().RetrieveIncoming()

	return true
}

// handleReset drops every in-flight transaction, discards stale Top/Bottom
// traffic, and lands the reorder buffer back in Enabled. This is the v4
// "restart" flow.
func (m *middleware) handleReset(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(m.makeCtrlRsp(memcontrolprotocol.CmdReset,
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

	m.ctrlPort().Send(m.makeCtrlRsp(req.Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()

	return true
}

func (m *middleware) makeCtrlRsp(
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
	rsp.Src = m.ctrlPort().AsRemote()
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
// entries that topDown allocated for each in-flight top-side request, so the
// entries do not outlive the transactions they describe.
func (m *middleware) forgetInflightReceiverIDs() {
	for _, trans := range m.comp.State.Transactions {
		tracing.ForgetMsgIDAtReceiver(trans.ReqFromTopID, m.comp)
	}
}
