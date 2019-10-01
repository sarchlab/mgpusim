package gpubuilder

import (
	"fmt"
	"log"
	"os"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/gcn3/pagemigrationcontroller"
	"gitlab.com/akita/gcn3/rdma"
	"gitlab.com/akita/gcn3/timing"
	"gitlab.com/akita/gcn3/timing/caches/l1v"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/cache/writeback"
	"gitlab.com/akita/mem/idealmemcontroller"
	memtraces "gitlab.com/akita/mem/trace"
	"gitlab.com/akita/mem/vm/addresstranslator"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/mem/vm/tlb"
	"gitlab.com/akita/util/tracing"
)

// R9NanoGPUBuilder can build R9 Nano GPUs.
type R9NanoGPUBuilder struct {
	engine              akita.Engine
	freq                akita.Freq
	memAddrOffset       uint64
	mmu                 *mmu.MMUImpl
	externalConn        akita.Connection
	numShaderArray      int
	numCUPerShaderArray int
	numMemoryBank       int

	EnableISADebug    bool
	EnableInstTracing bool
	EnableMemTracing  bool
	enableVisTracing  bool
	visTracer         tracing.Tracer
	MemTracer         tracing.Tracer

	gpuName                           string
	gpu                               *gcn3.GPU
	InternalConn                      *akita.DirectConnection
	CP                                *gcn3.CommandProcessor
	ACE                               *gcn3.Dispatcher
	L1VCaches                         []*l1v.Cache
	L1SCaches                         []*l1v.Cache
	L1ICaches                         []*l1v.Cache
	L2Caches                          []*writeback.Cache
	l1vAddrTrans                      []*addresstranslator.AddressTranslator
	l1sAddrTrans                      []*addresstranslator.AddressTranslator
	l1iAddrTrans                      []*addresstranslator.AddressTranslator
	L1VTLBs                           []*tlb.TLB
	L1STLBs                           []*tlb.TLB
	L1ITLBs                           []*tlb.TLB
	L2TLBs                            []*tlb.TLB
	DRAMs                             []*idealmemcontroller.Comp
	LowModuleFinderForL1              *cache.InterleavedLowModuleFinder
	LowModuleFinderForL2              *cache.InterleavedLowModuleFinder
	LowModuleFinderForPMC             *cache.InterleavedLowModuleFinder
	DMAEngine                         *gcn3.DMAEngine
	RDMAEngine                        *rdma.Engine
	PageMigrationController           *pagemigrationcontroller.PageMigrationController
	cuToL1VAddrTranslatorConnections  []*akita.DirectConnection
	cuToL1SAddrTranslatorConnections  []*akita.DirectConnection
	cuToL1IConnections                []*akita.DirectConnection
	addrTranslatorToL1VConnections    []*akita.DirectConnection
	addrTranslatorToL1SConnections    []*akita.DirectConnection
	addrTranslatorToTLBL1Connections  []*akita.DirectConnection
	addrTranslatorToSTLBL1Connections []*akita.DirectConnection
	l1TLBToL2TLBConnection            *akita.DirectConnection
	l1ToL2Connection                  *akita.DirectConnection
	l2ToDramConnections               []*akita.DirectConnection
	l1ITol1AddrTranslatorConnections  []*akita.DirectConnection
}

// MakeR9NanoGPUBuilder provides a GPU builder that can builds the R9Nano GPU.
func MakeR9NanoGPUBuilder() R9NanoGPUBuilder {
	b := R9NanoGPUBuilder{
		freq:                1 * akita.GHz,
		numShaderArray:      16,
		numCUPerShaderArray: 4,
		numMemoryBank:       8,
	}
	return b
}

// WithEngine sets the engine that the GPU use.
func (b R9NanoGPUBuilder) WithEngine(engine akita.Engine) R9NanoGPUBuilder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the GPU works at.
func (b R9NanoGPUBuilder) WithFreq(freq akita.Freq) R9NanoGPUBuilder {
	b.freq = freq
	return b
}

// WithExternalConn sets the external connection for CPU-GPU and inter-GPU
// communication.
func (b R9NanoGPUBuilder) WithExternalConn(
	conn akita.Connection,
) R9NanoGPUBuilder {
	b.externalConn = conn
	return b
}

// WithMemAddrOffset sets the address of the first byte of the GPU to build.
func (b R9NanoGPUBuilder) WithMemAddrOffset(
	offset uint64,
) R9NanoGPUBuilder {
	b.memAddrOffset = offset
	return b
}

