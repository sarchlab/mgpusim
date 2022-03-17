package runner

import (
	"fmt"
	"log"
	"os"

	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/akita/v3/tracing"
	"gitlab.com/akita/mem/v3/idealmemcontroller"
	"gitlab.com/akita/mem/v3/mem"
	memtraces "gitlab.com/akita/mem/v3/trace"
	"gitlab.com/akita/mem/v3/vm"
	"gitlab.com/akita/mgpusim/v3/driver"
	"gitlab.com/akita/mgpusim/v3/emu"
	"gitlab.com/akita/mgpusim/v3/insts"
	"gitlab.com/akita/mgpusim/v3/timing/cp"
)

// EmuGPUBuilder provide services to assemble usable GPUs
type EmuGPUBuilder struct {
	engine           sim.Engine
	freq             sim.Freq
	driver           *driver.Driver
	pageTable        vm.PageTable
	log2PageSize     uint64
	memOffset        uint64
	memCapacity      uint64
	gpuName          string
	gpu              *sim.Domain
	storage          *mem.Storage
	commandProcessor *cp.CommandProcessor
	gpuMem           *idealmemcontroller.Comp
	dmaEngine        *cp.DMAEngine
	computeUnits     []*emu.ComputeUnit

	enableISADebug   bool
	enableMemTracing bool
}

// MakeEmuGPUBuilder creates a new EmuGPUBuilder
func MakeEmuGPUBuilder() EmuGPUBuilder {
	b := EmuGPUBuilder{}
	b.freq = 1 * sim.GHz
	b.log2PageSize = 12

	b.enableISADebug = false
	return b
}

// WithEngine sets the engine that the emulator GPUs to use
func (b EmuGPUBuilder) WithEngine(e sim.Engine) EmuGPUBuilder {
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

// WithStorage sets the global memory storage that is shared by multiple GPUs
func (b EmuGPUBuilder) WithStorage(s *mem.Storage) EmuGPUBuilder {
	b.storage = s
	return b
}

// WithISADebugging enables the simulation to dump instruction execution
// information.
func (b EmuGPUBuilder) WithISADebugging() EmuGPUBuilder {
	b.enableISADebug = true
	return b
}

// WithMemTracing enables the simulation to dump memory transaction information.
func (b EmuGPUBuilder) WithMemTracing() EmuGPUBuilder {
	b.enableMemTracing = true
	return b
}

// Build creates a very simple GPU for emulation purposes
func (b EmuGPUBuilder) Build(name string) *GPU {
	b.clear()
	b.gpuName = name
	b.buildMemory()
	b.buildComputeUnits()
	b.buildGPU()
	b.connectInternalComponents()
	b.populateExternalPorts()

	return &GPU{
		Domain:           b.gpu,
		CommandProcessor: b.commandProcessor,
	}
}

func (b *EmuGPUBuilder) populateExternalPorts() {
	b.gpu.AddPort("CommandProcessor", b.commandProcessor.ToDriver)
}

func (b *EmuGPUBuilder) clear() {
	b.commandProcessor = nil
	b.computeUnits = nil
	b.gpuMem = nil
	b.dmaEngine = nil
	b.gpu = nil
}

func (b *EmuGPUBuilder) buildComputeUnits() {
	disassembler := insts.NewDisassembler()

	for i := 0; i < 64; i++ {
		computeUnit := emu.BuildComputeUnit(
			fmt.Sprintf("%s.CU%d", b.gpuName, i),
			b.engine, disassembler, b.pageTable,
			b.log2PageSize, b.gpuMem.Storage, nil)

		b.computeUnits = append(b.computeUnits, computeUnit)

		if b.enableISADebug {
			isaDebug, err := os.Create(
				fmt.Sprintf("isa_%s.debug", computeUnit.Name()))
			if err != nil {
				log.Fatal(err.Error())
			}
			isaDebugger := emu.NewISADebugger(log.New(isaDebug, "", 0))
			computeUnit.AcceptHook(isaDebugger)
		}
	}
}

func (b *EmuGPUBuilder) buildMemory() {
	b.gpuMem = idealmemcontroller.New(
		b.gpuName+".GlobalMem", b.engine, b.memCapacity)
	b.gpuMem.Freq = 1 * sim.GHz
	b.gpuMem.Latency = 1
	b.gpuMem.Storage = b.storage

	if b.enableMemTracing {
		file, _ := os.Create("mem.trace")
		logger := log.New(file, "", 0)
		memTracer := memtraces.NewTracer(logger, b.engine)
		tracing.CollectTrace(b.gpuMem, memTracer)
	}
}

func (b *EmuGPUBuilder) buildGPU() {
	b.commandProcessor = cp.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(1 * sim.GHz).
		Build(b.gpuName + ".CommandProcessor")

	b.gpu = sim.NewDomain(b.gpuName)
	b.commandProcessor.Driver = b.driver.GetPortByName("GPU")

	localDataSource := new(mem.SingleLowModuleFinder)
	localDataSource.LowModule = b.gpuMem.GetPortByName("Top")
	b.dmaEngine = cp.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.gpuName), b.engine, localDataSource)
	b.commandProcessor.DMAEngine = b.dmaEngine.ToCP
}

func (b *EmuGPUBuilder) connectInternalComponents() {
	connection := sim.NewDirectConnection(
		"InterGPUConn", b.engine, 1*sim.GHz)

	connection.PlugIn(b.commandProcessor.ToDriver, 1)
	connection.PlugIn(b.commandProcessor.ToDMA, 1)
	connection.PlugIn(b.commandProcessor.ToCUs, 1)
	connection.PlugIn(b.driver.GetPortByName("GPU"), 1)
	connection.PlugIn(b.gpuMem.GetPortByName("Top"), 1)
	connection.PlugIn(b.dmaEngine.ToCP, 1)
	connection.PlugIn(b.dmaEngine.ToMem, 1)

	for _, cu := range b.computeUnits {
		b.commandProcessor.RegisterCU(cu)
		connection.PlugIn(cu.ToDispatcher, 4)
	}
}
