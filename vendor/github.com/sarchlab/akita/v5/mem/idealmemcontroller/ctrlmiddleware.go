package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *ctrlMiddleware) Tick() (madeProgress bool) {
	madeProgress = m.handleStateUpdate() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.ControlState != memcontrolprotocol.StateDraining {
		madeProgress = m.handleIncomingCommands() || madeProgress
	}
	return madeProgress
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

// handleStateUpdate notices Drain completion and acks the pending
// Drain request, transitioning the component to Paused.
func (m *ctrlMiddleware) handleStateUpdate() (madeProgress bool) {
	state := &m.comp.State
	if state.ControlState != memcontrolprotocol.StateDraining {
		return false
	}

	if len(state.InflightTransactions) != 0 {
		return false
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	rsp := makeRsp(m.ctrlPort(), memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, "")
	m.ctrlPort().Send(rsp)
	state.ControlState = memcontrolprotocol.StatePaused

	return true
}

func (m *ctrlMiddleware) handleIncomingCommands() (madeProgress bool) {
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

func (m *ctrlMiddleware) handlePause(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StatePaused

	m.ctrlPort().Send(makeRsp(m.ctrlPort(), memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateEnabled

	m.ctrlPort().Send(makeRsp(m.ctrlPort(), memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req memcontrolprotocol.Req) bool {
	state := &m.comp.State
	state.ControlState = memcontrolprotocol.StateDraining
	state.CurrentCmdID = req.ID
	state.CurrentCmdSrc = req.Src

	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleReset(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	state.InflightTransactions = nil
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = memcontrolprotocol.StateEnabled

	// Reset is a hard reset to a freshly-built, no-work state. Drop any
	// requests still queued on the Top port; otherwise (the control
	// middleware runs before the memory middleware) takeNewReqs would consume
	// a stale request in the very same tick, right after the reset ack.
	top := m.comp.GetPortByName("Top")
	for top.PeekIncoming() != nil {
		top.RetrieveIncoming()
	}

	m.ctrlPort().Send(makeRsp(m.ctrlPort(), memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleUnsupported(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeRsp(m.ctrlPort(), req.Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func makeRsp(
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
