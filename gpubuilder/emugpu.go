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
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	memtraces "gitlab.com/akita/mem/trace"
	"gitlab.com/akita/mem/vm"
)

// EmuGPUBuilder provide services to assemble usable GPUs
type EmuGPUBuilder struct {
	engine           akita.Engine
	freq             akita.Freq
	Driver           *driver.Driver
	GPUName          string
	MMU              vm.MMU
	GPUMemAddrOffset uint64

	EnableISADebug    bool
	EnableInstTracing bool
	EnableMemTracing  bool
}

// NewEmuGPUBuilder returns a new EmuGPUBuilder
func NewEmuGPUBuilder(engine akita.Engine) *EmuGPUBuilder {
	b := new(EmuGPUBuilder)
	b.engine = engine
	b.freq = 1 * akita.GHz
	b.GPUName = "GPU"

	b.EnableISADebug = false
	b.EnableInstTracing = false
	return b
}

// BuildEmulationGPU creates a very simple GPU for emulation purposes
func (b *EmuGPUBuilder) BuildEmulationGPU() (*gcn3.GPU, *mem.IdealMemController) {
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

	gpuMem := mem.NewIdealMemController(
		b.GPUName+".GlobalMem", b.engine, 4*mem.GB)
	gpuMem.Freq = 1 * akita.GHz
	gpuMem.Latency = 1
	addrConverter := mem.InterleavingConverter{
		InterleavingSize:    4 * mem.GB,
		TotalNumOfElements:  1,
		CurrentElementIndex: 0,
		Offset:              b.GPUMemAddrOffset,
	}
	gpuMem.AddressConverter = addrConverter
	if b.EnableMemTracing {
		gpuMem.AcceptHook(memTracer)
	}

	disassembler := insts.NewDisassembler()

	for i := 0; i < 4; i++ {
		computeUnit := emu.BuildComputeUnit(
			fmt.Sprintf("%s.CU%d", b.GPUName, i),
			b.engine, disassembler, b.MMU, gpuMem.Storage, &addrConverter)

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
	commandProcessor.DMAEngine = dmaEngine.ToCP

	connection.PlugIn(gpu.ToCommandProcessor)
	connection.PlugIn(commandProcessor.ToDriver)
	connection.PlugIn(commandProcessor.ToDispatcher)
	connection.PlugIn(b.Driver.ToGPUs)
	connection.PlugIn(dispatcher.ToCommandProcessor)
	connection.PlugIn(dispatcher.ToCUs)
	connection.PlugIn(gpuMem.ToTop)
	connection.PlugIn(dmaEngine.ToCP)
	connection.PlugIn(dmaEngine.ToMem)
	gpu.InternalConnection = connection

	return gpu, gpuMem
}
