package tlb

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *ctrlMiddleware) controlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.TLBState != tlbStateDrain {
		madeProgress = m.handleIncomingCommands() || madeProgress
	}
	return madeProgress
}

// completePendingDrain detects that the data-path middleware has
// finished draining (TLBState transitioned to pause) and sends the
// async Drain Rsp.
func (m *ctrlMiddleware) completePendingDrain() bool {
	state := &m.comp.State
	if !state.PendingDrainRsp {
		return false
	}
	if state.TLBState != tlbStatePause {
		return false
	}
	if !m.controlPort().CanSend() {
		return false
	}

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdDrain,
		state.CurrentCmdSrc, state.CurrentCmdID, true, ""))
	state.PendingDrainRsp = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	return true
}

func (m *ctrlMiddleware) handleIncomingCommands() bool {
	msg := m.controlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	ctrlReq, ok := msg.(memcontrolprotocol.Req)
	if !ok {
		// Drop unexpected message types so the Control port does not stall.
		m.controlPort().RetrieveIncoming()
		return true
	}

	switch ctrlReq.Command {
	case memcontrolprotocol.CmdEnable:
		return m.performCtrlEnable(ctrlReq)
	case memcontrolprotocol.CmdDrain:
		return m.performCtrlDrain(ctrlReq)
	case memcontrolprotocol.CmdPause:
		return m.performCtrlPause(ctrlReq)
	case memcontrolprotocol.CmdReset:
		return m.handleReset(ctrlReq)
	case memcontrolprotocol.CmdInvalidate:
		return m.handleInvalidate(ctrlReq)
	case memcontrolprotocol.CmdFlush:
		// A TLB holds no dirty data, so Flush is not meaningful; callers
		// drop entries with Invalidate instead.
		return m.handleUnsupported(ctrlReq)
	default:
		return m.handleUnsupported(ctrlReq)
	}
}

func (m *ctrlMiddleware) performCtrlEnable(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	state := &m.comp.State
	state.TLBState = tlbStateEnable

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdEnable,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.controlPort().Name(),
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

func (m *ctrlMiddleware) performCtrlDrain(msg memcontrolprotocol.Req) bool {
	state := &m.comp.State
	state.TLBState = tlbStateDrain
	state.PendingDrainRsp = true
	state.CurrentCmdID = msg.ID
	state.CurrentCmdSrc = msg.Src

	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.controlPort().Name(),
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

func (m *ctrlMiddleware) performCtrlPause(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	state := &m.comp.State
	state.TLBState = tlbStatePause

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdPause,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.controlPort().Name(),
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

// handleInvalidate drops cached translations matching the request's
// address/PID filter (empty address list = all addresses, zero PID = all
// PIDs). Invalidate is a synchronous verb but is only legal once the TLB
// is paused or drained; issued while Enabled it is rejected.
func (m *ctrlMiddleware) handleInvalidate(msg memcontrolprotocol.Req) bool {
	state := &m.comp.State
	// Invalidate is only legal once the TLB is fully paused. While it is
	// still draining, in-flight bottom responses can still be parsed into
	// the sets after the invalidate, so accept only the paused state.
	if state.TLBState != tlbStatePause {
		return m.rejectMustBePaused(msg)
	}
	if !m.controlPort().CanSend() {
		return false
	}

	invalidateEntries(state, m.comp.Spec(), msg.Addresses, msg.PID)

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdInvalidate,
		msg.Src, msg.ID, true, ""))
	m.controlPort().RetrieveIncoming()
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindDependency,
		What:   m.comp.Name() + ".Sets",
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	return true
}

// rejectMustBePaused responds that a conditional verb is illegal while the
// component is Enabled.
func (m *ctrlMiddleware) rejectMustBePaused(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	m.controlPort().Send(makeCtrlRsp(m.controlPort(), msg.Command,
		msg.Src, msg.ID, false, memcontrolprotocol.ErrMustBePausedOrDrained))
	m.controlPort().RetrieveIncoming()
	return true
}

// invalidateEntries marks every cached page matching the filter invalid.
func invalidateEntries(
	state *State,
	spec Spec,
	addresses []uint64,
	pid vm.PID,
) {
	matchAddr := make(map[uint64]bool, len(addresses))
	for _, a := range addresses {
		matchAddr[a/spec.PageSize*spec.PageSize] = true
	}

	for si := range state.Sets {
		set := &state.Sets[si]
		for wi := range set.Blocks {
			page := set.Blocks[wi].Page
			if !page.Valid {
				continue
			}
			if pid != 0 && page.PID != pid {
				continue
			}
			if len(addresses) > 0 && !matchAddr[page.VAddr] {
				continue
			}
			page.Valid = false
			setUpdate(set, wi, page)
		}
	}
}

func (m *ctrlMiddleware) handleReset(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}

	m.controlPort().Send(makeCtrlRsp(m.controlPort(), memcontrolprotocol.CmdReset,
		msg.Src, msg.ID, true, ""))
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtReceiver(msg, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.controlPort().Name(),
	})
	tracing.ForgetMsgIDAtReceiver(msg.ID, m.comp)

	state := &m.comp.State
	state.TLBState = tlbStateEnable
	state.PendingDrainRsp = false
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""

	// Reset is a hard reset to the freshly-built shape: discard in-flight
	// misses, the cached translations, and any staged pipeline work.
	state.MSHREntries = nil
	state.HasRespondingMSHR = false
	state.RespondingMSHRData = mshrEntryState{}

	spec := m.comp.Spec()
	state.Sets = initSets(spec.NumSets, spec.NumWays)
	state.Pipeline.Clear()
	state.BufferItems.Clear()

	for m.topPort().PeekIncoming() != nil {
		m.topPort().RetrieveIncoming()
	}
	for m.bottomPort().PeekIncoming() != nil {
		m.bottomPort().RetrieveIncoming()
	}

	m.controlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleUnsupported(msg memcontrolprotocol.Req) bool {
	if !m.controlPort().CanSend() {
		return false
	}
	m.controlPort().Send(makeCtrlRsp(m.controlPort(), msg.Command,
		msg.Src, msg.ID, false, memcontrolprotocol.ErrUnsupported))
	m.controlPort().RetrieveIncoming()
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
