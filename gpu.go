package mgpusim

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/cache/writeback"
	"gitlab.com/akita/mem/vm/addresstranslator"
	"gitlab.com/akita/mem/vm/tlb"
	"gitlab.com/akita/mgpusim/pagemigrationcontroller"
	"gitlab.com/akita/mgpusim/rdma"
	"gitlab.com/akita/mgpusim/timing/caches/rob"
	"gitlab.com/akita/mgpusim/timing/cp"
)

// A GPU is a wrapper that holds all the subcomponent of a GPU.
type GPU struct {
	CommandProcessor   *cp.CommandProcessor
	RDMAEngine         *rdma.Engine
	PMC                *pagemigrationcontroller.PageMigrationController
	Dispatchers        []akita.Component
	CUs                []akita.Component
	L1VCaches          []akita.Component
	L1ICaches          []akita.Component
	L1SCaches          []akita.Component
	L2Caches           []*writeback.Cache
	L2CacheFinder      cache.LowModuleFinder
	L2TLBs             []*tlb.TLB
	L1VTLBs            []*tlb.TLB
	L1STLBs            []*tlb.TLB
	L1ITLBs            []*tlb.TLB
	L1VROBs            []*rob.ReorderBuffer
	L1IROBs            []*rob.ReorderBuffer
	L1SROBs            []*rob.ReorderBuffer
	ReorderBuffers     []*rob.ReorderBuffer
	L1VAddrTranslator  []*addresstranslator.AddressTranslator
	L1IAddrTranslator  []*addresstranslator.AddressTranslator
	L1SAddrTranslator  []*addresstranslator.AddressTranslator
	MemoryControllers  []akita.Component
	Storage            *mem.Storage
	InternalConnection akita.Connection

	GPUID uint64
}

// ExternalPorts returns external ports
func (g *GPU) ExternalPorts() []akita.Port {
	ports := []akita.Port{
		g.CommandProcessor.ToDriver,
		g.RDMAEngine.ToOutside,
		g.PMC.RemotePort,
	}

	for _, l2tlb := range g.L2TLBs {
		ports = append(ports, l2tlb.BottomPort)
	}

	return ports
}

// NewGPU returns a newly created GPU
func NewGPU(name string) *GPU {
	g := new(GPU)

	return g
}
