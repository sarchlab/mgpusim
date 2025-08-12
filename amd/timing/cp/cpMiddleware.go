package cp

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
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

	for _, port := range m.L1ICaches {
		m.flushCache(port)
	}

	for _, port := range m.L1SCaches {
		m.flushCache(port)
	}

	for _, port := range m.L1VCaches {
		m.flushCache(port)
	}

	for _, port := range m.L2Caches {
		m.flushCache(port)
	}

	m.currFlushRequest = req
	if m.numCacheACK == 0 {
		rsp := sim.GeneralRspBuilder{}.
			WithSrc(m.ToDriver.AsRemote()).
			WithDst(m.Driver.AsRemote()).
			WithOriginalReq(req).
			Build()
		m.ToDriver.Send(rsp)
	}

	m.ToDriver.RetrieveIncoming()

	tracing.TraceReqReceive(req, m.CommandProcessor)

	return true
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
	flushReq := cache.FlushReqBuilder{}.
		WithSrc(m.ToCaches.AsRemote()).
		WithDst(port.AsRemote()).
		Build()

	err := m.ToCaches.Send(flushReq)
	if err != nil {
		panic(err)
	}

	m.numCacheACK++
}
