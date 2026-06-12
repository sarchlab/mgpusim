package tlb

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State]
}

func (m *ctrlMiddleware) controlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) bottomPort() sim.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.handleIncomingCommands() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) handleIncomingCommands() bool {
	msg := m.controlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	ctrlReq, ok := msg.(*mem.ControlReq)
	if !ok {
		panic("Unhandled message")
	}

	switch ctrlReq.Command {
	case mem.CmdEnable:
		return m.performCtrlEnable(ctrlReq)
	case mem.CmdDrain:
		return m.performCtrlDrain(ctrlReq)
	case mem.CmdPause:
		return m.performCtrlPause(ctrlReq)
	case mem.CmdFlush:
		return m.handleTLBFlush(ctrlReq)
	case mem.CmdReset:
		return m.handleTLBRestart(ctrlReq)
	default:
		panic("Unhandled control command")
	}
}

func (m *ctrlMiddleware) performCtrlEnable(msg *mem.ControlReq) bool {
	state := m.comp.GetNextState()
	state.TLBState = tlbStateEnable

	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	return true
}

func (m *ctrlMiddleware) performCtrlDrain(msg *mem.ControlReq) bool {
	state := m.comp.GetNextState()
	state.TLBState = tlbStateDrain

	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	return true
}

func (m *ctrlMiddleware) performCtrlPause(msg *mem.ControlReq) bool {
	state := m.comp.GetNextState()
	state.TLBState = tlbStatePause

	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	return true
}

func (m *ctrlMiddleware) handleTLBFlush(msg *mem.ControlReq) bool {
	state := m.comp.GetNextState()
	state.HasInflightFlushReq = true
	state.InflightFlush = inflightFlushState{
		VAddr: msg.Addresses,
		PID:   msg.PID,
		Meta:  msg.MsgMeta,
	}
	m.controlPort().RetrieveIncoming()
	state.TLBState = tlbStateFlush

	return true
}

func (m *ctrlMiddleware) handleTLBRestart(msg *mem.ControlReq) bool {
	rsp := &mem.ControlRsp{Command: mem.CmdReset, Success: true}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.controlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.TrafficClass = "mem.ControlRsp"

	err := m.controlPort().Send(rsp)
	if err != nil {
		return false
	}
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(msg, m.comp),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	state := m.comp.GetNextState()
	state.TLBState = tlbStateEnable

	for m.topPort().PeekIncoming() != nil {
		m.topPort().RetrieveIncoming()
	}

	for m.bottomPort().PeekIncoming() != nil {
		m.bottomPort().RetrieveIncoming()
	}

	m.controlPort().RetrieveIncoming()

	return true
}
