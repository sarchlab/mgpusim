package rdma

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// ctrlMiddleware handles the drain/restart control flow on the "Ctrl" port.
// The drain/restart messages are mgpusim-internal (see rdmaprotocol.go); they
// are sent by the Command Processor.
type ctrlMiddleware struct {
	comp *Comp
}

func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.comp.GetPortByName("Ctrl")
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
	msg := m.ctrlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case DrainReq:
		return m.processDrainReq(msg)
	case RestartReq:
		return m.processRestartReq(msg)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(msg))
		return false
	}
}

func (m *ctrlMiddleware) processDrainReq(req DrainReq) bool {
	state := &m.comp.State
	state.CurrentDrainReqID = req.ID
	state.CurrentDrainReqSrc = req.Src
	state.IsDraining = true
	state.PauseIncomingReqsFromL1 = true

	m.ctrlPort().RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRestartReq(req RestartReq) bool {
	port := m.ctrlPort()
	if !port.CanSend() {
		return false
	}

	rsp := RestartRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   port.AsRemote(),
			Dst:   req.Src,
			RspTo: req.ID,
		},
	}
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
	rsp := DrainRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   port.AsRemote(),
			Dst:   state.CurrentDrainReqSrc,
			RspTo: state.CurrentDrainReqID,
		},
	}
	port.Send(rsp)

	state.IsDraining = false

	return true
}

func (m *ctrlMiddleware) fullyDrained() bool {
	return len(m.comp.State.TransactionsFromOutside) == 0 &&
		len(m.comp.State.TransactionsFromInside) == 0
}
