package dram

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

type ctrlMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain) is
	// in progress, the next command is not accepted — it stays queued on the
	// Control port and is handled once the component settles.
	if m.comp.State.ControlState != memcontrolprotocol.StateDraining {
		madeProgress = m.handleIncoming() || madeProgress
	}
	return madeProgress
}

// completePendingDrain finalizes a Drain when in-flight transactions
// have settled. "Settled" here means no transactions in the controller
// queue.
func (m *ctrlMiddleware) completePendingDrain() bool {
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

func (m *ctrlMiddleware) handleIncoming() bool {
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
	m.comp.State.ControlState = memcontrolprotocol.StatePaused
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.comp.State.ControlState = memcontrolprotocol.StateEnabled
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdEnable,
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

// handleReset clears runtime state back to a freshly-built controller
// (empty queues, closed bank state). Persistent storage stays
// untouched — it lives in shared resources.
func (m *ctrlMiddleware) handleReset(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	state := &m.comp.State
	spec := m.comp.Spec()

	state.Transactions = nil
	state.SubTransQueue = subTransQueueState{Entries: []subTransRef{}}
	state.CommandQueues = commandQueueState{
		NumQueues: spec.NumChannel * spec.NumRank,
		Entries:   []queueEntry{},
	}
	state.BankStates = initBankStatesFlat(
		spec.NumRank, spec.NumBankGroup, spec.NumBank)
	state.TickCount = 0
	state.RefreshCycleCounter = 0
	state.RefreshInProgress = false
	state.RefreshCyclesRemaining = 0
	state.CurrentCmdID = 0
	state.CurrentCmdSrc = ""
	state.ControlState = memcontrolprotocol.StateEnabled

	resetStatistics(state)

	for m.topPort().RetrieveIncoming() != nil {
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// resetStatistics zeroes the accumulated DRAM statistics so a Reset returns
// the controller to its freshly-built shape. These counters are reported for
// experiment results, so a reset before a new measurement phase must not carry
// pre-reset traffic into the post-reset run.
func resetStatistics(state *State) {
	state.TotalReadCommands = 0
	state.TotalWriteCommands = 0
	state.TotalActivates = 0
	state.TotalPrecharges = 0
	state.RowBufferHits = 0
	state.RowBufferMisses = 0
	state.TotalCycles = 0
	state.TotalReadLatencyCycles = 0
	state.TotalWriteLatencyCycles = 0
	state.CompletedReads = 0
	state.CompletedWrites = 0
	state.BytesRead = 0
	state.BytesWritten = 0
}

func (m *ctrlMiddleware) handleUnsupported(req memcontrolprotocol.Req) bool {
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
