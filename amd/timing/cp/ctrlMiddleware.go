package cp

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm/tlb"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
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
	madeProgress = m.processRspFromRDMAs() || madeProgress
	madeProgress = m.processRspFromCUs() || madeProgress
	madeProgress = m.processRspFromROBs() || madeProgress
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
	case *cache.FlushRsp:
		return m.processCacheFlushRsp(req)
	case *cache.RestartRsp:
		return m.processCacheRestartRsp(req)
	}

	panic("never")
}

func (m *ctrlMiddleware) processRspFromATs() bool {
	item := m.ToAddressTranslators.PeekIncoming()
	if item == nil {
		return false
	}

	msg := item.(*mem.ControlMsg)

	if m.numAddrTranslationFlushAck > 0 {
		return m.processAddressTranslatorFlushRsp(msg)
	} else if m.numAddrTranslationRestartAck > 0 {
		return m.processAddressTranslatorRestartRsp(msg)
	}

	panic("never")
}

func (m *ctrlMiddleware) processRspFromROBs() bool {
	item := m.ToROBs.PeekIncoming()
	if item == nil {
		return false
	}

	msg := item.(*mem.ControlMsg)

	if m.numROBFlushAck > 0 {
		return m.processROBFlushRsp(msg)
	} else if m.numROBRestartAck > 0 {
		return m.processROBRestartRsp(msg)
	}

	panic("never")
}

