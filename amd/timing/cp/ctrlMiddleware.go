package cp

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"github.com/sarchlab/mgpusim/v4/amd/timing/pagemigrationcontroller"
	"github.com/sarchlab/mgpusim/v4/amd/timing/rdma"
)

type ctrlMiddleware struct {
	*CommandProcessor
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.Handle() || madeProgress
	madeProgress = m.HandleInternal() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) Handle() bool {
	msg := m.ToDriver.PeekIncoming()

	switch req := msg.(type) {
	case *protocol.RDMADrainCmdFromDriver:
		return m.processRDMADrainCmd(req)
	case *protocol.RDMARestartCmdFromDriver:
		return m.processRDMARestartCommand(req)
	case *protocol.ShootDownCommand:
		return m.processShootdownCommand(req)
	case *protocol.GPURestartReq:
		return m.processGPURestartReq(req)
	case *protocol.PageMigrationReqToCP:
		return m.processPageMigrationReq(req)
	}
	return false
}

func (m *ctrlMiddleware) HandleInternal() bool {
	madeProgress := false

	// Retry sending flush response if a previous attempt failed.
	if m.pendingFlushRsp {
		madeProgress = m.retryFlushResponse() || madeProgress
	}

	madeProgress = m.processRspFromRDMAs() || madeProgress
	madeProgress = m.processRspFromCUs() || madeProgress
	madeProgress = m.processRspFromATs() || madeProgress
	madeProgress = m.processRspFromCaches() || madeProgress
	madeProgress = m.processRspFromTLBs() || madeProgress
	madeProgress = m.processRspFromPMC() || madeProgress
	return madeProgress
}

func (m *ctrlMiddleware) processRspFromRDMAs() bool {
	msg := m.ToRDMA.PeekIncoming()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *rdma.DrainRsp:
		return m.processRDMADrainRsp(req)
	case *rdma.RestartRsp:
		return m.processRDMARestartRsp(req)
	}

	panic("never")
}

func (m *ctrlMiddleware) processRspFromCUs() bool { //ctrl
	msg := m.ToCUs.PeekIncoming()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *protocol.CUPipelineFlushRsp:
		return m.processCUPipelineFlushRsp(req)
	case *protocol.CUPipelineRestartRsp:
		return m.processCUPipelineRestartRsp(req)
	}

	return false
}

func (m *ctrlMiddleware) processRspFromCaches() bool {
	msg := m.ToCaches.PeekIncoming()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *mem.ControlRsp:
		if m.numCacheACK > 0 {
			if req.Command == mem.CmdFlush {
				return m.processCacheFlushRsp(req)
			}
			return m.processCacheRestartRsp(req)
		}
	}

	panic("never")
}

func (m *ctrlMiddleware) processRspFromATs() bool {
	item := m.ToAddressTranslators.PeekIncoming()
	if item == nil {
		return false
	}

	msg := item.(*mem.ControlRsp)

	if m.numAddrTranslationFlushAck > 0 {
		return m.processAddressTranslatorFlushRsp(msg)
	} else if m.numAddrTranslationRestartAck > 0 {
		return m.processAddressTranslatorRestartRsp(msg)
	}

	panic("never")
}

func (m *ctrlMiddleware) processRspFromTLBs() bool {
	msg := m.ToTLBs.PeekIncoming()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *mem.ControlRsp:
		if m.numTLBAck > 0 {
			if req.Command == mem.CmdFlush {
				return m.processTLBFlushRsp(req)
			}
			return m.processTLBRestartRsp(req)
		}
	}

	panic("never")
}

func (m *ctrlMiddleware) processRspFromPMC() bool {
	msg := m.ToPMC.PeekIncoming()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *pagemigrationcontroller.PageMigrationRspFromPMC:
		return m.processPageMigrationRsp(req)
	}

	panic("never")
}

