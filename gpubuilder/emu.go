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
	"gitlab.com/akita/mem/idealmemcontroller"
	memtraces "gitlab.com/akita/mem/trace"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/util/tracing"
)

// EmuGPUBuilder provide services to assemble usable GPUs
type EmuGPUBuilder struct {
	engine       akita.Engine
	freq         akita.Freq
	driver       *driver.Driver
	pageTable    vm.PageTable
	log2PageSize uint64
	memOffset    uint64
	memCapacity  uint64
	storage      *mem.Storage

	EnableISADebug    bool
	EnableInstTracing bool
	EnableMemTracing  bool
}

// MakeEmuGPUBuilder creates a new EmuGPUBuilder
func MakeEmuGPUBuilder() EmuGPUBuilder {
	b := EmuGPUBuilder{}
	b.freq = 1 * akita.GHz
	b.log2PageSize = 12

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

// WithPageTable sets the page table that provides the address translation
func (b EmuGPUBuilder) WithPageTable(pageTable vm.PageTable) EmuGPUBuilder {
	b.pageTable = pageTable
	return b
}

// WithLog2PageSize sets the page size of the GPU, as a power of 2.
func (b EmuGPUBuilder) WithLog2PageSize(n uint64) EmuGPUBuilder {
	b.log2PageSize = n
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

// Storage sets the global memory storage that is shared by multiple GPUs
func (b EmuGPUBuilder) WithStorage(s *mem.Storage) EmuGPUBuilder {
	b.storage = s
	return b
}

//nolint:gocyclo,funlen
// Build creates a very simple GPU for emulation purposes
func (b EmuGPUBuilder) Build(name string) *gcn3.GPU {
	connection := akita.NewDirectConnection(
		"InterGPUConn", b.engine, 1*akita.GHz)

	dispatcher := gcn3.NewDispatcher(
		name+".Dispatcher",
		b.engine,
		kernels.NewGridBuilder())
	dispatcher.Freq = b.freq

	commandProcessor := gcn3.NewCommandProcessor(
		name+".CommandProcessor", b.engine)
	commandProcessor.Dispatcher = dispatcher.ToCommandProcessor

	gpuMem := idealmemcontroller.New(
		name+".GlobalMem", b.engine, b.memCapacity)
	gpuMem.Freq = 1 * akita.GHz
	gpuMem.Latency = 1
	// addrConverter := idealmemcontroller.InterleavingConverter{
	// 	InterleavingSize:    b.memCapacity,
	// 	TotalNumOfElements:  1,
	// 	CurrentElementIndex: 0,
	// 	Offset:              b.memOffset,
	// }
	// gpuMem.AddressConverter = addrConverter
	gpuMem.Storage = b.storage
	if b.EnableMemTracing {
		file, _ := os.Create("mem.trace")
		logger := log.New(file, "", 0)
		memTracer := memtraces.NewTracer(logger)
		tracing.CollectTrace(gpuMem, memTracer)
	}

	disassembler := insts.NewDisassembler()

	for i := 0; i < 4; i++ {
		computeUnit := emu.BuildComputeUnit(
			fmt.Sprintf("%s.CU%d", name, i),
			b.engine, disassembler, b.pageTable,
			b.log2PageSize, gpuMem.Storage, nil)

		connection.PlugIn(computeUnit.ToDispatcher, 4)
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

	gpu := gcn3.NewGPU(name)
	gpu.CommandProcessor = commandProcessor
	commandProcessor.Driver = b.driver.ToGPUs
	gpu.Storage = b.storage

	localDataSource := new(cache.SingleLowModuleFinder)
	localDataSource.LowModule = gpuMem.ToTop
	dmaEngine := gcn3.NewDMAEngine(
		fmt.Sprintf("%s.DMA", name), b.engine, localDataSource)
	commandProcessor.DMAEngine = dmaEngine.ToCP

	connection.PlugIn(commandProcessor.ToDriver, 1)
	connection.PlugIn(commandProcessor.ToDispatcher, 1)
	connection.PlugIn(b.driver.ToGPUs, 1)
	connection.PlugIn(dispatcher.ToCommandProcessor, 1)
	connection.PlugIn(dispatcher.ToCUs, 1)
	connection.PlugIn(gpuMem.ToTop, 1)
	connection.PlugIn(dmaEngine.ToCP, 1)
	connection.PlugIn(dmaEngine.ToMem, 1)
	gpu.InternalConnection = connection

	return gpu
}
