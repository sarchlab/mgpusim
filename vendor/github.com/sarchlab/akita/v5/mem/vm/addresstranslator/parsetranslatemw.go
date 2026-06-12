package addresstranslator

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// parseTranslateMW handles incoming requests from topPort and initiates
// address translation. It also handles control messages (flush/restart).
type parseTranslateMW struct {
	comp *modeling.Component[Spec, State]
}

func (m *parseTranslateMW) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *parseTranslateMW) bottomPort() sim.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *parseTranslateMW) translationPort() sim.Port {
	return m.comp.GetPortByName("Translation")
}

func (m *parseTranslateMW) ctrlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

// Tick runs the parseTranslate stage: translate (if not flushing) + handleCtrlRequest.
func (m *parseTranslateMW) Tick() bool {
	madeProgress := false

	nextState := m.comp.GetNextState()
	if !nextState.IsFlushing {
		spec := m.comp.GetSpec()
		for i := 0; i < spec.NumReqPerCycle; i++ {
			madeProgress = m.translate() || madeProgress
		}
	}

	madeProgress = m.handleCtrlRequest() || madeProgress

	return madeProgress
}

func (m *parseTranslateMW) translate() bool {
	itemI := m.topPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	item := itemI.(mem.AccessReq)
	vAddr := item.GetAddress()
	spec := m.comp.GetSpec()
	vPageID := addrToPageID(vAddr, spec.Log2PageSize)

	transReq := &vm.TranslationReq{}
	transReq.ID = sim.GetIDGenerator().Generate()
	transReq.Src = m.translationPort().AsRemote()
	transReq.Dst = findTranslationPort(spec, vAddr)
	transReq.PID = item.GetPID()
	transReq.VAddr = vPageID
	transReq.DeviceID = spec.DeviceID
	transReq.TrafficClass = "vm.TranslationReq"

	err := m.translationPort().Send(transReq)
	if err != nil {
		return false
	}

	incoming := msgToIncomingReqState(itemI)

	nextState := m.comp.GetNextState()

	tracing.TraceReqReceive(itemI, m.comp)
	tracing.TraceReqInitiate(
		transReq,
		m.comp,
		tracing.MsgIDAtReceiver(itemI, m.comp),
	)

	// Update incoming state with recv task ID after tracing
	incoming.RecvTaskID = itemI.(sim.Msg).Meta().RecvTaskID

	trans := transactionState{
		IncomingReqs:              []incomingReqState{incoming},
		TranslationReqID:         transReq.ID,
		TranslationReqSendTaskID: transReq.SendTaskID,
		TranslationReqSrc:        transReq.Src,
		TranslationReqDst:        transReq.Dst,
	}
	nextState.Transactions = append(nextState.Transactions, trans)

	m.topPort().RetrieveIncoming()

	return true
}

func (m *parseTranslateMW) handleCtrlRequest() bool {
	msgI := m.ctrlPort().PeekIncoming()
	if msgI == nil {
		return false
	}

	msg := msgI.(*mem.ControlReq)

	switch msg.Command {
	case mem.CmdFlush:
		return m.handleFlushReq(msg)
	case mem.CmdReset:
		return m.handleRestartReq(msg)
	default:
		panic("unhandled control command")
	}
}

func (m *parseTranslateMW) handleFlushReq(msg *mem.ControlReq) bool {
	rsp := &mem.ControlRsp{Command: mem.CmdFlush, Success: true}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.TrafficBytes = 4
	rsp.TrafficClass = "mem.ControlRsp"

	err := m.ctrlPort().Send(rsp)
	if err != nil {
		return false
	}

	m.ctrlPort().RetrieveIncoming()

	nextState := m.comp.GetNextState()
	nextState.Transactions = nil
	nextState.InflightReqToBottom = nil
	nextState.IsFlushing = true

	return true
}

func (m *parseTranslateMW) handleRestartReq(msg *mem.ControlReq) bool {
	rsp := &mem.ControlRsp{Command: mem.CmdReset, Success: true}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.ctrlPort().AsRemote()
	rsp.Dst = msg.Src
	rsp.TrafficBytes = 4
	rsp.TrafficClass = "mem.ControlRsp"

	err := m.ctrlPort().Send(rsp)

	if err != nil {
		return false
	}

	for m.topPort().RetrieveIncoming() != nil {
	}

	for m.bottomPort().RetrieveIncoming() != nil {
	}

	for m.translationPort().RetrieveIncoming() != nil {
	}

	nextState := m.comp.GetNextState()
	nextState.IsFlushing = false

	m.ctrlPort().RetrieveIncoming()

	return true
}
