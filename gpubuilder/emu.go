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
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/idealmemcontroller"
	memtraces "gitlab.com/akita/mem/trace"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/util/tracing"
)

// EmuGPUBuilder provide services to assemble usable GPUs
type EmuGPUBuilder struct {
	engine      akita.Engine
	freq        akita.Freq
	driver      *driver.Driver
	gpuName     string
	iommu       mmu.MMU
	memOffset   uint64
	memCapacity uint64

	EnableISADebug    bool
	EnableInstTracing bool
	EnableMemTracing  bool
}

// MakeEmuGPUBuilder creates a new EmuGPUBuilder
func MakeEmuGPUBuilder() EmuGPUBuilder {
	b := EmuGPUBuilder{}
	b.freq = 1 * akita.GHz

	b.EnableISADebug = false
	b.EnableInstTracing = false
	return b
}

// WithEngine sets the engine that the emulator GPUs to use
func (b EmuGPUBuilder) WithEngine(e akita.Engine) EmuGPUBuilder {
	b.engine = e
	return b
}

// WithDriver sets the GPU driver that the GPUs connect to.
func (b EmuGPUBuilder) WithDriver(d *driver.Driver) EmuGPUBuilder {
	b.driver = d
	return b
}

// WithIOMMU sets the IOMMU unit that provides the address translation
func (b EmuGPUBuilder) WithIOMMU(mmu mmu.MMU) EmuGPUBuilder {
	b.iommu = mmu
	return b
}

// WithMemCapacity sets the capacity of the GPU memory
func (b EmuGPUBuilder) WithMemCapacity(c uint64) EmuGPUBuilder {
	b.memCapacity = c
	return b
}

// WithMemOffset sets the first byte address of the GPU memory
func (b EmuGPUBuilder) WithMemOffset(offset uint64) EmuGPUBuilder {
	b.memOffset = offset
	return b
}

// Build creates a very simple GPU for emulation purposes
func (b EmuGPUBuilder) Build(name string) *gcn3.GPU {
	b.gpuName = name
	connection := akita.NewDirectConnection(b.engine)

	dispatcher := gcn3.NewDispatcher(
		b.gpuName+".Dispatcher",
		b.engine,
		kernels.NewGridBuilder())
	dispatcher.Freq = b.freq

	commandProcessor := gcn3.NewCommandProcessor(
		b.gpuName+".CommandProcessor", b.engine)
	commandProcessor.Dispatcher = dispatcher.ToCommandProcessor

	gpuMem := idealmemcontroller.New(
		b.gpuName+".GlobalMem", b.engine, b.memCapacity)
	gpuMem.Freq = 1 * akita.GHz
	gpuMem.Latency = 1
	addrConverter := idealmemcontroller.InterleavingConverter{
		InterleavingSize:    b.memCapacity,
		TotalNumOfElements:  1,
		CurrentElementIndex: 0,
		Offset:              b.memOffset,
	}
	gpuMem.AddressConverter = addrConverter
	if b.EnableMemTracing {
		file, _ := os.Create("mem.trace")
		logger := log.New(file, "", 0)
		memTracer := memtraces.NewTracer(logger)
		tracing.CollectTrace(gpuMem, memTracer)
	}

	disassembler := insts.NewDisassembler()

	for i := 0; i < 4; i++ {
		computeUnit := emu.BuildComputeUnit(
			fmt.Sprintf("%s.CU%d", b.gpuName, i),
			b.engine, disassembler, b.iommu, gpuMem.Storage, &addrConverter)

		connection.PlugIn(computeUnit.ToDispatcher)
		dispatcher.RegisterCU(computeUnit.ToDispatcher)

		if b.EnableISADebug {
			isaDebug, err := os.Create(fmt.Sprintf("isa_%s.debug", computeUnit.Name()))
			if err != nil {
				log.Fatal(err.Error())
			}
			isaDebugger := emu.NewISADebugger(log.New(isaDebug, "", 0))
			computeUnit.AcceptHook(isaDebugger)
		}
	}

	gpu := gcn3.NewGPU(b.gpuName, b.engine)
	gpu.Driver = b.driver.ToGPUs
	gpu.CommandProcessor = commandProcessor.ToDriver
	commandProcessor.Driver = gpu.ToCommandProcessor

	localDataSource := new(cache.SingleLowModuleFinder)
	localDataSource.LowModule = gpuMem.ToTop
	dmaEngine := gcn3.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.gpuName), b.engine, localDataSource)
	commandProcessor.DMAEngine = dmaEngine.ToCP

	connection.PlugIn(gpu.ToCommandProcessor)
	connection.PlugIn(commandProcessor.ToDriver)
	connection.PlugIn(commandProcessor.ToDispatcher)
	connection.PlugIn(b.driver.ToGPUs)
	connection.PlugIn(dispatcher.ToCommandProcessor)
	connection.PlugIn(dispatcher.ToCUs)
	connection.PlugIn(gpuMem.ToTop)
	connection.PlugIn(dmaEngine.ToCP)
	connection.PlugIn(dmaEngine.ToMem)
	gpu.InternalConnection = connection

	return gpu
}
