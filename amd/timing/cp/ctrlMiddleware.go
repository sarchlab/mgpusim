package cp

import (
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/sim"
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
