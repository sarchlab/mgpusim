package gpubuilder

import (
	"fmt"

	"log"

	"os"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/emu"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/gcn3/timing"
	"gitlab.com/akita/gcn3/timing/caches"
	"gitlab.com/akita/gcn3/trace"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	memtraces "gitlab.com/akita/mem/trace"
)

// GPUBuilder provide services to assemble usable GPUs
type GPUBuilder struct {
	engine  akita.Engine
	freq    akita.Freq
	Driver  *driver.Driver
	GPUName string

	EnableISADebug    bool
	EnableInstTracing bool
	EnableMemTracing  bool
}

// NewGPUBuilder returns a new GPUBuilder
func NewGPUBuilder(engine akita.Engine) *GPUBuilder {
	b := new(GPUBuilder)
	b.engine = engine
	b.freq = 1 * akita.GHz
	b.GPUName = "GPU"

	b.EnableISADebug = false
	b.EnableInstTracing = false
	return b
}

// BuildEmulationGPU creates a very simple GPU for emulation purposes
func (b *GPUBuilder) BuildEmulationGPU() (*gcn3.GPU, *mem.IdealMemController) {
	connection := akita.NewDirectConnection(b.engine)

	dispatcher := gcn3.NewDispatcher(b.GPUName+".Dispatcher", b.engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = b.freq

	commandProcessor := gcn3.NewCommandProcessor(
		b.GPUName+".CommandProcessor", b.engine)
	commandProcessor.Dispatcher = dispatcher.ToCommandProcessor

	var memTracer *memtraces.Tracer
	if b.EnableMemTracing {
		file, _ := os.Create("mem.trace")
		memTracer = memtraces.NewTracer(file)
	}

	gpuMem := mem.NewIdealMemController(b.GPUName+".GlobalMem", b.engine, 4*mem.GB)
	gpuMem.Freq = 1 * akita.GHz
	gpuMem.Latency = 1
	if b.EnableMemTracing {
		gpuMem.AcceptHook(memTracer)
	}

	disassembler := insts.NewDisassembler()

	for i := 0; i < 4; i++ {
		scratchpadPreparer := emu.NewScratchpadPreparerImpl()
		alu := emu.NewALUImpl(gpuMem.Storage)
		computeUnit := emu.NewComputeUnit(
			fmt.Sprintf("%s.CU%d", b.GPUName, i),
			b.engine, disassembler, scratchpadPreparer, alu)
		computeUnit.Freq = b.freq
		computeUnit.GlobalMemStorage = gpuMem.Storage
		connection.PlugIn(computeUnit.ToDispatcher)
		dispatcher.RegisterCU(computeUnit.ToDispatcher)

		if b.EnableISADebug && i == 0 {
			isaDebug, err := os.Create(fmt.Sprintf("isa_%s.debug", computeUnit.Name()))
			if err != nil {
				log.Fatal(err.Error())
			}
			wfHook := emu.NewWfHook(log.New(isaDebug, "", 0))
			computeUnit.AcceptHook(wfHook)
		}
	}

	gpu := gcn3.NewGPU(b.GPUName, b.engine)
	gpu.CommandProcessor = commandProcessor.ToDriver
	commandProcessor.Driver = gpu.ToCommandProcessor

	localDataSource := new(cache.SingleLowModuleFinder)
	localDataSource.LowModule = gpuMem.ToTop
	dmaEngine := gcn3.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.GPUName), b.engine, localDataSource)
	commandProcessor.DMAEngine = dmaEngine.ToCommandProcessor

	connection.PlugIn(gpu.ToCommandProcessor)
	connection.PlugIn(commandProcessor.ToDriver)
	connection.PlugIn(commandProcessor.ToDispatcher)
	connection.PlugIn(b.Driver.ToGPUs)
	connection.PlugIn(dispatcher.ToCommandProcessor)
	connection.PlugIn(dispatcher.ToCUs)
	connection.PlugIn(gpuMem.ToTop)
	connection.PlugIn(dmaEngine.ToCommandProcessor)
	connection.PlugIn(dmaEngine.ToMem)

	return gpu, gpuMem
}

