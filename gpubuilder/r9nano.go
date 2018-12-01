package gpubuilder

import (
	"fmt"
	"log"
	"os"

	"gitlab.com/akita/gcn3/rdma"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/gcn3/timing"
	"gitlab.com/akita/gcn3/timing/caches"
	"gitlab.com/akita/gcn3/trace"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	memtraces "gitlab.com/akita/mem/trace"
	"gitlab.com/akita/mem/vm"
)

type R9NanoGPUBuilder struct {
	Engine           akita.Engine
	Freq             akita.Freq
	Driver           *driver.Driver
	GPUName          string
	GPUMemAddrOffset uint64
	MMU              *vm.MMUImpl
	ExternalConn     akita.Connection

	EnableISADebug    bool
	EnableInstTracing bool
	EnableMemTracing  bool

	GPU                  *gcn3.GPU
	InternalConn         *akita.DirectConnection
	CP                   *gcn3.CommandProcessor
	ACE                  *gcn3.Dispatcher
	L1VCaches            []*caches.L1VCache
	L1SCaches            []*caches.L1VCache
	L1ICaches            []*caches.L1VCache
	L2Caches             []*cache.WriteBackCache
	TLBs                 []*vm.TLB
	DRAMs                []*mem.IdealMemController
	LowModuleFinderForL1 *cache.InterleavedLowModuleFinder
	LowModuleFinderForL2 *cache.InterleavedLowModuleFinder
	DMAEngine            *gcn3.DMAEngine
	RDMAEngine           *rdma.Engine

	MemTracer *memtraces.Tracer
}

// BuildR9 creates a pre-configure GPU similar to the AMD R9 Nano GPU.
func (b *R9NanoGPUBuilder) Build() *gcn3.GPU {
	b.Freq = 1000 * akita.MHz
	b.InternalConn = akita.NewDirectConnection(b.Engine)

	b.GPU = gcn3.NewGPU(b.GPUName, b.Engine)

	b.buildCP()
	b.buildMemSystem()
	b.buildCUs()
	b.buildDMAEngine()
	b.buildRDMAEngine()

	b.InternalConn.PlugIn(b.GPU.ToCommandProcessor)
	b.InternalConn.PlugIn(b.DMAEngine.ToCP)
	b.InternalConn.PlugIn(b.DMAEngine.ToMem)
	b.ExternalConn.PlugIn(b.GPU.ToDriver)

	b.GPU.InternalConnection = b.InternalConn

	return b.GPU
}

func (b *R9NanoGPUBuilder) buildRDMAEngine() {
	b.RDMAEngine = rdma.NewEngine(
		fmt.Sprintf("%s.RDMA", b.GPUName),
		b.Engine,
		b.LowModuleFinderForL2,
		nil,
	)
	b.GPU.RDMAEngine = b.RDMAEngine

}

func (b *R9NanoGPUBuilder) buildDMAEngine() {
	b.DMAEngine = gcn3.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.GPUName),
		b.Engine,
		b.LowModuleFinderForL2)
	b.CP.DMAEngine = b.DMAEngine.ToCP

}

func (b *R9NanoGPUBuilder) buildCP() {
	b.CP = gcn3.NewCommandProcessor(b.GPUName+".CommandProcessor", b.Engine)
	b.CP.Driver = b.GPU.ToCommandProcessor
	b.GPU.CommandProcessor = b.CP.ToDriver

	b.ACE = gcn3.NewDispatcher(b.GPUName+"Dispatcher", b.Engine,
		new(kernels.GridBuilderImpl))
	b.ACE.Freq = b.Freq
	b.CP.Dispatcher = b.ACE.ToCommandProcessor

	b.InternalConn.PlugIn(b.CP.ToDriver)
	b.InternalConn.PlugIn(b.CP.ToDispatcher)
	b.InternalConn.PlugIn(b.ACE.ToCommandProcessor)
	b.InternalConn.PlugIn(b.ACE.ToCUs)
}

func (b *R9NanoGPUBuilder) buildMemSystem() {
	if b.EnableMemTracing {
		file, err := os.Create("mem.trace")
		if err != nil {
			panic(err)
		}
		b.MemTracer = memtraces.NewTracer(file)
	}

	b.buildMemControllers()
	b.buildTLBs()
	b.buildL2Caches()
	b.buildL1VCaches()
	b.buildL1SCaches()
	b.buildL1ICaches()
}

