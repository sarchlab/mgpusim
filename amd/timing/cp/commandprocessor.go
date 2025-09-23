package cp

import (
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

func (p *CommandProcessor) processReqFromDriver() bool {
	madeProgress := false
	msg := p.ToDriver.PeekIncoming()

	if msg == nil {
		return madeProgress
	}

	madeProgress = p.middleware.Tick() || madeProgress
	madeProgress = p.ctrlMiddleware.Tick() || madeProgress

	if !madeProgress {
		return false
	}
	return madeProgress
}

func (p *CommandProcessor) processRspFromInternal() bool {
	madeProgress := false

	madeProgress = p.middleware.Tick() || madeProgress
	madeProgress = p.ctrlMiddleware.Tick() || madeProgress

	return madeProgress
}
