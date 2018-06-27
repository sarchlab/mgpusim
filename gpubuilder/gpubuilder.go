package gpubuilder

import (
	"fmt"

	"gitlab.com/yaotsu/mem"

	"log"

	"os"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/connections"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
	"gitlab.com/yaotsu/gcn3/trace"
	"gitlab.com/yaotsu/mem/cache"
)

// GPUBuilder provide services to assemble usable GPUs
type GPUBuilder struct {
	engine  core.Engine
	freq    util.Freq
	Driver  core.Component
	GPUName string

	EnableISADebug    bool
	EnableInstTracing bool
}

// NewGPUBuilder returns a new GPUBuilder
func NewGPUBuilder(engine core.Engine) *GPUBuilder {
	b := new(GPUBuilder)
	b.engine = engine
	b.freq = 1 * util.GHz
	b.GPUName = "GPU"

	b.EnableISADebug = false
	b.EnableInstTracing = false
	return b
}

// BuildEmulationGPU creates a very simple GPU for emulation purposes
func (b *GPUBuilder) BuildEmulationGPU() (*gcn3.GPU, *mem.IdealMemController) {
	connection := connections.NewDirectConnection(b.engine)

	dispatcher := gcn3.NewDispatcher(b.GPUName+".Dispatcher", b.engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = b.freq

	commandProcessor := gcn3.NewCommandProcessor(b.GPUName + ".CommandProcessor")
	commandProcessor.Dispatcher = dispatcher

	gpuMem := mem.NewIdealMemController(b.GPUName+".GlobalMem", b.engine, 4*mem.GB)
	gpuMem.Freq = 1 * util.GHz
	gpuMem.Latency = 1

	disassembler := insts.NewDisassembler()

	for i := 0; i < 4; i++ {
		scratchpadPreparer := emu.NewScratchpadPreparerImpl()
		alu := emu.NewALUImpl(gpuMem.Storage)
		computeUnit := emu.NewComputeUnit(
			fmt.Sprintf("%s.CU%d", b.GPUName, i),
			b.engine, disassembler, scratchpadPreparer, alu)
		computeUnit.Freq = b.freq
		computeUnit.GlobalMemStorage = gpuMem.Storage
		core.PlugIn(computeUnit, "ToDispatcher", connection)
		dispatcher.RegisterCU(computeUnit)

		if b.EnableISADebug {
			isaDebug, err := os.Create(fmt.Sprintf("isa_%s.debug", computeUnit.Name()))
			if err != nil {
				log.Fatal(err.Error())
			}
			wfHook := emu.NewWfHook(log.New(isaDebug, "", 0))
			computeUnit.AcceptHook(wfHook)
		}
	}

	gpu := gcn3.NewGPU(b.GPUName)
	gpu.CommandProcessor = commandProcessor
	commandProcessor.Driver = gpu

	core.PlugIn(gpu, "ToCommandProcessor", connection)
	core.PlugIn(commandProcessor, "ToDriver", connection)
	core.PlugIn(commandProcessor, "ToDispatcher", connection)
	core.PlugIn(b.Driver, "ToGPUs", connection)
	core.PlugIn(dispatcher, "ToCommandProcessor", connection)
	core.PlugIn(dispatcher, "ToCUs", connection)
	core.PlugIn(gpuMem, "Top", connection)

	return gpu, gpuMem
}

func (b *GPUBuilder) BuildR9Nano() (*gcn3.GPU, *mem.IdealMemController) {
	b.freq = 1000 * util.MHz
	connection := connections.NewDirectConnection(b.engine)

	// Memory
	gpuMem := mem.NewIdealMemController("GlobalMem", b.engine, 4*mem.GB)
	gpuMem.Freq = b.freq
	gpuMem.Latency = 100

	// GPU
	gpu := gcn3.NewGPU(b.GPUName)
	commandProcessor := gcn3.NewCommandProcessor(b.GPUName + ".CommandProcessor")
	dispatcher := gcn3.NewDispatcher(b.GPUName+"Dispatcher", b.engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = b.freq

	gpu.CommandProcessor = commandProcessor
	commandProcessor.Dispatcher = dispatcher
	commandProcessor.Driver = gpu

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
	l2Caches := make([]*cache.WriteBackCache, 0, 6)

	lowModuleFinderForL2 := new(cache.SingleLowModuleFinder)
	lowModuleFinderForL2.LowModule = gpuMem
	cacheBuilder.LowModuleFinder = lowModuleFinderForL2
	lowModuleFinderForL1 := cache.NewInterleavedLowModuleFinder(4096)

	for i := 0; i < 4; i++ {
		l2Cache := cacheBuilder.BuildWriteBackCache(
			fmt.Sprintf("%s.L2_%d", b.GPUName, i), 16, 512*mem.MB)
		l2Caches = append(l2Caches, l2Cache)
		l2Cache.Latency = 20
		l2Cache.Freq = 1 * util.GHz
		lowModuleFinderForL1.LowModules = append(
			lowModuleFinderForL1.LowModules, l2Cache)
		core.PlugIn(l2Cache, "ToTop", connection)
		core.PlugIn(l2Cache, "ToBottom", connection)
	}

	cacheBuilder.LowModuleFinder = lowModuleFinderForL1
	for i := 0; i < 64; i++ {
		dCache := cacheBuilder.BuildWriteAroundCache(
			fmt.Sprintf("%s.L1D_%02d", b.GPUName, i), 4, 16*mem.KB)
		dCache.Latency = 1
		core.PlugIn(dCache, "ToTop", connection)
		core.PlugIn(dCache, "ToBottom", connection)
		dCaches = append(dCaches, dCache)
		commandProcessor.ToResetAfterKernel = append(
			commandProcessor.ToResetAfterKernel, dCache,
		)
	}

	for i := 0; i < 16; i++ {
		kCache := cacheBuilder.BuildWriteAroundCache(
			fmt.Sprintf("%s.L1K_%02d", b.GPUName, i), 4, 16*mem.KB)
		kCache.Latency = 1
		core.PlugIn(kCache, "ToTop", connection)
		core.PlugIn(kCache, "ToBottom", connection)
		kCaches = append(kCaches, kCache)
		commandProcessor.ToResetAfterKernel = append(
			commandProcessor.ToResetAfterKernel, kCache,
		)

		iCache := cacheBuilder.BuildWriteAroundCache(
			fmt.Sprintf("%s.L1I_%02d", b.GPUName, i), 4, 32*mem.KB)
		iCache.Latency = 0
		iCache.NumPort = 4
		core.PlugIn(iCache, "ToTop", connection)
		core.PlugIn(iCache, "ToBottom", connection)
		iCaches = append(iCaches, iCache)
		commandProcessor.ToResetAfterKernel = append(
			commandProcessor.ToResetAfterKernel, iCache,
		)
	}

	for i := 0; i < 64; i++ {
		cuBuilder.CUName = fmt.Sprintf("%s.CU%02d", b.GPUName, i)
		cuBuilder.InstMem = iCaches[i/4]
		cuBuilder.ScalarMem = kCaches[i/4]
		cuBuilder.VectorMem = dCaches[i]
		//cuBuilder.InstMem = gpuMem
		//cuBuilder.ScalarMem = gpuMem
		//cuBuilder.VectorMem = gpuMem
		cu := cuBuilder.Build()
		dispatcher.RegisterCU(cu)

		core.PlugIn(cu, "ToACE", connection)

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

	core.PlugIn(gpu, "ToCommandProcessor", connection)
	core.PlugIn(gpu, "ToDriver", connection)
	core.PlugIn(commandProcessor, "ToDriver", connection)
	core.PlugIn(commandProcessor, "ToDispatcher", connection)
	core.PlugIn(dispatcher, "ToCommandProcessor", connection)
	core.PlugIn(dispatcher, "ToCUs", connection)
	core.PlugIn(gpuMem, "Top", connection)

	return gpu, gpuMem
}
