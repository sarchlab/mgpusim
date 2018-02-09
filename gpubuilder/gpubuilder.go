package gpubuilder

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
)

type GPUBuilder struct {
	engine core.Engine
	freq   util.Freq
}

// NewGPUBuilder returns a new GPUBuilder
func NewGPUBuilder(engine core.Engine) *GPUBuilder {
	b := new(GPUBuilder)
	b.engine = engine
	b.freq = 1 * util.GHz
	return b
}

// BuildEmulationGPU creates a very simple GPU for emulation purposes
func (b *GPUBuilder) BuildEmulationGPU() *gcn3.GPU {
	dispatcher := gcn3.NewDispatcher("GPU.Dispatcher", b.engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = b.freq

	commandProcessor := gcn3.NewCommandProcessor("GPU.CommandProcessor")
	commandProcessor.Dispatcher = dispatcher

	disassembler = insts.NewDisassembler()

	for i := 0; i < 4; i++ {
		scratchpadPreparer := emu.NewScratchpadPreparerImpl()
		alu := emu.NewALUImpl(globalMem.Storage)
	}

	gpu := gcn3.NewGPU("GPU")
	gpu.CommandProcessor = commandProcessor
	return gpu
}