// WithMMU sets the MMU component that provides the address translation service
// for the GPU.
func (b R9NanoGPUBuilder) WithMMU(mmu *mmu.MMUImpl) R9NanoGPUBuilder {
	b.mmu = mmu
	return b
}

// WithNumMemoryBank sets the number of L2 cache modules and number of memory
// controllers in each GPU.
func (b R9NanoGPUBuilder) WithNumMemoryBank(n int) R9NanoGPUBuilder {
	b.numMemoryBank = n
	return b
}

// WithNumShaderArray sets the number of shader arrays in each GPU. Each shader
// array contains a certain number of CUs, a certain number of L1V caches, 1
// L1S cache, and 1 L1V cache.
func (b R9NanoGPUBuilder) WithNumShaderArray(n int) R9NanoGPUBuilder {
	b.numShaderArray = n
	return b
}

// WithNumCUPerShaderArray sets the number of CU and number of L1V caches in
// each Shader Array.
func (b R9NanoGPUBuilder) WithNumCUPerShaderArray(n int) R9NanoGPUBuilder {
	b.numCUPerShaderArray = n
	return b
}

// WithVisTracer applies a tracer to trace all the tasks of all the GPU
// components
func (b R9NanoGPUBuilder) WithVisTracer(t tracing.Tracer) R9NanoGPUBuilder {
	b.enableVisTracing = true
	b.visTracer = t
	return b
}

// Build creates a pre-configure GPU similar to the AMD R9 Nano GPU.
func (b R9NanoGPUBuilder) Build(name string, ID uint64) *gcn3.GPU {
	b.gpuName = name

	b.InternalConn = akita.NewDirectConnection(b.engine)
	b.gpu = gcn3.NewGPU(b.gpuName, b.engine)

	b.gpu.GPUID = ID

	b.buildCP()
	b.buildMemSystem()
	b.buildDMAEngine()
	b.buildRDMAEngine()
	b.buildPageMigrationController()
	b.buildCUs()

	b.InternalConn.PlugIn(b.gpu.ToCommandProcessor)
	b.InternalConn.PlugIn(b.DMAEngine.ToCP)
	b.InternalConn.PlugIn(b.DMAEngine.ToMem)
	b.externalConn.PlugIn(b.gpu.ToDriver)

	b.gpu.InternalConnection = b.InternalConn

	b.connectCUToCP()

	return b.gpu
}

func (b *R9NanoGPUBuilder) reset() {
	b.L1VCaches = nil
	b.L1SCaches = nil
	b.L1ICaches = nil
	b.L2Caches = nil
	b.l1vAddrTrans = nil
	b.l1sAddrTrans = nil
	b.l1iAddrTrans = nil
	b.L1VTLBs = nil
	b.L1STLBs = nil
	b.L1ITLBs = nil
	b.L2TLBs = nil
	b.DRAMs = nil
}

func (b *R9NanoGPUBuilder) buildRDMAEngine() {
	b.RDMAEngine = rdma.NewEngine(
		fmt.Sprintf("%s.RDMA", b.gpuName),
		b.engine,
		b.LowModuleFinderForL2,
		nil,
	)
	b.gpu.RDMAEngine = b.RDMAEngine
	b.LowModuleFinderForL1.ModuleForOtherAddresses = b.RDMAEngine.ToL1
	b.InternalConn.PlugIn(b.gpu.RDMAEngine.ToL1)
	b.InternalConn.PlugIn(b.gpu.RDMAEngine.ToL2)
	b.CP.RDMA = b.gpu.RDMAEngine.CtrlPort
	b.InternalConn.PlugIn(b.CP.RDMA)

}

func (b *R9NanoGPUBuilder) buildPageMigrationController() {
	b.PageMigrationController = pagemigrationcontroller.NewPageMigrationController(
		fmt.Sprintf("%s.PMC", b.gpuName),
		b.engine,
		b.LowModuleFinderForPMC,
		nil)

	b.gpu.PMC = b.PageMigrationController

	b.gpu.PMC.MemCtrlFinder = b.LowModuleFinderForPMC

	b.InternalConn.PlugIn(b.gpu.PMC.CtrlPort)
	b.InternalConn.PlugIn(b.gpu.PMC.LocalMemPort)

	b.CP.PMC = b.gpu.PMC.CtrlPort
	b.InternalConn.PlugIn(b.CP.PMC)

}

