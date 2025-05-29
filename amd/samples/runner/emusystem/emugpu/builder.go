// Package emugpu contains the configuration for the emulation of a GPU.
package emugpu

import (
	"fmt"
	"log"
	"os"

	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cp"
)

// Builder builds a GPU for emulation.
type Builder struct {
	simulation       *simulation.Simulation
	freq             sim.Freq
	log2PageSize     uint64
	enableISADebug   bool
	gpuName          string
	gpu              *sim.Domain
	engine           sim.Engine
	pageTable        vm.PageTable
	gpuMem           *idealmemcontroller.Comp
	computeUnits     []*emu.ComputeUnit
	commandProcessor *cp.CommandProcessor
	dmaEngine        *cp.DMAEngine
	driver           *driver.Driver
	storage          *mem.Storage
}

// MakeBuilder creates a new Builder with default parameters.
func MakeBuilder() Builder {
	b := Builder{}

	b.freq = 1 * sim.GHz
	b.log2PageSize = 12
	b.enableISADebug = false

	return b
}

// WithSimulation sets the simulation to use.
func (b Builder) WithSimulation(sim *simulation.Simulation) Builder {
	b.simulation = sim
	b.engine = sim.GetEngine()

	return b
}

// WithDriver sets the GPU driver that the GPUs connect to.
func (b Builder) WithDriver(d *driver.Driver) Builder {
	b.driver = d
	return b
}

// WithPageTable sets the page table that provides the address translation
func (b Builder) WithPageTable(pageTable vm.PageTable) Builder {
	b.pageTable = pageTable
	return b
}

// WithLog2PageSize sets the page size of the GPU, as a power of 2.
func (b Builder) WithLog2PageSize(n uint64) Builder {
	b.log2PageSize = n
	return b
}

// WithStorage sets the global memory storage that is shared by multiple GPUs
func (b Builder) WithStorage(s *mem.Storage) Builder {
	b.storage = s
	return b
}

// WithISADebugging enables the simulation to dump instruction execution
// information.
func (b Builder) WithISADebugging() Builder {
	b.enableISADebug = true
	return b
}

// Build builds the GPU.
func (b Builder) Build(name string) *sim.Domain {
	b.gpuName = name

	b.gpu = sim.NewDomain(name)

	b.buildMemory()
	b.buildComputeUnits()
	b.buildGPU()
	b.connectInternalComponents()
	b.populateExternalPorts()

	return b.gpu
}

func (b *Builder) buildComputeUnits() {
	disassembler := insts.NewDisassembler()

	for i := range 64 {
		computeUnit := emu.BuildComputeUnit(
			fmt.Sprintf("%s.CU%d", b.gpuName, i),
			b.engine, disassembler, b.pageTable,
			b.log2PageSize, b.gpuMem.Storage, nil)
		b.simulation.RegisterComponent(computeUnit)

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

func (b *Builder) buildMemory() {
	b.gpuMem = idealmemcontroller.
		MakeBuilder().
		WithStorage(b.storage).
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithLatency(1).
		Build(b.gpuName + ".GlobalMem")

	b.simulation.RegisterComponent(b.gpuMem)
}

func (b *Builder) buildGPU() {
	b.commandProcessor = cp.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		Build(b.gpuName + ".CommandProcessor")

	b.simulation.RegisterComponent(b.commandProcessor)

	b.commandProcessor.Driver = b.driver.GetPortByName("GPU")

	localDataSource := new(mem.SinglePortMapper)
	localDataSource.Port = b.gpuMem.GetPortByName("Top").AsRemote()
	b.dmaEngine = cp.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.gpuName), b.engine, localDataSource)
	b.commandProcessor.DMAEngine = b.dmaEngine.ToCP
}

func (b *Builder) connectInternalComponents() {
	connection := directconnection.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		Build(b.gpuName + ".IntraGPUConn")

	b.simulation.RegisterComponent(connection)

	connection.PlugIn(b.commandProcessor.ToDMA)
	connection.PlugIn(b.commandProcessor.ToCUs)
	connection.PlugIn(b.gpuMem.GetPortByName("Top"))
	connection.PlugIn(b.dmaEngine.ToCP)
	connection.PlugIn(b.dmaEngine.ToMem)

	for _, cu := range b.computeUnits {
		b.commandProcessor.RegisterCU(cu)
		connection.PlugIn(cu.ToDispatcher)
	}
}

func (b *Builder) populateExternalPorts() {
	b.gpu.AddPort("CommandProcessor", b.commandProcessor.ToDriver)
}
