package gpubuilder

import (
	"fmt"
	"log"
	"os"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/gcn3/rdma"
	"gitlab.com/akita/gcn3/timing"
	"gitlab.com/akita/gcn3/timing/caches"
	"gitlab.com/akita/gcn3/trace"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	memtraces "gitlab.com/akita/mem/trace"
	"gitlab.com/akita/mem/vm"
)

// R9NanoGPUBuilder can build R9 Nano GPUs.
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
	EnableVisTracing  bool

	GPU                  *gcn3.GPU
	InternalConn         *akita.DirectConnection
	CP                   *gcn3.CommandProcessor
	ACE                  *gcn3.Dispatcher
	L1VCaches            []*caches.L1VCache
	L1SCaches            []*caches.L1VCache
	L1ICaches            []*caches.L1VCache
	L2Caches             []*cache.WriteBackCache
	L1VTLBs              []*vm.TLB
	L1STLBs              []*vm.TLB
	L1ITLBs              []*vm.TLB
	L2TLBs               []*vm.TLB
	DRAMs                []*mem.IdealMemController
	LowModuleFinderForL1 *cache.InterleavedLowModuleFinder
	LowModuleFinderForL2 *cache.InterleavedLowModuleFinder
	DMAEngine            *gcn3.DMAEngine
	RDMAEngine           *rdma.Engine

	Tracer *trace.Tracer

	MemTracer *memtraces.Tracer
}

// Build creates a pre-configure GPU similar to the AMD R9 Nano GPU.
func (b *R9NanoGPUBuilder) Build() *gcn3.GPU {
	b.reset()

	b.Freq = 1000 * akita.MHz

	b.InternalConn = akita.NewDirectConnection(b.Engine)
	b.GPU = gcn3.NewGPU(b.GPUName, b.Engine)

	b.buildCP()
	b.buildMemSystem()
	b.buildDMAEngine()
	b.buildRDMAEngine()
	b.buildCUs()

	b.InternalConn.PlugIn(b.GPU.ToCommandProcessor)
	b.InternalConn.PlugIn(b.DMAEngine.ToCP)
	b.InternalConn.PlugIn(b.DMAEngine.ToMem)
	b.InternalConn.PlugIn(b.MMU.ToCP)
	b.ExternalConn.PlugIn(b.GPU.ToDriver)

	b.GPU.InternalConnection = b.InternalConn

	b.connectCUToCP()
	b.connectVMToCP()

	if b.EnableVisTracing {
		gpuTracer := trace.NewGPUTracer(b.Tracer)
		b.GPU.AcceptHook(gpuTracer)
	}

	return b.GPU
}

func (b *R9NanoGPUBuilder) reset() {
	b.L1VCaches = nil
	b.L1SCaches = nil
	b.L1ICaches = nil
	b.L2Caches = nil
	b.L1VTLBs = nil
	b.L1STLBs = nil
	b.L1ITLBs = nil
	b.L2TLBs = nil
	b.DRAMs = nil
}

func (b *R9NanoGPUBuilder) buildRDMAEngine() {
	b.RDMAEngine = rdma.NewEngine(
		fmt.Sprintf("%s.RDMA", b.GPUName),
		b.Engine,
		b.LowModuleFinderForL2,
		nil,
	)
	b.GPU.RDMAEngine = b.RDMAEngine
	b.LowModuleFinderForL1.ModuleForOtherAddresses = b.RDMAEngine.ToInside
	b.InternalConn.PlugIn(b.RDMAEngine.ToInside)
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

	b.ACE = gcn3.NewDispatcher(b.GPUName+".Dispatcher", b.Engine,
		new(kernels.GridBuilderImpl))
	b.ACE.Freq = b.Freq
	b.CP.Dispatcher = b.ACE.ToCommandProcessor
	b.GPU.Dispatchers = append(b.GPU.Dispatchers, b.ACE)

	b.InternalConn.PlugIn(b.CP.ToDriver)
	b.InternalConn.PlugIn(b.CP.ToDispatcher)
	b.InternalConn.PlugIn(b.ACE.ToCommandProcessor)
	b.InternalConn.PlugIn(b.ACE.ToCUs)
	b.InternalConn.PlugIn(b.CP.ToCUs)
	b.InternalConn.PlugIn(b.CP.ToVMModules)

	if b.EnableVisTracing {
		dispatcherTracer := trace.NewDispatcherTracer(b.Tracer)
		b.ACE.AcceptHook(dispatcherTracer)
	}
}

func (b *R9NanoGPUBuilder) connectCUToCP() {
	for i := 0; i < 64; i++ {
		b.CP.CUs = append(b.CP.CUs, akita.NewLimitNumReqPort(b.CP, 1))
		b.InternalConn.PlugIn(b.CP.CUs[i])
		b.CP.CUs[i] = b.GPU.CUs[i].(*timing.ComputeUnit).ToCP
		b.CP.ToCUs = b.GPU.CUs[i].(*timing.ComputeUnit).CP
	}

}