func (b *R9NanoGPUBuilder) buildDMAEngine() {
	b.DMAEngine = gcn3.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.gpuName),
		b.engine,
		b.LowModuleFinderForL2)
	b.CP.DMAEngine = b.DMAEngine.ToCP

}

func (b *R9NanoGPUBuilder) buildCP() {
	b.CP = gcn3.NewCommandProcessor(b.gpuName+".CommandProcessor", b.engine)
	b.CP.Driver = b.gpu.ToCommandProcessor
	b.gpu.CommandProcessor = b.CP.ToDriver

	b.ACE = gcn3.NewDispatcher(
		b.gpuName+".Dispatcher",
		b.engine,
		kernels.NewGridBuilder())
	b.ACE.Freq = b.freq
	b.CP.Dispatcher = b.ACE.ToCommandProcessor
	b.gpu.Dispatchers = append(b.gpu.Dispatchers, b.ACE)

	b.InternalConn.PlugIn(b.CP.ToDriver)
	b.InternalConn.PlugIn(b.CP.ToDispatcher)
	b.InternalConn.PlugIn(b.CP.ToCaches)
	b.InternalConn.PlugIn(b.ACE.ToCommandProcessor)
	b.InternalConn.PlugIn(b.ACE.ToCUs)
	b.InternalConn.PlugIn(b.CP.ToCUs)

	b.InternalConn.PlugIn(b.CP.ToRDMA)
	b.InternalConn.PlugIn(b.CP.ToPMC)

	if b.enableVisTracing {
		tracing.CollectTrace(b.CP, b.visTracer)
		tracing.CollectTrace(b.ACE, b.visTracer)
	}
}

func (b *R9NanoGPUBuilder) connectCUToCP() {
	for i := 0; i < b.numCU(); i++ {
		b.CP.CUs = append(b.CP.CUs, akita.NewLimitNumMsgPort(b.CP, 1))
		b.InternalConn.PlugIn(b.CP.CUs[i])
		b.CP.CUs[i] = b.gpu.CUs[i].(*timing.ComputeUnit).ToCP
		b.CP.ToCUs = b.gpu.CUs[i].(*timing.ComputeUnit).CP
	}
}

func (b *R9NanoGPUBuilder) buildMemSystem() {
	if b.EnableMemTracing {
		file, err := os.Create("mem.trace")
		if err != nil {
			panic(err)
		}
		logger := log.New(file, "", 0)
		b.MemTracer = memtraces.NewTracer(logger)
	}

	b.buildMemControllers()
	b.buildTLBs()
	b.buildL2Caches()
	b.buildL1VCaches()
	b.buildL1VAddrTranslators()
	b.buildL1SCaches()
	b.buildL1SAddrTranslators()
	b.buildL1IAddrTranslators()
	b.buildL1ICaches()
}

func (b *R9NanoGPUBuilder) buildL1VAddrTranslators() {
	builder := addresstranslator.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumReqPerCycle(4).
		WithLog2PageSize(12).
		WithGPUID(b.gpu.GPUID)
	for i := 0; i < b.numCU(); i++ {
		lowModuleFinder :=
			&cache.SingleLowModuleFinder{LowModule: b.L1VCaches[i].TopPort}
		name := fmt.Sprintf("%s.L1VAddrTrans_%d", b.gpuName, i)
		at := builder.
			WithLowModuleFinder(lowModuleFinder).
			WithTranslationProvider(b.L1VTLBs[i].TopPort).
			Build(name)

		topConn := akita.NewDirectConnection(b.engine)
		b.cuToL1VAddrTranslatorConnections = append(
			b.cuToL1VAddrTranslatorConnections, topConn)
		topConn.PlugIn(at.TopPort)

		bottomConn := b.addrTranslatorToL1VConnections[i]
		bottomConn.PlugIn(at.BottomPort)

		translationConn := b.addrTranslatorToTLBL1Connections[i]
		translationConn.PlugIn(at.TranslationPort)
		b.InternalConn.PlugIn(at.CtrlPort)

		b.CP.AddressTranslators = append(b.CP.AddressTranslators, at.CtrlPort)

		b.l1vAddrTrans = append(b.l1vAddrTrans, at)

		if b.enableVisTracing {
			tracing.CollectTrace(at, b.visTracer)
		}
	}
	b.gpu.L1VAddrTranslator = append(
		[]*addresstranslator.AddressTranslator{},
		b.l1vAddrTrans...)
}

