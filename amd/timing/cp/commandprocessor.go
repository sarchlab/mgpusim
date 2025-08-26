package cp

import (
	"log"

	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cp/internal/dispatching"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cp/internal/resource"
)

// CommandProcessor is an Akita component that is responsible for receiving
// requests from the driver and dispatch the requests to other parts of the
// GPU.
type CommandProcessor struct {
	*sim.TickingComponent

	Dispatchers        []dispatching.Dispatcher
	DMAEngine          sim.Port
	Driver             sim.Port
	TLBs               []sim.Port
	CUs                []sim.RemotePort
	AddressTranslators []sim.Port
	RDMA               sim.Port
	PMC                sim.Port
	L1VCaches          []sim.Port
	L1SCaches          []sim.Port
	L1ICaches          []sim.Port
	L2Caches           []sim.Port
	DRAMControllers    []*idealmemcontroller.Comp

	ToDriver             sim.Port
	ToDMA                sim.Port
	ToCUs                sim.Port
	ToTLBs               sim.Port
	ToAddressTranslators sim.Port
	ToCaches             sim.Port
	ToRDMA               sim.Port
	ToPMC                sim.Port

	currShootdownRequest *protocol.ShootDownCommand
	currFlushRequest     *protocol.FlushReq

	//numTLBs                      uint64 //unused
	numCUAck                     uint64
	numAddrTranslationFlushAck   uint64
	numAddrTranslationRestartAck uint64
	numTLBAck                    uint64
	numCacheACK                  uint64

	shootDownInProcess bool

	bottomKernelLaunchReqIDToTopReqMap map[string]*protocol.LaunchKernelReq
	bottomMemCopyH2DReqIDToTopReqMap   map[string]*protocol.MemCopyH2DReq
	bottomMemCopyD2HReqIDToTopReqMap   map[string]*protocol.MemCopyD2HReq

	middleware     *cpMiddleware
	ctrlMiddleware *ctrlMiddleware
}

// CUInterfaceForCP defines the interface that a CP requires from CU.
type CUInterfaceForCP interface {
	resource.DispatchableCU

	// ControlPort returns a port on the CU that the CP can send controlling
	// messages to.
	ControlPort() sim.RemotePort
}

// RegisterCU allows the Command Processor to control the CU.
func (p *CommandProcessor) RegisterCU(cu CUInterfaceForCP) {
	p.CUs = append(p.CUs, cu.ControlPort())
	for _, d := range p.Dispatchers {
		d.RegisterCU(cu)
	}
}

// Tick ticks
func (p *CommandProcessor) Tick() bool {
	madeProgress := false

	madeProgress = p.tickDispatchers() || madeProgress
	madeProgress = p.processReqFromDriver() || madeProgress
	madeProgress = p.processRspFromInternal() || madeProgress

	return madeProgress
}

func (p *CommandProcessor) tickDispatchers() (madeProgress bool) {
	for _, d := range p.Dispatchers {
		madeProgress = d.Tick() || madeProgress
	}

	return madeProgress
}

// func (p *CommandProcessor) processReqFromDriver() bool {
// 	msg := p.ToDriver.PeekIncoming()
// 	if msg == nil {
// 		return false
// 	}

// 	switch req := msg.(type) {
// 	case *protocol.LaunchKernelReq:
// 		return p.processLaunchKernelReq(req)
// 	case *protocol.FlushReq:
// 		return p.processFlushReq(req)
// 	case *protocol.MemCopyD2HReq, *protocol.MemCopyH2DReq:
// 		return p.processMemCopyReq(req)
// 		// case *protocol.RDMADrainCmdFromDriver:
// 		// 	return p.processRDMADrainCmd(req)
// 		// case *protocol.RDMARestartCmdFromDriver:
// 		// 	return p.processRDMARestartCommand(req)
// 		// case *protocol.ShootDownCommand:
// 		// 	return p.processShootdownCommand(req)
// 		// case *protocol.GPURestartReq:
// 		// 	return p.processGPURestartReq(req)
// 		// case *protocol.PageMigrationReqToCP:
// 		// 	return p.processPageMigrationReq(req)
// 	}

// 	panic("never")
// }

func (p *CommandProcessor) processReqFromDriver() bool {
	madeProgress := false
	msg := p.ToDriver.PeekIncoming()

	if msg == nil {
		return madeProgress
	}

	madeProgress = p.middleware.Tick() || madeProgress
	madeProgress = p.ctrlMiddleware.Tick() || madeProgress

	if !madeProgress {
		log.Panicf("Unhandled message in Command Processor: %v", msg)
	}
	return madeProgress
}

// func (p *CommandProcessor) processRspFromInternal() bool {
// 	madeProgress := false

// 	madeProgress = p.processRspFromDMAs() || madeProgress
// 	madeProgress = p.processRspFromRDMAs() || madeProgress
// 	madeProgress = p.processRspFromCUs() || madeProgress
// 	madeProgress = p.processRspFromATs() || madeProgress
// 	madeProgress = p.processRspFromCaches() || madeProgress
// 	madeProgress = p.processRspFromTLBs() || madeProgress
// 	madeProgress = p.processRspFromPMC() || madeProgress

// 	return madeProgress
// }

func (p *CommandProcessor) processRspFromInternal() bool {
	madeProgress := false

	madeProgress = p.middleware.Tick() || madeProgress
	madeProgress = p.ctrlMiddleware.Tick() || madeProgress

	return madeProgress
}
