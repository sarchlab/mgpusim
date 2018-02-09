package gpubuilder

import (
	"fmt"

	"gitlab.com/yaotsu/mem"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/connections"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

// GPUBuilder provide services to assemble usable GPUs
type GPUBuilder struct {
	engine  core.Engine
	freq    util.Freq
	Driver  core.Component
	GPUName string
}

// NewGPUBuilder returns a new GPUBuilder
func NewGPUBuilder(engine core.Engine) *GPUBuilder {
	b := new(GPUBuilder)
	b.engine = engine
	b.freq = 1 * util.GHz
	b.GPUName = "GPU"
	return b
}

// BuildEmulationGPU creates a very simple GPU for emulation purposes
func (b *GPUBuilder) BuildEmulationGPU() (*gcn3.GPU, *mem.IdealMemController) {
	connection := connections.NewDirectConnection(b.engine)

	dispatcher := gcn3.NewDispatcher(b.GPUName+"Dispatcher", b.engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = b.freq

	commandProcessor := gcn3.NewCommandProcessor(b.GPUName + "CommandProcessor")
	commandProcessor.Dispatcher = dispatcher

	gpuMem := mem.NewIdealMemController("GlobalMem", b.engine, 4*mem.GB)
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
	}

	gpu := gcn3.NewGPU("GPU")
	gpu.CommandProcessor = commandProcessor
	commandProcessor.Driver = gpu

	core.PlugIn(gpu, "ToCommandProcessor", connection)
	core.PlugIn(commandProcessor, "ToDriver", connection)
	core.PlugIn(commandProcessor, "ToDispatcher", connection)
	core.PlugIn(b.Driver, "ToGpu", connection)
	core.PlugIn(dispatcher, "ToCommandProcessor", connection)
	core.PlugIn(dispatcher, "ToCUs", connection)
	core.PlugIn(gpuMem, "Top", connection)

	return gpu, gpuMem
}
