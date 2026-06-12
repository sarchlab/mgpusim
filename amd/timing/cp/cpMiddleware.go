package cp

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"github.com/sarchlab/mgpusim/v4/amd/sampling"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cp/internal/dispatching"
)

type cpMiddleware struct {
	*CommandProcessor
}

func (m *cpMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.Handle() || madeProgress
	madeProgress = m.HandleInternal() || madeProgress
	return madeProgress
}

func (m *cpMiddleware) Handle() bool {
	msg := m.ToDriver.PeekIncoming()

	switch req := msg.(type) {
	case *protocol.LaunchKernelReq:
		return m.processLaunchKernelReq(req)
	case *protocol.FlushReq:
		return m.processFlushReq(req)
	case *protocol.MemCopyH2DReq, *protocol.MemCopyD2HReq:
		return m.processMemCopyReq(req)
	}
	return false
}

func (m *cpMiddleware) HandleInternal() bool {
	madeProgress := false
	madeProgress = m.processRspFromDMAs() || madeProgress
	return madeProgress
}

func (m *cpMiddleware) processRspFromDMAs() bool {
	msg := m.ToDMA.PeekIncoming()
	if msg == nil {
		return false
	}

	// TODO: V5 migration - sim.GeneralRsp removed. Handle response by msg type.
	return m.processMemCopyRsp(msg)
}

func (m *cpMiddleware) processMemCopyRsp(
	req sim.Msg,
) bool {
	originalReq := m.findAndRemoveOriginalMemCopyRequest(req)

	// Send the original request object back to the driver so the driver
	// can find it by pointer comparison in findCommandByReq.
	origSrc := originalReq.Meta().Src
	originalReq.Meta().Src = m.ToDriver.AsRemote()
	originalReq.Meta().Dst = origSrc

	m.ToDriver.Send(originalReq)
	m.ToDMA.RetrieveIncoming()

	tracing.TraceReqComplete(originalReq, m.CommandProcessor)
	tracing.TraceReqFinalize(req, m.CommandProcessor)

	return true
}

func (m *cpMiddleware) findAndRemoveOriginalMemCopyRequest(
	rsp sim.Msg,
) sim.Msg {
	rspTo := rsp.Meta().RspTo

	originalH2DReq, ok := m.bottomMemCopyH2DReqIDToTopReqMap[rspTo]
	if ok {
		delete(m.bottomMemCopyH2DReqIDToTopReqMap, rspTo)
		return originalH2DReq
	}

	originalD2HReq, ok := m.bottomMemCopyD2HReqIDToTopReqMap[rspTo]
	if ok {
		delete(m.bottomMemCopyD2HReqIDToTopReqMap, rspTo)
		return originalD2HReq
	}

	panic("never")
}

func (m *cpMiddleware) processLaunchKernelReq(
	req *protocol.LaunchKernelReq,
) bool {
	d := m.findAvailableDispatcher()

	if d == nil {
		return false
	}

	if *sampling.SampledRunnerFlag {
		sampling.SampledEngineInstance.Reset()
	}
	d.StartDispatching(req)
	m.ToDriver.RetrieveIncoming()

	tracing.TraceReqReceive(req, m.CommandProcessor)

	return true
}

func (m *cpMiddleware) findAvailableDispatcher() dispatching.Dispatcher {
	for _, d := range m.Dispatchers {
		if !d.IsDispatching() {
			return d
		}
	}

	return nil
}

func (m *cpMiddleware) processFlushReq(
	req *protocol.FlushReq,
) bool {
	if m.numCacheACK > 0 {
		return false
	}

	m.currFlushRequest = req

	// Phase 1: Flush L1 caches first. L1V caches are write-through to L2,
	// so all L1 flushes must complete before L2 flush begins. Otherwise,
	// L2 may flush and miss late-arriving write-through data from L1V.
	m.flushPhase = 1

	for _, port := range m.L1ICaches {
		m.flushCache(port)
	}

	for _, port := range m.L1SCaches {
		m.flushCache(port)
	}

	for _, port := range m.L1VCaches {
		m.flushCache(port)
	}

	// If no L1 caches, start phase 2 immediately
	if m.numCacheACK == 0 {
		m.startL2CacheFlush()
	}

	m.ToDriver.RetrieveIncoming()

	tracing.TraceReqReceive(req, m.CommandProcessor)

	return true
}

// startL2CacheFlush begins phase 2 of the two-phase flush by sending
// flush commands to all L2 caches. If there are no L2 caches, it
// completes the flush immediately.
func (m *cpMiddleware) startL2CacheFlush() {
	m.flushPhase = 2

	for _, port := range m.L2Caches {
		m.flushCache(port)
	}

	// If no L2 caches, respond immediately
	if m.numCacheACK == 0 {
		m.flushPhase = 0
		req := m.currFlushRequest
		origSrc := req.Meta().Src
		req.Meta().Src = m.ToDriver.AsRemote()
		req.Meta().Dst = origSrc
		m.ToDriver.Send(req)
		m.currFlushRequest = nil
	}
}

func (m *cpMiddleware) processMemCopyReq(
	req sim.Msg,
) bool {
	if m.numCacheACK > 0 {
		return false
	}

	var cloned sim.Msg
	switch req := req.(type) {
	case *protocol.MemCopyH2DReq:
		cloned = m.cloneMemCopyH2DReq(req)
	case *protocol.MemCopyD2HReq:
		cloned = m.cloneMemCopyD2HReq(req)
	default:
		panic("unknown type")
	}

	cloned.Meta().Dst = m.DMAEngine.AsRemote()
	cloned.Meta().Src = m.ToDMA.AsRemote()

	m.ToDMA.Send(cloned)
	m.ToDriver.RetrieveIncoming()

	tracing.TraceReqReceive(req, m.CommandProcessor)
	tracing.TraceReqInitiate(cloned, m.CommandProcessor, tracing.MsgIDAtReceiver(req, m.CommandProcessor))

	return true
}

func (m *cpMiddleware) cloneMemCopyH2DReq(
	req *protocol.MemCopyH2DReq,
) *protocol.MemCopyH2DReq {
	cloned := *req
	cloned.ID = sim.GetIDGenerator().Generate()
	m.bottomMemCopyH2DReqIDToTopReqMap[cloned.ID] = req
	return &cloned
}

func (m *cpMiddleware) cloneMemCopyD2HReq(
	req *protocol.MemCopyD2HReq,
) *protocol.MemCopyD2HReq {
	cloned := *req
	cloned.ID = sim.GetIDGenerator().Generate()
	m.bottomMemCopyD2HReqIDToTopReqMap[cloned.ID] = req
	return &cloned
}

func (m *cpMiddleware) flushCache(port sim.Port) {
	flushReq := &mem.ControlReq{
		Command: mem.CmdFlush,
	}
	flushReq.ID = sim.GetIDGenerator().Generate()
	flushReq.Src = m.ToCaches.AsRemote()
	flushReq.Dst = port.AsRemote()

	err := m.ToCaches.Send(flushReq)
	if err != nil {
		panic(err)
	}

	m.numCacheACK++
}