func (b *R9NanoGPUBuilder) buildTLBs() {

	l2TLB := vm.NewTLB(
		fmt.Sprintf("%s.TLB_L2_%d", b.GPUName, 1),
		b.Engine)
	l2TLB.LowModule = b.MMU.ToTop
	//traceFile, _ := os.Create("l2_tlb.trace")
	//tlbTracer := vm.NewTLBTracer(traceFile)
	//l2TLB.AcceptHook(tlbTracer)
	//b.TLBs = append(b.TLBs, l2TLB)
	b.GPU.L2TLB = l2TLB
	b.InternalConn.PlugIn(l2TLB.ToTop)
	b.ExternalConn.PlugIn(l2TLB.ToBottom)

	l1TLBCount := 64 + 16 + 16
	for i := 0; i < l1TLBCount; i++ {
		l1TLB := vm.NewTLB(
			fmt.Sprintf("%s.TLB_L1_%d", b.GPUName, 1),
			b.Engine)
		l1TLB.LowModule = b.GPU.L2TLB.ToTop

		b.TLBs = append(b.TLBs, l1TLB)
		b.GPU.L1TLBs = append(b.GPU.L1TLBs, l1TLB)
		b.InternalConn.PlugIn(l1TLB.ToTop)
		b.InternalConn.PlugIn(l1TLB.ToBottom)
	}
}