func (b *R9NanoGPUBuilder) buildL1SAddrTranslators() {
	builder := addresstranslator.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumReqPerCycle(4).
		WithLog2PageSize(12).
		WithGPUID(b.gpu.GPUID)
	for i := 0; i < b.numShaderArray; i++ {
		lowModuleFinder :=
			&cache.SingleLowModuleFinder{LowModule: b.L1SCaches[i].TopPort}
		name :=
			fmt.Sprintf("%s.L1SAddrTrans_%d", b.gpuName, i)
		at := builder.
			WithLowModuleFinder(lowModuleFinder).
			WithTranslationProvider(b.L1STLBs[i].TopPort).
			Build(name)

		topConn := akita.NewDirectConnection(b.engine)
		b.cuToL1SAddrTranslatorConnections = append(
			b.cuToL1SAddrTranslatorConnections, topConn)
		topConn.PlugIn(at.TopPort)

		bottomConn := b.addrTranslatorToL1SConnections[i]
		bottomConn.PlugIn(at.BottomPort)

		translationConn := b.addrTranslatorToSTLBL1Connections[i]
		translationConn.PlugIn(at.TranslationPort)
		b.InternalConn.PlugIn(at.CtrlPort)
		b.CP.AddressTranslators = append(b.CP.AddressTranslators, at.CtrlPort)

		b.l1sAddrTrans = append(b.l1sAddrTrans, at)

		if b.enableVisTracing {
			tracing.CollectTrace(at, b.visTracer)
		}
	}
	b.gpu.L1SAddrTranslator = append(
		[]*addresstranslator.AddressTranslator{},
		b.l1sAddrTrans...)
}

func (b *R9NanoGPUBuilder) buildL1IAddrTranslators() {
	builder := addresstranslator.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumReqPerCycle(4).
		WithLog2PageSize(12).
		WithGPUID(b.gpu.GPUID).
		WithLowModuleFinder(b.LowModuleFinderForL1)
	for i := 0; i < b.numShaderArray; i++ {
		name :=
			fmt.Sprintf("%s.L1IAddrTrans_%d", b.gpuName, i)
		at := builder.
			WithTranslationProvider(b.L1ITLBs[i].TopPort).
			Build(name)

		conn := akita.NewDirectConnection(b.engine)
		b.l1ITol1AddrTranslatorConnections = append(
			b.l1ITol1AddrTranslatorConnections, conn)
		conn.PlugIn(at.TopPort)
		b.l1ToL2Connection.PlugIn(at.BottomPort)
		b.InternalConn.PlugIn(at.TranslationPort)
		b.InternalConn.PlugIn(at.CtrlPort)
		b.CP.AddressTranslators = append(b.CP.AddressTranslators, at.CtrlPort)

		b.l1iAddrTrans = append(b.l1iAddrTrans, at)

		if b.enableVisTracing {
			tracing.CollectTrace(at, b.visTracer)
		}
	}
	b.gpu.L1IAddrTranslator = append(
		[]*addresstranslator.AddressTranslator{},
		b.l1iAddrTrans...)
}

func (b *R9NanoGPUBuilder) buildTLBs() {
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumWays(64).
		WithNumSets(64).
		WithNumReqPerCycle(1024).
		WithLowModule(b.mmu.ToTop)
	l2TLB := builder.Build(fmt.Sprintf("%s.L2TLB", b.gpuName))
	b.L2TLBs = append(b.L2TLBs, l2TLB)
	b.gpu.L2TLBs = append(b.gpu.L2TLBs, l2TLB)
	b.l1TLBToL2TLBConnection = akita.NewDirectConnection(b.engine)
	b.l1TLBToL2TLBConnection.PlugIn(l2TLB.TopPort)
	b.InternalConn.PlugIn(l2TLB.ControlPort)
	b.externalConn.PlugIn(l2TLB.BottomPort)
	b.CP.TLBs = append(b.CP.TLBs, l2TLB.ControlPort)

	if b.enableVisTracing {
		tracing.CollectTrace(l2TLB, b.visTracer)
	}

	b.buildL1VTLBs()
	b.buildL1STLBs()
	b.buildL1ITLBs()
}

