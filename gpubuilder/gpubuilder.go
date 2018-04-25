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
)

// GPUBuilder provide services to assemble usable GPUs
type GPUBuilder struct {
	engine  core.Engine
	freq    util.Freq
	Driver  core.Component
	GPUName string

	EnableISADebug bool
}

// NewGPUBuilder returns a new GPUBuilder
func NewGPUBuilder(engine core.Engine) *GPUBuilder {
	b := new(GPUBuilder)
	b.engine = engine
	b.freq = 1 * util.GHz
	b.GPUName = "GPU"

	b.EnableISADebug = false
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
	gpuMem.Latency = 2

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
	cuBuilder.InstMem = gpuMem
	cuBuilder.ScalarMem = gpuMem
	cuBuilder.VectorMem = gpuMem
	cuBuilder.GlobalStorage = gpuMem.Storage
	cuBuilder.ConnToInstMem = connection
	cuBuilder.ConnToScalarMem = connection
	cuBuilder.ConnToVectorMem = connection

	for i := 0; i < 64; i++ {
		cuBuilder.CUName = fmt.Sprintf("%s.CU%02d", b.GPUName, i)
		cu := cuBuilder.Build()
		dispatcher.RegisterCU(cu)

		core.PlugIn(cu, "ToACE", connection)

		if b.EnableISADebug {
			isaDebug, err := os.Create(fmt.Sprintf("isa_%s.debug", cu.Name()))
			if err != nil {
				log.Fatal(err.Error())
			}
			isaDebugger := timing.NewISADebugger(log.New(isaDebug, "", 0))
			cu.AcceptHook(isaDebugger)
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