func (b *R9NanoGPUBuilder) connectVMToCP() {
	l1VTLBCount := 64
	l1STLBCount := 16
	l1ITLBCount := 64
	l2TLBCount := 1
	mmuCount := 1

	totalVMUnits := l1VTLBCount + l1STLBCount + l1ITLBCount + mmuCount + l2TLBCount

	for i := 0; i < totalVMUnits; i++ {
		b.CP.VMModules = append(b.CP.VMModules, akita.NewLimitNumReqPort(b.CP, 1))
		b.InternalConn.PlugIn(b.CP.VMModules[i])
	}

	currentVMCount := 0

	for i := 0; i < l1VTLBCount; i++ {
		b.CP.VMModules[currentVMCount] = b.L1VTLBs[i].ToCP
		currentVMCount++
	}

	for i := 0; i < l1STLBCount; i++ {
		b.CP.VMModules[currentVMCount] = b.L1STLBs[i].ToCP
		currentVMCount++
	}

	for i := 0; i < l1ITLBCount; i++ {
		b.CP.VMModules[currentVMCount] = b.L1ITLBs[i].ToCP
		currentVMCount++
	}

	b.CP.VMModules[currentVMCount] = b.L2TLBs[0].ToCP
	currentVMCount++

	b.CP.VMModules[currentVMCount] = b.MMU.ToCP
	currentVMCount++

	if currentVMCount != totalVMUnits {
		log.Panicf(" You missed some VM units in initialization")
	}
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
		fmt.Sprintf("%s.L2TLB", b.GPUName),
		b.Engine)
	l2TLB.LowModule = b.MMU.ToTop
	//traceFile, _ := os.Create("l2_tlb.trace")
	//tlbTracer := vm.NewTLBTracer(traceFile)
	//l2TLB.AcceptHook(tlbTracer)
	l2TLB.NumSets = 64
	l2TLB.NumWays = 64
	l2TLB.Latency = 3
	l2TLB.Reset()
	b.L2TLBs = append(b.L2TLBs, l2TLB)
	b.GPU.L2TLBs = append(b.GPU.L2TLBs, l2TLB)
	b.InternalConn.PlugIn(l2TLB.ToTop)
	b.InternalConn.PlugIn(l2TLB.ToCP)
	b.ExternalConn.PlugIn(l2TLB.ToBottom)

	l1VTLBCount := 64
	for i := 0; i < l1VTLBCount; i++ {
		l1TLB := vm.NewTLB(
			fmt.Sprintf("%s.L1VTLB%d", b.GPUName, i),
			b.Engine)
		l1TLB.LowModule = b.GPU.L2TLBs[0].ToTop
		l1TLB.NumWays = 64
		l1TLB.NumSets = 1
		l1TLB.Latency = 1
		l1TLB.Reset()

		b.L1VTLBs = append(b.L1VTLBs, l1TLB)
		b.GPU.L1VTLBs = append(b.GPU.L1VTLBs, l1TLB)
		b.InternalConn.PlugIn(l1TLB.ToTop)
		b.InternalConn.PlugIn(l1TLB.ToBottom)
		b.InternalConn.PlugIn(l1TLB.ToCP)

	}

	l1STLBCount := 16
	for i := 0; i < l1STLBCount; i++ {
		l1TLB := vm.NewTLB(
			fmt.Sprintf("%s.L1STLB%d", b.GPUName, i),
			b.Engine)
		l1TLB.LowModule = b.GPU.L2TLBs[0].ToTop
		l1TLB.NumWays = 64
		l1TLB.NumSets = 1
		l1TLB.Latency = 1
		l1TLB.Reset()

		b.L1STLBs = append(b.L1STLBs, l1TLB)
		b.GPU.L1STLBs = append(b.GPU.L1STLBs, l1TLB)
		b.InternalConn.PlugIn(l1TLB.ToTop)
		b.InternalConn.PlugIn(l1TLB.ToBottom)
		b.InternalConn.PlugIn(l1TLB.ToCP)

	}

	l1ITLBCount := 64
	for i := 0; i < l1ITLBCount; i++ {
		l1TLB := vm.NewTLB(
			fmt.Sprintf("%s.L1ITLB%d", b.GPUName, i),
			b.Engine)
		l1TLB.LowModule = b.GPU.L2TLBs[0].ToTop
		l1TLB.NumWays = 64
		l1TLB.NumSets = 1
		l1TLB.Latency = 1
		l1TLB.Reset()

		b.L1ITLBs = append(b.L1ITLBs, l1TLB)
		b.GPU.L1ITLBs = append(b.GPU.L1ITLBs, l1TLB)
		b.InternalConn.PlugIn(l1TLB.ToTop)
		b.InternalConn.PlugIn(l1TLB.ToBottom)
		b.InternalConn.PlugIn(l1TLB.ToCP)

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
			b.L1STLBs[i].ToTop)
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
			b.L1ITLBs[i].ToTop)
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
			b.L1VTLBs[i].ToTop)

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
	b.LowModuleFinderForL1.UseAddressSpaceLimitation = true
	b.LowModuleFinderForL1.LowAddress = b.GPUMemAddrOffset
	b.LowModuleFinderForL1.HighAddress = b.GPUMemAddrOffset + 4*mem.GB
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

		b.InternalConn.PlugIn(cu.ToCP)
		b.InternalConn.PlugIn(cu.CP)

		if b.EnableISADebug && i == 0 {
			isaDebug, err := os.Create(fmt.Sprintf("isa_%s.debug", cu.Name()))
			if err != nil {
				log.Fatal(err)
			}
			isaDebugger := timing.NewISADebugger(log.New(isaDebug, "", 0))
			cu.AcceptHook(isaDebugger)
		}

		if b.EnableVisTracing {
			wgTracer := trace.NewWGTracer(b.Tracer)
			cu.AcceptHook(wgTracer)

			isaTracer := trace.NewInstTracer(b.Tracer)
			cu.AcceptHook(isaTracer)
		}

		//if b.EnableInstTracing {
		//	isaTracer := trace.NewInstTracer(b.Tracer)
		//	cu.AcceptHook(isaTracer)
		//}
	}
}