func (b *R9NanoGPUBuilder) buildL1VTLBs() {
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithLowModule(b.gpu.L2TLBs[0].TopPort).
		WithNumWays(64).
		WithNumSets(1).
		WithNumReqPerCycle(4)

	l1VTLBCount := b.numCU()
	for i := 0; i < l1VTLBCount; i++ {
		name := fmt.Sprintf("%s.L1VTLB_%d", b.gpuName, i)
		l1TLB := builder.Build(name)

		b.L1VTLBs = append(b.L1VTLBs, l1TLB)
		b.gpu.L1VTLBs = append(b.gpu.L1VTLBs, l1TLB)

		conn := akita.NewDirectConnection(b.engine)
		b.addrTranslatorToTLBL1Connections = append(
			b.addrTranslatorToTLBL1Connections, conn)

		conn.PlugIn(l1TLB.TopPort)
		b.l1TLBToL2TLBConnection.PlugIn(l1TLB.BottomPort)
		b.InternalConn.PlugIn(l1TLB.ControlPort)
		b.CP.TLBs = append(b.CP.TLBs, l1TLB.ControlPort)

		if b.enableVisTracing {
			tracing.CollectTrace(l1TLB, b.visTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildL1STLBs() {
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithLowModule(b.gpu.L2TLBs[0].TopPort).
		WithNumWays(64).
		WithNumSets(1)

	l1STLBCount := b.numShaderArray
	for i := 0; i < l1STLBCount; i++ {
		l1TLB := builder.
			Build(fmt.Sprintf("%s.L1STLB_%d", b.gpuName, i))

		b.L1STLBs = append(b.L1STLBs, l1TLB)
		b.gpu.L1STLBs = append(b.gpu.L1STLBs, l1TLB)

		conn := akita.NewDirectConnection(b.engine)
		b.addrTranslatorToSTLBL1Connections = append(
			b.addrTranslatorToSTLBL1Connections, conn)
		conn.PlugIn(l1TLB.TopPort)
		b.l1TLBToL2TLBConnection.PlugIn(l1TLB.BottomPort)
		b.InternalConn.PlugIn(l1TLB.ControlPort)
		b.CP.TLBs = append(b.CP.TLBs, l1TLB.ControlPort)

		if b.enableVisTracing {
			tracing.CollectTrace(l1TLB, b.visTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildL1ITLBs() {
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithLowModule(b.gpu.L2TLBs[0].TopPort).
		WithNumWays(64).
		WithNumSets(1)

	l1ITLBCount := b.numShaderArray
	for i := 0; i < l1ITLBCount; i++ {
		l1TLB := builder.
			Build(fmt.Sprintf("%s.L1ITLB_%d", b.gpuName, i))

		b.L1ITLBs = append(b.L1ITLBs, l1TLB)
		b.gpu.L1ITLBs = append(b.gpu.L1ITLBs, l1TLB)
		b.InternalConn.PlugIn(l1TLB.TopPort)
		b.l1TLBToL2TLBConnection.PlugIn(l1TLB.BottomPort)
		b.InternalConn.PlugIn(l1TLB.ControlPort)
		b.CP.TLBs = append(b.CP.TLBs, l1TLB.ControlPort)

		if b.enableVisTracing {
			tracing.CollectTrace(l1TLB, b.visTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildL1SCaches() {
	b.L1SCaches = make([]*l1v.Cache, 0, 16)
	builder := l1v.NewBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithBankLatency(0).
		WithNumBanks(4).
		WithLog2BlockSize(6).
		WithWayAssocitivity(4).
		WithNumMSHREntry(16).
		WithTotalByteSize(16 * mem.KB).
		WithLowModuleFinder(b.LowModuleFinderForL1)
	for i := 0; i < b.numShaderArray; i++ {
		name := fmt.Sprintf("%s.L1K_%02d", b.gpuName, i)
		sCache := builder.Build(name)

		topConn := akita.NewDirectConnection(b.engine)
		b.addrTranslatorToL1SConnections = append(
			b.addrTranslatorToL1SConnections, topConn)
		topConn.PlugIn(sCache.TopPort)

		b.InternalConn.PlugIn(sCache.ControlPort)
		b.l1ToL2Connection.PlugIn(sCache.BottomPort)
		b.L1SCaches = append(b.L1SCaches, sCache)
		b.CP.L1SCaches = append(b.CP.L1SCaches, sCache.ControlPort)
		b.gpu.L1SCaches = append(b.gpu.L1SCaches, sCache)
		if b.EnableMemTracing {
			tracing.CollectTrace(sCache, b.MemTracer)
		}
		if b.enableVisTracing {
			tracing.CollectTrace(sCache, b.visTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildL1ICaches() {
	b.L1ICaches = make([]*l1v.Cache, 0, 16)
	builder := l1v.NewBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithBankLatency(0).
		WithNumBanks(4).
		WithLog2BlockSize(6).
		WithWayAssocitivity(4).
		WithNumMSHREntry(16).
		WithTotalByteSize(32 * mem.KB)
	for i := 0; i < b.numShaderArray; i++ {
		name := fmt.Sprintf("%s.L1I_%02d", b.gpuName, i)
		iCache := builder.
			WithLowModuleFinder(&cache.SingleLowModuleFinder{
				LowModule: b.l1iAddrTrans[i].TopPort,
			}).
			Build(name)

		topConn := akita.NewDirectConnection(b.engine)
		b.cuToL1IConnections = append(b.cuToL1IConnections, topConn)
		topConn.PlugIn(iCache.TopPort)

		b.InternalConn.PlugIn(iCache.ControlPort)

		bottomConn := b.l1ITol1AddrTranslatorConnections[i]
		bottomConn.PlugIn(iCache.BottomPort)

		b.L1ICaches = append(b.L1ICaches, iCache)
		b.CP.L1ICaches = append(b.CP.L1ICaches, iCache.ControlPort)
		b.gpu.L1ICaches = append(b.gpu.L1ICaches, iCache)
		if b.EnableMemTracing {
			tracing.CollectTrace(iCache, b.MemTracer)
		}
		if b.enableVisTracing {
			tracing.CollectTrace(iCache, b.visTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildL1VCaches() {
	b.L1VCaches = make([]*l1v.Cache, 0, 64)
	builder := l1v.NewBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithBankLatency(0).
		WithNumBanks(4).
		WithLog2BlockSize(6).
		WithWayAssocitivity(4).
		WithNumMSHREntry(16).
		WithTotalByteSize(16 * mem.KB).
		WithLowModuleFinder(b.LowModuleFinderForL1)
	for i := 0; i < b.numCU(); i++ {
		name := fmt.Sprintf("%s.L1D_%02d", b.gpuName, i)
		dCache := builder.Build(name)

		conn := akita.NewDirectConnection(b.engine)
		b.addrTranslatorToL1VConnections = append(
			b.addrTranslatorToL1VConnections,
			conn)

		conn.PlugIn(dCache.TopPort)
		b.InternalConn.PlugIn(dCache.ControlPort)
		b.l1ToL2Connection.PlugIn(dCache.BottomPort)
		b.L1VCaches = append(b.L1VCaches, dCache)
		b.CP.L1VCaches = append(b.CP.L1VCaches, dCache.ControlPort)
		b.gpu.L1VCaches = append(b.gpu.L1VCaches, dCache)

		if b.EnableMemTracing {
			tracing.CollectTrace(dCache, b.MemTracer)
		}
		if b.enableVisTracing {
			tracing.CollectTrace(dCache, b.visTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildL2Caches() {
	b.L2Caches = make([]*writeback.Cache, 0, b.numMemoryBank)
	cacheBuilder := new(writeback.Builder)
	cacheBuilder.Engine = b.engine
	b.LowModuleFinderForL1 = cache.NewInterleavedLowModuleFinder(4096)
	b.LowModuleFinderForL1.UseAddressSpaceLimitation = true
	b.LowModuleFinderForL1.LowAddress = b.memAddrOffset
	b.LowModuleFinderForL1.HighAddress = b.memAddrOffset + 4*mem.GB
	b.l1ToL2Connection = akita.NewDirectConnection(b.engine)
	for i := 0; i < b.numMemoryBank; i++ {
		cacheBuilder.LowModuleFinder = b.LowModuleFinderForL2
		cacheBuilder.CacheName = fmt.Sprintf("%s.L2_%d", b.gpuName, i)
		cacheBuilder.WayAssociativity = 16
		cacheBuilder.BlockSize = 64
		cacheBuilder.ByteSize = 256 * mem.KB
		cacheBuilder.NumMSHREntry = 4096
		l2Cache := cacheBuilder.Build()
		b.L2Caches = append(b.L2Caches, l2Cache)
		b.CP.L2Caches = append(b.CP.L2Caches, l2Cache.ControlPort)
		b.gpu.L2Caches = append(b.gpu.L2Caches, l2Cache)

		b.LowModuleFinderForL1.LowModules = append(
			b.LowModuleFinderForL1.LowModules, l2Cache.TopPort)

		b.l1ToL2Connection.PlugIn(l2Cache.TopPort)

		bottomConn := b.l2ToDramConnections[i]
		bottomConn.PlugIn(l2Cache.BottomPort)
		b.InternalConn.PlugIn(l2Cache.ControlPort)

		if b.EnableMemTracing {
			tracing.CollectTrace(l2Cache, b.MemTracer)
		}
		if b.enableVisTracing {
			tracing.CollectTrace(l2Cache, b.visTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildMemControllers() {
	b.LowModuleFinderForL2 = cache.NewInterleavedLowModuleFinder(4096)
	b.LowModuleFinderForPMC = cache.NewInterleavedLowModuleFinder(4096)

	numDramController := b.numMemoryBank
	for i := 0; i < numDramController; i++ {
		memCtrl := idealmemcontroller.New(
			fmt.Sprintf("%s.DRAM_%d", b.gpuName, i),
			b.engine, 512*mem.MB)

		addrConverter := idealmemcontroller.InterleavingConverter{
			InterleavingSize:    4096,
			TotalNumOfElements:  numDramController,
			CurrentElementIndex: i,
			Offset:              b.memAddrOffset,
		}
		memCtrl.AddressConverter = addrConverter

		conn := akita.NewDirectConnection(b.engine)
		b.l2ToDramConnections = append(b.l2ToDramConnections, conn)
		conn.PlugIn(memCtrl.ToTop)

		b.LowModuleFinderForL2.LowModules = append(
			b.LowModuleFinderForL2.LowModules, memCtrl.ToTop)
		b.LowModuleFinderForPMC.LowModules = append(
			b.LowModuleFinderForPMC.LowModules, memCtrl.ToTop)

		b.gpu.MemoryControllers = append(
			b.gpu.MemoryControllers, memCtrl)
		b.CP.DRAMControllers = append(
			b.CP.DRAMControllers, memCtrl)

		if b.EnableMemTracing {
			tracing.CollectTrace(memCtrl, b.MemTracer)
		}
		if b.enableVisTracing {
			tracing.CollectTrace(memCtrl, b.visTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildCUs() {
	cuBuilder := timing.NewBuilder()
	cuBuilder.Engine = b.engine
	cuBuilder.Freq = b.freq
	cuBuilder.Decoder = insts.NewDisassembler()

	for i := 0; i < b.numCU(); i++ {
		cuBuilder.ConnToVectorMem = b.cuToL1VAddrTranslatorConnections[i]
		cuBuilder.ConnToInstMem = b.cuToL1IConnections[i/b.numCUPerShaderArray]
		cuBuilder.ConnToScalarMem =
			b.cuToL1SAddrTranslatorConnections[i/b.numCUPerShaderArray]
		cuBuilder.CUName = fmt.Sprintf("%s.CU%02d", b.gpuName, i)
		cuBuilder.InstMem = b.L1ICaches[i/b.numCUPerShaderArray].TopPort
		cuBuilder.ScalarMem = b.l1sAddrTrans[i/b.numCUPerShaderArray].TopPort

		lowModuleFinderForCU := &cache.SingleLowModuleFinder{
			LowModule: b.l1vAddrTrans[i].TopPort,
		}
		cuBuilder.VectorMemModules = lowModuleFinderForCU

		cu := cuBuilder.Build()
		b.gpu.CUs = append(b.gpu.CUs, cu)
		b.ACE.RegisterCU(cu.ToACE)

		b.InternalConn.PlugIn(cu.ToACE)
		b.InternalConn.PlugIn(cu.ToCP)
		b.InternalConn.PlugIn(cu.CP)

		if b.EnableISADebug && i == 0 {
			isaDebug, err := os.Create(fmt.Sprintf(
				"isa_timing_%s.debug", cu.Name()))
			if err != nil {
				log.Fatal(err)
			}
			isaDebugger := timing.NewISADebugger(log.New(isaDebug, "", 0))
			cu.AcceptHook(isaDebugger)
		}

		if b.enableVisTracing {
			tracing.CollectTrace(cu, b.visTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) numCU() int {
	return b.numCUPerShaderArray * b.numShaderArray
}