func (m *ctrlMiddleware) processRDMADrainRsp(
	rsp *rdma.DrainRsp,
) bool {
	req := protocol.NewRDMADrainRspToDriver(m.ToDriver, m.Driver)

	err := m.ToDriver.Send(req)
	if err != nil {
		panic(err)
	}

	m.ToRDMA.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processCUPipelineFlushRsp(
	rsp *protocol.CUPipelineFlushRsp,
) bool {
	m.numCUAck--

	if m.numCUAck == 0 {
		for i := 0; i < len(m.AddressTranslators); i++ {
			req := &mem.ControlReq{
				Command:         mem.CmdDrain,
				DiscardInflight: true,
			}
			req.ID = sim.GetIDGenerator().Generate()
			req.Src = m.ToAddressTranslators.AsRemote()
			req.Dst = m.AddressTranslators[i].AsRemote()
			m.ToAddressTranslators.Send(req)
			m.numAddrTranslationFlushAck++
		}
	}

	m.ToCUs.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processAddressTranslatorFlushRsp(
	msg *mem.ControlRsp,
) bool {
	m.numAddrTranslationFlushAck--

	if m.numAddrTranslationFlushAck == 0 {
		for _, port := range m.L1SCaches {
			m.flushAndResetL1Cache(port)
		}

		for _, port := range m.L1VCaches {
			m.flushAndResetL1Cache(port)
		}

		for _, port := range m.L1ICaches {
			m.flushAndResetL1Cache(port)
		}

		for _, port := range m.L2Caches {
			m.flushAndResetL2Cache(port)
		}
	}

	m.ToAddressTranslators.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) flushAndResetL1Cache(
	port sim.Port,
) {
	req := &mem.ControlReq{
		Command:         mem.CmdFlush,
		DiscardInflight: true,
		InvalidateAfter: true,
		PauseAfter:      true,
	}
	req.ID = sim.GetIDGenerator().Generate()
	req.Src = m.ToCaches.AsRemote()
	req.Dst = port.AsRemote()

	m.ToCaches.Send(req)
	m.numCacheACK++
}

func (m *ctrlMiddleware) flushAndResetL2Cache(port sim.Port) {
	req := &mem.ControlReq{
		Command:         mem.CmdFlush,
		DiscardInflight: true,
		InvalidateAfter: true,
		PauseAfter:      true,
	}
	req.ID = sim.GetIDGenerator().Generate()
	req.Src = m.ToCaches.AsRemote()
	req.Dst = port.AsRemote()

	m.ToCaches.Send(req)
	m.numCacheACK++
}

func (m *ctrlMiddleware) processCacheFlushRsp(
	rsp *mem.ControlRsp,
) bool {
	m.numCacheACK--
	m.ToCaches.RetrieveIncoming()

	if m.numCacheACK == 0 {
		if m.flushPhase == 1 {
			// Phase 1 (L1 flush) complete. Start phase 2 (L2 flush).
			m.flushPhase = 2
			for _, port := range m.L2Caches {
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

			if m.numCacheACK > 0 {
				return true
			}
			// No L2 caches — fall through to complete the flush
		}

		if m.flushPhase == 2 {
			m.flushPhase = 0
			return m.processRegularCacheFlush(rsp)
		}

		if m.shootDownInProcess {
			return m.processCacheFlushCausedByTLBShootdown(rsp)
		}
		return m.processRegularCacheFlush(rsp)
	}

	return true
}

func (m *ctrlMiddleware) processRegularCacheFlush(
	flushRsp *mem.ControlRsp,
) bool {
	// Prepare the original flush request to send back to the driver.
	req := m.currFlushRequest
	if !m.pendingFlushRsp {
		// Swap Src/Dst only once (skip on retry).
		origSrc := req.Meta().Src
		req.Meta().Src = m.ToDriver.AsRemote()
		req.Meta().Dst = origSrc
	}

	err := m.ToDriver.Send(req)
	if err != nil {
		// Port busy — mark for retry on the next tick.
		m.pendingFlushRsp = true
		return false
	}

	tracing.TraceReqComplete(req, m)
	m.currFlushRequest = nil
	m.pendingFlushRsp = false

	return true
}

// retryFlushResponse retries sending the flush response to the driver
// after a previous attempt failed due to a busy port.
func (m *ctrlMiddleware) retryFlushResponse() bool {
	if m.currFlushRequest == nil {
		m.pendingFlushRsp = false
		return false
	}

	req := m.currFlushRequest
	err := m.ToDriver.Send(req)
	if err != nil {
		return false // Still busy, will retry next tick
	}

	tracing.TraceReqComplete(req, m)
	m.currFlushRequest = nil
	m.pendingFlushRsp = false

	return true
}

func (m *ctrlMiddleware) processCacheFlushCausedByTLBShootdown(
	flushRsp *mem.ControlRsp,
) bool {
	m.currFlushRequest = nil

	for i := 0; i < len(m.TLBs); i++ {
		shootDownCmd := m.currShootdownRequest
		req := &mem.ControlReq{
			Command: mem.CmdFlush,
		}
		req.ID = sim.GetIDGenerator().Generate()
		req.Src = m.ToTLBs.AsRemote()
		req.Dst = m.TLBs[i].AsRemote()
		// TODO: V5 migration - TLB flush with PID/VAddrs needs ControlReq extension
		_ = shootDownCmd

		m.ToTLBs.Send(req)
		m.numTLBAck++
	}

	return true
}

func (m *ctrlMiddleware) processTLBFlushRsp(
	rsp *mem.ControlRsp,
) bool {
	m.numTLBAck--

	if m.numTLBAck == 0 {
		req := protocol.NewShootdownCompleteRsp(m.ToDriver, m.Driver)
		m.ToDriver.Send(req)

		m.shootDownInProcess = false
	}

	m.ToTLBs.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRDMARestartRsp(rsp *rdma.RestartRsp) bool {
	req := protocol.NewRDMARestartRspToDriver(m.ToDriver, m.Driver)
	m.ToDriver.Send(req)
	m.ToRDMA.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processCacheRestartRsp(
	rsp *mem.ControlRsp,
) bool {
	m.numCacheACK--
	if m.numCacheACK == 0 {
		for i := 0; i < len(m.TLBs); i++ {
			m.numTLBAck++

			req := &mem.ControlReq{
				Command: mem.CmdEnable,
			}
			req.ID = sim.GetIDGenerator().Generate()
			req.Src = m.ToTLBs.AsRemote()
			req.Dst = m.TLBs[i].AsRemote()
			m.ToTLBs.Send(req)
		}
	}

	m.ToCaches.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processTLBRestartRsp(
	rsp *mem.ControlRsp,
) bool {
	m.numTLBAck--

	if m.numTLBAck == 0 {
		for i := 0; i < len(m.AddressTranslators); i++ {
			req := &mem.ControlReq{
				Command: mem.CmdEnable,
			}
			req.ID = sim.GetIDGenerator().Generate()
			req.Src = m.ToAddressTranslators.AsRemote()
			req.Dst = m.AddressTranslators[i].AsRemote()
			m.ToAddressTranslators.Send(req)

			m.numAddrTranslationRestartAck++
		}
	}

	m.ToTLBs.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processAddressTranslatorRestartRsp(
	rsp *mem.ControlRsp,
) bool {
	m.numAddrTranslationRestartAck--

	if m.numAddrTranslationRestartAck == 0 {
		for i := 0; i < len(m.CUs); i++ {
			req := protocol.CUPipelineRestartReqBuilder{}.
				WithSrc(m.ToCUs.AsRemote()).
				WithDst(m.CUs[i]).
				Build()
			m.ToCUs.Send(req)

			m.numCUAck++
		}
	}

	m.ToAddressTranslators.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processCUPipelineRestartRsp(
	rsp *protocol.CUPipelineRestartRsp,
) bool {
	m.numCUAck--

	if m.numCUAck == 0 {
		rsp := protocol.NewGPURestartRsp(m.ToDriver, m.Driver)
		m.ToDriver.Send(rsp)
	}

	m.ToCUs.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processPageMigrationRsp(
	rsp *pagemigrationcontroller.PageMigrationRspFromPMC,
) bool {
	req := protocol.NewPageMigrationRspToDriver(m.ToDriver, m.Driver)

	err := m.ToDriver.Send(req)
	if err != nil {
		panic(err)
	}

	m.ToPMC.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRDMADrainCmd(
	cmd *protocol.RDMADrainCmdFromDriver,
) bool {
	req := rdma.DrainReqBuilder{}.
		WithSrc(m.ToRDMA.AsRemote()).
		WithDst(m.RDMA.AsRemote()).
		Build()

	err := m.ToRDMA.Send(req)
	if err != nil {
		panic(err)
	}

	m.ToDriver.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processRDMARestartCommand(
	cmd *protocol.RDMARestartCmdFromDriver,
) bool {
	req := rdma.RestartReqBuilder{}.
		WithSrc(m.ToRDMA.AsRemote()).
		WithDst(m.RDMA.AsRemote()).
		Build()

	m.ToRDMA.Send(req)

	m.ToDriver.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processShootdownCommand(
	cmd *protocol.ShootDownCommand,
) bool {
	if m.shootDownInProcess {
		return false
	}

	m.currShootdownRequest = cmd
	m.shootDownInProcess = true

	for i := 0; i < len(m.CUs); i++ {
		m.numCUAck++
		req := protocol.CUPipelineFlushReqBuilder{}.
			WithSrc(m.ToCUs.AsRemote()).
			WithDst(m.CUs[i]).
			Build()
		m.ToCUs.Send(req)
	}

	m.ToDriver.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processGPURestartReq(
	cmd *protocol.GPURestartReq,
) bool {
	for _, port := range m.L2Caches {
		m.restartCache(port)
	}
	for _, port := range m.L1ICaches {
		m.restartCache(port)
	}
	for _, port := range m.L1SCaches {
		m.restartCache(port)
	}

	for _, port := range m.L1VCaches {
		m.restartCache(port)
	}

	m.ToDriver.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processPageMigrationReq(
	cmd *protocol.PageMigrationReqToCP,
) bool {
	req := pagemigrationcontroller.PageMigrationReqToPMCBuilder{}.
		WithSrc(m.ToPMC.AsRemote()).
		WithDst(m.PMC.AsRemote()).
		WithPageSize(cmd.PageSize).
		WithPMCPortOfRemoteGPU(cmd.DestinationPMCPort.AsRemote()).
		WithReadFrom(cmd.ToReadFromPhysicalAddress).
		WithWriteTo(cmd.ToWriteToPhysicalAddress).
		Build()

	err := m.ToPMC.Send(req)
	if err != nil {
		panic(err)
	}

	m.ToDriver.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) restartCache(port sim.Port) {
	req := &mem.ControlReq{
		Command: mem.CmdEnable,
	}
	req.ID = sim.GetIDGenerator().Generate()
	req.Src = m.ToCaches.AsRemote()
	req.Dst = port.AsRemote()

	err := m.ToCaches.Send(req)
	if err != nil {
		panic(err)
	}

	m.numCacheACK++
}
