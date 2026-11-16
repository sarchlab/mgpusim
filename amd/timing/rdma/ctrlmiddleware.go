package rdma

import (
	"log"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// ctrlMiddleware handles the drain/restart control flow on the "Ctrl" port.
// The drain/restart messages are mgpusim-internal (see rdmaprotocol.go); they
// are sent by the Command Processor.
type ctrlMiddleware struct {
	comp *Comp
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.Ports.Ctrl
}

// Tick processes control messages and, while draining, checks whether the
// drain has completed.
func (m *ctrlMiddleware) Tick() bool {
	madeProgress := m.processFromCtrlPort()

	if m.comp.State.IsDraining {
		madeProgress = m.drainRDMA() || madeProgress
	}

	return madeProgress
}

func (m *ctrlMiddleware) processFromCtrlPort() bool {
	msg, ok := m.ctrlPort().PeekIncoming()
	if !ok {
		return false
	}

	switch msg.Payload.(type) {
	case DrainReq:

		return m.processDrainReq(msg)
	case RestartReq:

		return m.processRestartReq(msg)
	default:

		log.Panicf("cannot process request of type %T", msg.Payload)
		return false
	}
}

func (m *ctrlMiddleware) processDrainReq(req messaging.Msg) bool {
	state := &m.comp.State
	state.CurrentDrainReqID = req.ID
	state.CurrentDrainReqSrc = req.Src
	state.IsDraining = true
	state.PauseIncomingReqsFromL1 = true

	m.ctrlPort().RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRestartReq(req messaging.Msg) bool {
	port := m.ctrlPort()
	if !port.CanSend() {
		return false
	}

	rsp := messaging.Msg{ID: m.comp.NewID(),
		Src:   port.AsRemote(),
		Dst:   req.Src,
		RspTo: req.ID, Payload: RestartRsp{}}

	port.Send(rsp)
	port.RetrieveIncoming()

	state := &m.comp.State
	state.CurrentDrainReqID = 0
	state.CurrentDrainReqSrc = ""
	state.PauseIncomingReqsFromL1 = false

	return true
}

func (m *ctrlMiddleware) drainRDMA() bool {
	if !m.fullyDrained() {
		return false
	}

	port := m.ctrlPort()
	if !port.CanSend() {
		return false
	}

	state := &m.comp.State
	rsp := messaging.Msg{ID: m.comp.NewID(),
		Src:   port.AsRemote(),
		Dst:   state.CurrentDrainReqSrc,
		RspTo: state.CurrentDrainReqID, Payload: DrainRsp{}}

	port.Send(rsp)

	state.IsDraining = false

	return true
}

func (m *ctrlMiddleware) fullyDrained() bool {
	return len(m.comp.State.TransactionsFromOutside) == 0 &&
		len(m.comp.State.TransactionsFromInside) == 0
}

func (m *ctrlMiddleware) Handle(timing.Event) bool { return m.Tick() }
