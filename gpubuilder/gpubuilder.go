package gpubuilder

import (
	"fmt"

	"gitlab.com/yaotsu/mem"

	"log"

	"os"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
	"gitlab.com/yaotsu/gcn3/trace"
	"gitlab.com/yaotsu/mem/cache"
	memtraces "gitlab.com/yaotsu/mem/trace"
)

// GPUBuilder provide services to assemble usable GPUs
type GPUBuilder struct {
	engine  core.Engine
	freq    core.Freq
	Driver  *driver.Driver
	GPUName string

	EnableISADebug    bool
	EnableInstTracing bool
	EnableMemTracing  bool
}

// NewGPUBuilder returns a new GPUBuilder
func NewGPUBuilder(engine core.Engine) *GPUBuilder {
	b := new(GPUBuilder)
	b.engine = engine
	b.freq = 1 * core.GHz
	b.GPUName = "GPU"

	b.EnableISADebug = false
	b.EnableInstTracing = false
	return b
}

// BuildEmulationGPU creates a very simple GPU for emulation purposes
func (b *GPUBuilder) BuildEmulationGPU() (*gcn3.GPU, *mem.IdealMemController) {
	connection := core.NewDirectConnection(b.engine)

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
	gpuMem.Freq = 1 * core.GHz
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

		if b.EnableISADebug {
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
	b.freq = 1000 * core.MHz
	connection := core.NewDirectConnection(b.engine)

	var memTracer *memtraces.Tracer
	if b.EnableMemTracing {
		file, _ := os.Create("mem.trace")
		memTracer = memtraces.NewTracer(file)
	}

	// Memory
	gpuMem := mem.NewIdealMemController("GlobalMem", b.engine, 4*mem.GB)
	gpuMem.Freq = b.freq
	gpuMem.Latency = 310
	if b.EnableMemTracing {
		gpuMem.AcceptHook(memTracer)
	}

	// GPU
	gpu := gcn3.NewGPU(b.GPUName, b.engine)
	commandProcessor := gcn3.NewCommandProcessor(b.GPUName+".CommandProcessor", b.engine)
	commandProcessor.GPUStorage = gpuMem.Storage
	dispatcher := gcn3.NewDispatcher(b.GPUName+"Dispatcher", b.engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = b.freq

	gpu.CommandProcessor = commandProcessor.ToDriver
	commandProcessor.Dispatcher = dispatcher.ToCommandProcessor
	commandProcessor.Driver = gpu.ToCommandProcessor

	cuBuilder := timing.NewBuilder()
	cuBuilder.Engine = b.engine
	cuBuilder.Freq = b.freq
	cuBuilder.Decoder = insts.NewDisassembler()
	cuBuilder.ConnToInstMem = connection
	cuBuilder.ConnToScalarMem = connection
	cuBuilder.ConnToVectorMem = connection

	cacheBuilder := new(cache.Builder)
	cacheBuilder.Engine = b.engine
	dCaches := make([]*cache.WriteAroundCache, 0, 64)
	kCaches := make([]*cache.WriteAroundCache, 0, 16)
	iCaches := make([]*cache.WriteAroundCache, 0, 16)
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
		commandProcessor.L2Caches = append(commandProcessor.L2Caches, l2Cache)
		l2Cache.DirectoryLatency = 0
		l2Cache.Latency = 110
		l2Cache.SetNumBanks(4096)
		l2Cache.Freq = 1 * core.GHz
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
		dCache := cacheBuilder.BuildWriteAroundCache(
			fmt.Sprintf("%s.L1D_%02d", b.GPUName, i), 4, 16*mem.KB, 128)
		dCache.DirectoryLatency = 0
		dCache.Latency = 10
		dCache.SetNumBanks(1)
		connection.PlugIn(dCache.ToTop)
		connection.PlugIn(dCache.ToBottom)
		dCaches = append(dCaches, dCache)
		commandProcessor.ToResetAfterKernel = append(
			commandProcessor.ToResetAfterKernel, dCache,
		)
		if b.EnableMemTracing {
			dCache.AcceptHook(memTracer)
		}
	}

	for i := 0; i < 16; i++ {
		kCache := cacheBuilder.BuildWriteAroundCache(
			fmt.Sprintf("%s.L1K_%02d", b.GPUName, i), 4, 16*mem.KB, 16)
		kCache.DirectoryLatency = 0
		kCache.Latency = 1
		kCache.SetNumBanks(1)
		connection.PlugIn(kCache.ToTop)
		connection.PlugIn(kCache.ToBottom)
		kCaches = append(kCaches, kCache)
		commandProcessor.ToResetAfterKernel = append(
			commandProcessor.ToResetAfterKernel, kCache,
		)
		if b.EnableMemTracing {
			kCache.AcceptHook(memTracer)
		}

		iCache := cacheBuilder.BuildWriteAroundCache(
			fmt.Sprintf("%s.L1I_%02d", b.GPUName, i), 4, 32*mem.KB, 16)
		iCache.DirectoryLatency = 0
		iCache.Latency = 0
		iCache.SetNumBanks(4)
		connection.PlugIn(iCache.ToTop)
		connection.PlugIn(iCache.ToBottom)
		iCaches = append(iCaches, iCache)
		commandProcessor.ToResetAfterKernel = append(
			commandProcessor.ToResetAfterKernel, iCache,
		)
		if b.EnableMemTracing {
			iCache.AcceptHook(memTracer)
		}
	}

	for i := 0; i < 64; i++ {
		cuBuilder.CUName = fmt.Sprintf("%s.CU%02d", b.GPUName, i)
		cuBuilder.InstMem = iCaches[i/4].ToTop
		cuBuilder.ScalarMem = kCaches[i/4].ToTop
		//lowModuleFinderForCU := new(cache.SingleLowModuleFinder)
		//lowModuleFinderForCU.LowModule = dCaches[i].ToTop
		cuBuilder.VectorMemModules = lowModuleFinderForL1
		//cuBuilder.InstMem = gpuMem
		//cuBuilder.ScalarMem = gpuMem
		//cuBuilder.VectorMem = gpuMem
		cu := cuBuilder.Build()
		dispatcher.RegisterCU(cu.ToACE)

		connection.PlugIn(cu.ToACE)

		if b.EnableISADebug {
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
	commandProcessor.DMAEngine = dmaEngine.ToCommandProcessor

	connection.PlugIn(gpu.ToCommandProcessor)
	connection.PlugIn(gpu.ToDriver)
	connection.PlugIn(commandProcessor.ToDriver)
	connection.PlugIn(commandProcessor.ToDispatcher)
	connection.PlugIn(dispatcher.ToCommandProcessor)
	connection.PlugIn(dispatcher.ToCUs)
	connection.PlugIn(gpuMem.ToTop)
	connection.PlugIn(dmaEngine.ToCommandProcessor)
	connection.PlugIn(dmaEngine.ToMem)

	return gpu, gpuMem
}