func (b *R9NanoGPUBuilder) buildL1SCaches() {
	b.L1SCaches = make([]*caches.L1VCache, 0, 16)
	for i := 0; i < 16; i++ {
		sCache := caches.BuildL1VCache(
			fmt.Sprintf("%s.L1K_%02d", b.GPUName, i),
			b.Engine, b.Freq,
			1,
			6, 4, 14,
			b.LowModuleFinderForL1,
			b.TLBs[i].ToTop)
		b.InternalConn.PlugIn(sCache.ToCU)
		b.InternalConn.PlugIn(sCache.ToCP)
		b.InternalConn.PlugIn(sCache.ToL2)
		b.InternalConn.PlugIn(sCache.ToTLB)
		b.L1SCaches = append(b.L1SCaches, sCache)
		b.CP.CachesToReset = append(b.CP.CachesToReset, sCache.ToCP)
		if b.EnableMemTracing {
			sCache.AcceptHook(b.MemTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildL1ICaches() {
	b.L1ICaches = make([]*caches.L1VCache, 0, 16)
	for i := 0; i < 16; i++ {
		iCache := caches.BuildL1VCache(
			fmt.Sprintf("%s.L1I_%02d", b.GPUName, i),
			b.Engine, b.Freq,
			1,
			6, 4, 15,
			b.LowModuleFinderForL1,
			b.TLBs[16+i].ToTop)
		b.InternalConn.PlugIn(iCache.ToCU)
		b.InternalConn.PlugIn(iCache.ToCP)
		b.InternalConn.PlugIn(iCache.ToL2)
		b.InternalConn.PlugIn(iCache.ToTLB)

		b.L1ICaches = append(b.L1ICaches, iCache)
		b.CP.CachesToReset = append(b.CP.CachesToReset, iCache.ToCP)
		if b.EnableMemTracing {
			iCache.AcceptHook(b.MemTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildL1VCaches() {
	b.L1VCaches = make([]*caches.L1VCache, 0, 64)
	cacheBuilder := new(cache.Builder)
	cacheBuilder.Engine = b.Engine
	cacheBuilder.LowModuleFinder = b.LowModuleFinderForL1
	for i := 0; i < 64; i++ {
		dCache := caches.BuildL1VCache(
			fmt.Sprintf("%s.L1D_%02d", b.GPUName, i),
			b.Engine, b.Freq,
			1,
			6, 4, 14,
			b.LowModuleFinderForL1,
			b.TLBs[32+i].ToTop)

		b.InternalConn.PlugIn(dCache.ToCU)
		b.InternalConn.PlugIn(dCache.ToCP)
		b.InternalConn.PlugIn(dCache.ToL2)
		b.InternalConn.PlugIn(dCache.ToTLB)
		b.L1VCaches = append(b.L1VCaches, dCache)

		b.CP.CachesToReset = append(b.CP.CachesToReset, dCache.ToCP)

		if b.EnableMemTracing {
			dCache.AcceptHook(b.MemTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildL2Caches() {
	b.L2Caches = make([]*cache.WriteBackCache, 0, 8)
	cacheBuilder := new(cache.Builder)
	cacheBuilder.Engine = b.Engine
	b.LowModuleFinderForL1 = cache.NewInterleavedLowModuleFinder(4096)
	for i := 0; i < 8; i++ {
		cacheBuilder.LowModuleFinder = b.LowModuleFinderForL2
		l2Cache := cacheBuilder.BuildWriteBackCache(
			fmt.Sprintf("%s.L2_%d", b.GPUName, i), 16, 256*mem.KB, 4096)
		b.L2Caches = append(b.L2Caches, l2Cache)
		b.CP.L2Caches = append(b.CP.L2Caches, l2Cache)
		l2Cache.DirectoryLatency = 0
		l2Cache.Latency = 70
		l2Cache.SetNumBanks(4096)
		l2Cache.Freq = 1 * akita.GHz

		b.LowModuleFinderForL1.LowModules = append(
			b.LowModuleFinderForL1.LowModules, l2Cache.ToTop)
		b.InternalConn.PlugIn(l2Cache.ToTop)
		b.InternalConn.PlugIn(l2Cache.ToBottom)

		if b.EnableMemTracing {
			l2Cache.AcceptHook(b.MemTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildMemControllers() {
	b.LowModuleFinderForL2 = cache.NewInterleavedLowModuleFinder(4096)
	numDramController := 8
	for i := 0; i < numDramController; i++ {
		memCtrl := mem.NewIdealMemController(
			fmt.Sprintf("%s.DRAM_%d", b.GPUName, i),
			b.Engine, 512*mem.MB)

		addrConverter := mem.InterleavingConverter{
			InterleavingSize:    4096,
			TotalNumOfElements:  numDramController,
			CurrentElementIndex: i,
			Offset:              b.GPUMemAddrOffset,
		}
		memCtrl.AddressConverter = addrConverter

		b.InternalConn.PlugIn(memCtrl.ToTop)

		b.LowModuleFinderForL2.LowModules = append(
			b.LowModuleFinderForL2.LowModules, memCtrl.ToTop)
		b.GPU.MemoryControllers = append(
			b.GPU.MemoryControllers, memCtrl)
		b.CP.DRAMControllers = append(
			b.CP.DRAMControllers, memCtrl)

		if b.EnableMemTracing {
			memCtrl.AcceptHook(b.MemTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildCUs() {
	cuBuilder := timing.NewBuilder()
	cuBuilder.Engine = b.Engine
	cuBuilder.Freq = b.Freq
	cuBuilder.Decoder = insts.NewDisassembler()
	cuBuilder.ConnToInstMem = b.InternalConn
	cuBuilder.ConnToScalarMem = b.InternalConn
	cuBuilder.ConnToVectorMem = b.InternalConn

	for i := 0; i < 64; i++ {
		cuBuilder.CUName = fmt.Sprintf("%s.CU%02d", b.GPUName, i)
		cuBuilder.InstMem = b.L1ICaches[i/4].ToCU
		cuBuilder.ScalarMem = b.L1SCaches[i/4].ToCU

		lowModuleFinderForCU := new(cache.SingleLowModuleFinder)
		lowModuleFinderForCU.LowModule = b.L1VCaches[i].ToCU
		cuBuilder.VectorMemModules = lowModuleFinderForCU

		cu := cuBuilder.Build()
		b.GPU.CUs = append(b.GPU.CUs, cu)
		b.ACE.RegisterCU(cu.ToACE)

		b.InternalConn.PlugIn(cu.ToACE)

		if b.EnableISADebug && i == 0 {
			isaDebug, err := os.Create(fmt.Sprintf("isa_%s.debug", cu.Name()))
			if err != nil {
				log.Fatal(err)
			}
			isaDebugger := timing.NewISADebugger(log.New(isaDebug, "", 0))
			cu.AcceptHook(isaDebugger)
		}

		if b.EnableInstTracing {
			isaTraceFile, err := os.Create(fmt.Sprintf("inst_%s.trace", cu.Name()))
			if err != nil {
				log.Fatal(err)
			}
			isaTracer := trace.NewInstTracer(isaTraceFile)
			cu.AcceptHook(isaTracer)
		}
	}
}