func (m *ctrlMiddleware) processRspFromTLBs() bool {
	msg := m.ToTLBs.PeekIncoming()
	if msg == nil {
		return false
	}

	switch req := msg.(type) {
	case *tlb.FlushRsp:
		return m.processTLBFlushRsp(req)
	case *tlb.RestartRsp:
		return m.processTLBRestartRsp(req)
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
		for i := 0; i < len(m.ROBs); i++ {
			req := mem.ControlMsgBuilder{}.
				WithSrc(m.ToROBs.AsRemote()).
				WithDst(m.ROBs[i].AsRemote()).
				ToDiscardTransactions().
				Build()
			m.ToROBs.Send(req)
			m.numROBFlushAck++
		}
	}

	m.ToCUs.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processROBFlushRsp(
	msg *mem.ControlMsg,
) bool {
	m.numROBFlushAck--

	if m.numROBFlushAck == 0 {
		for i := 0; i < len(m.AddressTranslators); i++ {
			req := mem.ControlMsgBuilder{}.
				WithSrc(m.ToAddressTranslators.AsRemote()).
				WithDst(m.AddressTranslators[i].AsRemote()).
				ToDiscardTransactions().
				Build()
			m.ToAddressTranslators.Send(req)
			m.numAddrTranslationFlushAck++
		}
	}

	m.ToROBs.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processAddressTranslatorFlushRsp(
	msg *mem.ControlMsg,
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
	req := cache.FlushReqBuilder{}.
		WithSrc(m.ToCaches.AsRemote()).
		WithDst(port.AsRemote()).
		PauseAfterFlushing().
		DiscardInflight().
		InvalidateAllCacheLines().
		Build()

	m.ToCaches.Send(req)
	m.numCacheACK++
}

func (m *ctrlMiddleware) flushAndResetL2Cache(port sim.Port) {
	req := cache.FlushReqBuilder{}.
		WithSrc(m.ToCaches.AsRemote()).
		WithDst(port.AsRemote()).
		PauseAfterFlushing().
		DiscardInflight().
		InvalidateAllCacheLines().
		Build()

	m.ToCaches.Send(req)
	m.numCacheACK++
}

func (m *ctrlMiddleware) processCacheFlushRsp(
	rsp *cache.FlushRsp,
) bool {
	m.numCacheACK--
	m.ToCaches.RetrieveIncoming()

	if m.numCacheACK == 0 {
		if m.shootDownInProcess {
			return m.processCacheFlushCausedByTLBShootdown(rsp)
		}
		return m.processRegularCacheFlush(rsp)
	}

	return true
}

func (m *ctrlMiddleware) processRegularCacheFlush(
	flushRsp *cache.FlushRsp,
) bool {
	rsp := sim.GeneralRspBuilder{}.
		WithSrc(m.ToDriver.AsRemote()).
		WithDst(m.currFlushRequest.Src).
		WithOriginalReq(m.currFlushRequest).
		Build()

	m.ToDriver.Send(rsp)

	tracing.TraceReqComplete(m.currFlushRequest, m)
	m.currFlushRequest = nil

	return true
}

func (m *ctrlMiddleware) processCacheFlushCausedByTLBShootdown(
	flushRsp *cache.FlushRsp,
) bool {
	m.currFlushRequest = nil

	for i := 0; i < len(m.TLBs); i++ {
		shootDownCmd := m.currShootdownRequest
		req := tlb.FlushReqBuilder{}.
			WithSrc(m.ToTLBs.AsRemote()).
			WithDst(m.TLBs[i].AsRemote()).
			WithPID(shootDownCmd.PID).
			WithVAddrs(shootDownCmd.VAddr).
			Build()

		m.ToTLBs.Send(req)
		m.numTLBAck++
	}

	return true
}

func (m *ctrlMiddleware) processTLBFlushRsp(
	rsp *tlb.FlushRsp,
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
	rsp *cache.RestartRsp,
) bool {
	m.numCacheACK--
	if m.numCacheACK == 0 {
		for i := 0; i < len(m.TLBs); i++ {
			m.numTLBAck++

			req := tlb.RestartReqBuilder{}.
				WithSrc(m.ToTLBs.AsRemote()).
				WithDst(m.TLBs[i].AsRemote()).
				Build()
			m.ToTLBs.Send(req)
		}
	}

	m.ToCaches.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processTLBRestartRsp(
	rsp *tlb.RestartRsp,
) bool {
	m.numTLBAck--

	if m.numTLBAck == 0 {
		for i := 0; i < len(m.AddressTranslators); i++ {
			req := mem.ControlMsgBuilder{}.
				WithSrc(m.ToAddressTranslators.AsRemote()).
				WithDst(m.AddressTranslators[i].AsRemote()).
				ToRestart().
				Build()
			m.ToAddressTranslators.Send(req)

			// fmt.Printf("Restarting %s\n", p.AddressTranslators[i].Name())

			m.numAddrTranslationRestartAck++
		}
	}

	m.ToTLBs.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processAddressTranslatorRestartRsp(
	rsp *mem.ControlMsg,
) bool {
	m.numAddrTranslationRestartAck--

	if m.numAddrTranslationRestartAck == 0 {
		for i := 0; i < len(m.ROBs); i++ {
			req := mem.ControlMsgBuilder{}.
				WithSrc(m.ToROBs.AsRemote()).
				WithDst(m.ROBs[i].AsRemote()).
				ToRestart().
				Build()
			m.ToROBs.Send(req)

			m.numROBRestartAck++
		}
	}

	m.ToAddressTranslators.RetrieveIncoming()

	return true
}

func (m *ctrlMiddleware) processROBRestartRsp(
	rsp *mem.ControlMsg,
) bool {
	m.numROBRestartAck--

	if m.numROBRestartAck == 0 {
		for i := 0; i < len(m.CUs); i++ {
			req := protocol.CUPipelineRestartReqBuilder{}.
				WithSrc(m.ToCUs.AsRemote()).
				WithDst(m.CUs[i]).
				Build()
			m.ToCUs.Send(req)

			m.numCUAck++
		}
	}

	m.ToROBs.RetrieveIncoming()

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
	req := cache.RestartReqBuilder{}.
		WithSrc(m.ToCaches.AsRemote()).
		WithDst(port.AsRemote()).
		Build()

	err := m.ToCaches.Send(req)
	if err != nil {
		panic(err)
	}

	m.numCacheACK++
}