func (b *GPUBuilder) BuildR9Nano() (*gcn3.GPU, *mem.IdealMemController) {
	b.freq = 1000 * akita.MHz
	connection := akita.NewDirectConnection(b.engine)

	var memTracer *memtraces.Tracer
	if b.EnableMemTracing {
		file, _ := os.Create("mem.trace")
		memTracer = memtraces.NewTracer(file)
	}

	// Memory
	gpuMem := mem.NewIdealMemController("GlobalMem", b.engine, 4*mem.GB)
	gpuMem.Freq = b.freq
	gpuMem.Latency = 225
	if b.EnableMemTracing {
		gpuMem.AcceptHook(memTracer)
	}

	// GPU
	gpu := gcn3.NewGPU(b.GPUName, b.engine)
	cp := gcn3.NewCommandProcessor(b.GPUName+".CommandProcessor", b.engine)
	cp.GPUStorage = gpuMem.Storage
	dispatcher := gcn3.NewDispatcher(b.GPUName+"Dispatcher", b.engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = b.freq

	gpu.CommandProcessor = cp.ToDriver
	cp.Dispatcher = dispatcher.ToCommandProcessor
	cp.Driver = gpu.ToCommandProcessor

	cuBuilder := timing.NewBuilder()
	cuBuilder.Engine = b.engine
	cuBuilder.Freq = b.freq
	cuBuilder.Decoder = insts.NewDisassembler()
	cuBuilder.ConnToInstMem = connection
	cuBuilder.ConnToScalarMem = connection
	cuBuilder.ConnToVectorMem = connection

	cacheBuilder := new(cache.Builder)
	cacheBuilder.Engine = b.engine
	dCaches := make([]*caches.L1VCache, 0, 64)
	kCaches := make([]*caches.L1VCache, 0, 16)
	iCaches := make([]*caches.L1VCache, 0, 16)
	l2Caches := make([]*cache.WriteBackCache, 0, 8)

	lowModuleFinderForL2 := new(cache.SingleLowModuleFinder)
	lowModuleFinderForL2.LowModule = gpuMem.ToTop
	cacheBuilder.LowModuleFinder = lowModuleFinderForL2
	lowModuleFinderForL1 := cache.NewInterleavedLowModuleFinder(4096)
	//lowModuleFinderForL1 := new(cache.SingleLowModuleFinder)
	//lowModuleFinderForL1.LowModule = gpuMem.ToTop

	for i := 0; i < 8; i++ {
		l2Cache := cacheBuilder.BuildWriteBackCache(
			fmt.Sprintf("%s.L2_%d", b.GPUName, i), 16, 256*mem.KB, 4096)
		l2Caches = append(l2Caches, l2Cache)
		cp.L2Caches = append(cp.L2Caches, l2Cache)
		l2Cache.DirectoryLatency = 0
		l2Cache.Latency = 70
		l2Cache.SetNumBanks(4096)
		l2Cache.Freq = 1 * akita.GHz
		lowModuleFinderForL1.LowModules = append(
			lowModuleFinderForL1.LowModules, l2Cache.ToTop)
		connection.PlugIn(l2Cache.ToTop)
		connection.PlugIn(l2Cache.ToBottom)
		if b.EnableMemTracing {
			l2Cache.AcceptHook(memTracer)
		}
	}

	cacheBuilder.LowModuleFinder = lowModuleFinderForL1
	for i := 0; i < 64; i++ {
		dCache := caches.BuildL1VCache(
			fmt.Sprintf("%s.L1D_%02d", b.GPUName, i),
			b.engine, b.freq,
			1,
			6, 4, 14,
			lowModuleFinderForL1)

		connection.PlugIn(dCache.ToCU)
		connection.PlugIn(dCache.ToCP)
		connection.PlugIn(dCache.ToL2)
		dCaches = append(dCaches, dCache)

		cp.CachesToReset = append(
			cp.CachesToReset, dCache.ToCP)

		if b.EnableMemTracing {
			dCache.AcceptHook(memTracer)
		}
	}

	for i := 0; i < 16; i++ {
		kCache := caches.BuildL1VCache(
			fmt.Sprintf("%s.L1K_%02d", b.GPUName, i),
			b.engine, b.freq,
			1,
			6, 4, 14,
			lowModuleFinderForL1)
		connection.PlugIn(kCache.ToCU)
		connection.PlugIn(kCache.ToCP)
		connection.PlugIn(kCache.ToL2)
		kCaches = append(kCaches, kCache)
		cp.CachesToReset = append(cp.CachesToReset, kCache.ToCP)
		if b.EnableMemTracing {
			kCache.AcceptHook(memTracer)
		}

		iCache := caches.BuildL1VCache(
			fmt.Sprintf("%s.L1I_%02d", b.GPUName, i),
			b.engine, b.freq,
			1,
			6, 4, 15,
			lowModuleFinderForL1)
		connection.PlugIn(iCache.ToCU)
		connection.PlugIn(iCache.ToCP)
		connection.PlugIn(iCache.ToL2)
		iCaches = append(iCaches, iCache)
		cp.CachesToReset = append(cp.CachesToReset, iCache.ToCP)
		if b.EnableMemTracing {
			iCache.AcceptHook(memTracer)
		}
	}

	for i := 0; i < 64; i++ {
		cuBuilder.CUName = fmt.Sprintf("%s.CU%02d", b.GPUName, i)
		cuBuilder.InstMem = iCaches[i/4].ToCU
		cuBuilder.ScalarMem = kCaches[i/4].ToCU
		lowModuleFinderForCU := new(cache.SingleLowModuleFinder)
		lowModuleFinderForCU.LowModule = dCaches[i].ToCU
		cuBuilder.VectorMemModules = lowModuleFinderForCU
		//cuBuilder.InstMem = gpuMem
		//cuBuilder.ScalarMem = gpuMem
		//cuBuilder.VectorMem = gpuMem
		cu := cuBuilder.Build()
		gpu.CUs = append(gpu.CUs, cu)
		dispatcher.RegisterCU(cu.ToACE)

		connection.PlugIn(cu.ToACE)

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

	dmaEngine := gcn3.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.GPUName), b.engine, lowModuleFinderForL2)
	cp.DMAEngine = dmaEngine.ToCommandProcessor

	connection.PlugIn(gpu.ToCommandProcessor)
	connection.PlugIn(gpu.ToDriver)
	connection.PlugIn(cp.ToDriver)
	connection.PlugIn(cp.ToDispatcher)
	connection.PlugIn(dispatcher.ToCommandProcessor)
	connection.PlugIn(dispatcher.ToCUs)
	connection.PlugIn(gpuMem.ToTop)
	connection.PlugIn(dmaEngine.ToCommandProcessor)
	connection.PlugIn(dmaEngine.ToMem)

	gpu.L2CacheFinder = lowModuleFinderForL1

	return gpu, gpuMem
}
