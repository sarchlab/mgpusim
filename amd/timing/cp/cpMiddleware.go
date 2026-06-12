package cp

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/dispatching"
)

// cpMiddleware handles the data-path duties of the Command Processor: kernel
// launching (through the dispatchers), memory copies (through the DMA
// engine), and cache flush requests from the driver.
type cpMiddleware struct {
	comp *Comp

	// TODO(akita5): state purity — the dispatchers hold rich runtime object
	// graphs (grid builders, in-flight work-group maps), so they live here
	// rather than in the component State.
	dispatchers []dispatching.Dispatcher
}

func (m *cpMiddleware) toDriver() messaging.Port {
	return m.comp.GetPortByName("ToDriver")
}

func (m *cpMiddleware) toDMA() messaging.Port {
	return m.comp.GetPortByName("ToDMA")
}

func (m *cpMiddleware) Tick() bool {
	madeProgress := false

	madeProgress = m.tickDispatchers() || madeProgress
	madeProgress = m.processReqFromDriver() || madeProgress
	madeProgress = m.processRspFromDMA() || madeProgress

	return madeProgress
}

func (m *cpMiddleware) tickDispatchers() (madeProgress bool) {
	for _, d := range m.dispatchers {
		madeProgress = d.Tick() || madeProgress
	}

	return madeProgress
}

func (m *cpMiddleware) processReqFromDriver() bool {
	msg := m.toDriver().PeekIncoming()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case protocol.LaunchKernelReq:
		return m.processLaunchKernelReq(req)
	case protocol.FlushReq:
		return m.processFlushReq(req)
	case protocol.MemCopyH2DReq, protocol.MemCopyD2HReq:
		return m.processMemCopyReq(req)
	}

	// Control requests are left in the queue for the control middleware.
	return false
}

func (m *cpMiddleware) processRspFromDMA() bool {
	msg := m.toDMA().PeekIncoming()
	if msg == nil {
		return false
	}

	switch rsp := msg.(type) {
	case protocol.GeneralRsp:
		return m.processMemCopyRsp(rsp)
	}

	panic("never")
}

func (m *cpMiddleware) processMemCopyRsp(rsp protocol.GeneralRsp) bool {
	if !m.toDriver().CanSend() {
		return false
	}

	originalReq := m.findAndRemoveOriginalMemCopyRequest(rsp)
	originalMeta := originalReq.Meta()

	rspToDriver := protocol.GeneralRsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   m.toDriver().AsRemote(),
			Dst:   originalMeta.Src,
			RspTo: originalMeta.ID,
		},
	}

	m.toDriver().Send(rspToDriver)
	m.toDMA().RetrieveIncoming()

	tracing.TraceReqComplete(m.comp, originalReq)
	// End the sender-side task of the cloned request that was sent to the
	// DMA engine.
	tracing.EndTask(m.comp, tracing.TaskEnd{ID: rsp.Meta().RspTo})

	return true
}

func (m *cpMiddleware) findAndRemoveOriginalMemCopyRequest(
	rsp protocol.GeneralRsp,
) messaging.Msg {
	rspTo := rsp.Meta().RspTo
	state := &m.comp.State

	originalH2DReq, ok := state.BottomMemCopyH2DToTop[rspTo]
	if ok {
		delete(state.BottomMemCopyH2DToTop, rspTo)
		return originalH2DReq
	}

	originalD2HReq, ok := state.BottomMemCopyD2HToTop[rspTo]
	if ok {
		delete(state.BottomMemCopyD2HToTop, rspTo)
		return originalD2HReq
	}

	panic("never")
}

func (m *cpMiddleware) processLaunchKernelReq(
	req protocol.LaunchKernelReq,
) bool {
	d := m.findAvailableDispatcher()
	if d == nil {
		return false
	}

	if *sampling.SampledRunnerFlag {
		sampling.SampledEngineInstance.Reset()
	}
	d.StartDispatching(req)
	m.toDriver().RetrieveIncoming()

	tracing.TraceReqReceive(m.comp, req)

	return true
}

func (m *cpMiddleware) findAvailableDispatcher() dispatching.Dispatcher {
	for _, d := range m.dispatchers {
		if !d.IsDispatching() {
			return d
		}
	}

	return nil
}

// processFlushReq starts the cache-flush control sequence. See
// ctrlMiddleware.go for the control-verb mapping.
func (m *cpMiddleware) processFlushReq(req protocol.FlushReq) bool {
	state := &m.comp.State
	if state.CtrlSeq != ctrlSeqNone {
		return false
	}

	state.CurrFlushReq = req
	m.ctrlMW().startSeq(ctrlSeqFlush)

	m.toDriver().RetrieveIncoming()

	tracing.TraceReqReceive(m.comp, req)

	return true
}

func (m *cpMiddleware) ctrlMW() *ctrlMiddleware {
	for _, mw := range m.comp.Middlewares() {
		if ctrlMW, ok := mw.(*ctrlMiddleware); ok {
			return ctrlMW
		}
	}

	panic("ctrl middleware not found")
}

func (m *cpMiddleware) processMemCopyReq(req messaging.Msg) bool {
	state := &m.comp.State
	if state.CtrlSeq != ctrlSeqNone {
		return false
	}

	if !m.toDMA().CanSend() {
		return false
	}

	var cloned messaging.Msg
	switch req := req.(type) {
	case protocol.MemCopyH2DReq:
		c := req
		c.ID = timing.GetIDGenerator().Generate()
		c.Src = m.toDMA().AsRemote()
		c.Dst = state.DMAEngine
		state.BottomMemCopyH2DToTop[c.ID] = req
		cloned = c
	case protocol.MemCopyD2HReq:
		c := req
		c.ID = timing.GetIDGenerator().Generate()
		c.Src = m.toDMA().AsRemote()
		c.Dst = state.DMAEngine
		state.BottomMemCopyD2HToTop[c.ID] = req
		cloned = c
	default:
		panic("unknown type")
	}

	m.toDMA().Send(cloned)
	m.toDriver().RetrieveIncoming()

	tracing.TraceReqReceive(m.comp, req)
	tracing.TraceReqInitiate(m.comp, cloned,
		tracing.MsgIDAtReceiver(req, m.comp))

	return true
}
