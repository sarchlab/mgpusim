package cp

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/dispatching"
)

// cpMiddleware handles the data-path duties of the Command Processor: kernel
// launching (through the dispatchers), memory copies (through the DMA
// engine), and cache flush requests from the driver.
type cpMiddleware struct {
	comp *Comp

	dispatchers []dispatching.Dispatcher
}

func (m *cpMiddleware) toDriver() messaging.Port {
	return m.comp.Ports.ToDriver
}

func (m *cpMiddleware) toDMA() messaging.Port {
	return m.comp.Ports.ToDMA
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
	msg, ok := m.toDriver().PeekIncoming()
	if !ok {
		return false
	}

	switch msg.Payload.(type) {
	case protocol.LaunchKernelReq:
		req := msg

		return m.processLaunchKernelReq(req)
	case protocol.FlushReq:
		req := msg

		return m.processFlushReq(req)
	case protocol.MemCopyH2DReq, protocol.MemCopyD2HReq:
		req := msg

		return m.processMemCopyReq(req)
	}

	// Control requests are left in the queue for the control middleware.
	return false
}

func (m *cpMiddleware) processRspFromDMA() bool {
	msg, ok := m.toDMA().PeekIncoming()
	if !ok {
		return false
	}

	switch msg.Payload.(type) {
	case protocol.GeneralRsp, protocol.MemCopyD2HRsp:
		rsp := msg

		return m.processMemCopyRsp(rsp)
	}

	panic("never")
}

func (m *cpMiddleware) processMemCopyRsp(rsp messaging.Msg) bool {
	if !m.toDriver().CanSend() {
		return false
	}

	originalReq := m.findAndRemoveOriginalMemCopyRequest(rsp)
	originalMeta := originalReq

	rspToDriver := messaging.Msg{ID: m.comp.NewID(),
		Src:   m.toDriver().AsRemote(),
		Dst:   originalMeta.Src,
		RspTo: originalMeta.ID, Payload: rsp.Payload}

	m.toDriver().Send(rspToDriver)
	m.toDMA().RetrieveIncoming()

	m.comp.TaskTracker().EndReqIn(m.comp.State.CopyRecvTasks[originalReq.ID])
	delete(m.comp.State.CopyRecvTasks, originalReq.ID)
	// End the sender-side task of the cloned request that was sent to the
	// DMA engine.
	m.comp.TaskTracker().EndReqOut(m.comp.State.CopySendTasks[rsp.RspTo])
	delete(m.comp.State.CopySendTasks, rsp.RspTo)

	return true
}

func (m *cpMiddleware) findAndRemoveOriginalMemCopyRequest(
	rsp messaging.Msg,
) messaging.Msg {
	rspTo := rsp.RspTo
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
	req messaging.Msg,
) bool {
	state := &m.comp.State
	if state.CtrlSeq != ctrlSeqNone {
		return false
	}

	d := m.findAvailableDispatcher()
	if d == nil {
		return false
	}

	// The L1 caches are not coherent, so they must drop the lines that
	// previous kernels and copies may have made stale before the kernel
	// starts (the acquire at kernel start). The request stays in the buffer
	// while the kernel-start sequence runs; see ctrlMiddleware.go.
	if state.KernelStartReqID != req.ID {
		state.KernelStartReqID = req.ID
		m.ctrlMW().startSeq(ctrlSeqKernelStart)

		if state.CtrlSeq != ctrlSeqNone {
			return true
		}
	}

	state.KernelStartReqID = 0

	if m.comp.Resources.Sampler != nil && !m.comp.Resources.Sampler.IfDisable() {
		m.comp.Resources.Sampler.Reset()
	}
	d.StartDispatching(req)

	// A dispatcher was free, so the kernel launch is admitted now. If it had to
	// wait in the incoming buffer for a free dispatcher, this marks the
	// resolution of that wait on the buffer task.
	m.toDriver().AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindHardwareResource, m.comp.Name()+".dispatcher"))

	m.toDriver().RetrieveIncoming()

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
func (m *cpMiddleware) processFlushReq(req messaging.Msg) bool {
	state := &m.comp.State
	if state.CtrlSeq != ctrlSeqNone {
		return false
	}

	// The previous flush's completion may still be queued in
	// PendingDriverRsps; it reads CurrFlushReq when sent, so wait until
	// that response has gone out and cleared it.
	if state.CurrFlushReq.ID != 0 {
		return false
	}

	state.CurrFlushReq = req
	m.ctrlMW().startSeq(ctrlSeqFlush)

	m.toDriver().RetrieveIncoming()

	m.comp.State.FlushTaskID = m.comp.TaskTracker().StartReqIn(m.comp, req)

	return true
}

func (m *cpMiddleware) ctrlMW() *ctrlMiddleware { return m.comp.Middlewares.Control }

func (m *cpMiddleware) processMemCopyReq(req messaging.Msg) bool {
	state := &m.comp.State
	if state.CtrlSeq != ctrlSeqNone {
		return false
	}

	if !m.toDMA().CanSend() {
		return false
	}

	var cloned messaging.Msg
	switch req.Payload.(type) {
	case protocol.MemCopyH2DReq:

		c := req
		c.ID = m.comp.NewID()
		c.Src = m.toDMA().AsRemote()
		c.Dst = state.DMAEngine
		state.BottomMemCopyH2DToTop[c.ID] = req
		cloned = c
	case protocol.MemCopyD2HReq:

		c := req
		c.ID = m.comp.NewID()
		c.Src = m.toDMA().AsRemote()
		c.Dst = state.DMAEngine
		state.BottomMemCopyD2HToTop[c.ID] = req
		cloned = c
	default:
		panic("unknown type")
	}

	m.comp.State.CopyRecvTasks[req.ID] = m.comp.TaskTracker().StartReqIn(m.comp, req)
	m.comp.State.CopySendTasks[cloned.ID] = m.comp.TaskTracker().
		StartReqOut(m.comp, cloned, m.comp.State.CopyRecvTasks[req.ID])
	m.toDMA().Send(cloned)

	// The downstream port to the DMA engine was free, so the copy is admitted
	// now. If it had to wait in the incoming buffer for the port to drain, this
	// marks the resolution of that wait on the buffer task.
	m.toDriver().AddIncomingMilestone(tasks.Reason(tasks.MilestoneKindNetworkBusy, "ToDMA"))

	m.toDriver().RetrieveIncoming()

	return true
}

func (m *cpMiddleware) Handle(timing.Event) bool { return m.Tick() }
