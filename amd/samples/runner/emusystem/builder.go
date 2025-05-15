// Package emusystem contains the configuration for emulation.
package emusystem

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/emusystem/emugpu"
)

// Builder builds a hardware platform for emulation.
type Builder struct {
	simulation   *simulation.Simulation
	numGPUs      int
	log2PageSize uint64
	debugISA     bool

	storage    *mem.Storage
	pageTable  vm.PageTable
	driver     *driver.Driver
	connection *directconnection.Comp
}

// MakeBuilder creates a new Builder with default parameters.
func MakeBuilder() Builder {
	return Builder{
		numGPUs:      4,
		log2PageSize: 12,
	}
}

// WithSimulation sets the simulation to use.
func (b Builder) WithSimulation(sim *simulation.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithNumGPUs sets the number of GPUs to use.
func (b Builder) WithNumGPUs(n int) Builder {
	b.numGPUs = n
	return b
}

// WithLog2PageSize sets the page size as a power of 2.
func (b Builder) WithLog2PageSize(n uint64) Builder {
	b.log2PageSize = n
	return b
}

// WithDebugISA enables the ISA debugging feature, which dumps the wavefront
// states after each instruction.
func (b Builder) WithDebugISA() Builder {
	b.debugISA = true
	return b
}

// Build builds the hardware platform.
func (b Builder) Build() *sim.Domain {
	domain := &sim.Domain{}

	b.storage = mem.NewStorage(uint64(b.numGPUs+1) * 4 * mem.GB)
	b.pageTable = vm.NewPageTable(b.log2PageSize)
	b.driver = b.buildDriver(b.simulation.GetEngine(), b.pageTable, b.storage)

	b.connection = directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(1 * sim.GHz).
		Build("ExternalConn")
	b.simulation.RegisterComponent(b.connection)

	b.connection.PlugIn(b.driver.GetPortByName("GPU"))

	gpuBuilder := b.createGPUBuilder(
		b.simulation.GetEngine(),
		b.driver,
		b.pageTable,
		b.storage,
	)

	for i := 0; i < b.numGPUs; i++ {
		gpu := gpuBuilder.Build(fmt.Sprintf("GPU[%d]", i+1))

		cpPort := gpu.GetPortByName("CommandProcessor")
		b.driver.RegisterGPU(cpPort, driver.DeviceProperties{
			DRAMSize: 4 * mem.GB,
			CUCount:  64,
		})
		b.connection.PlugIn(cpPort)
	}

	return domain
}

func (b *Builder) createGPUBuilder(
	engine sim.Engine,
	gpuDriver *driver.Driver,
	pageTable vm.PageTable,
	storage *mem.Storage,
) emugpu.Builder {
	gpuBuilder := emugpu.MakeBuilder().
		WithSimulation(b.simulation).
		WithDriver(gpuDriver).
		WithPageTable(pageTable).
		WithLog2PageSize(b.log2PageSize).
		WithStorage(storage)

	if b.debugISA {
		gpuBuilder = gpuBuilder.WithISADebugging()
	}

	return gpuBuilder
}

func (b *Builder) buildDriver(
	engine sim.Engine,
	pageTable vm.PageTable,
	storage *mem.Storage,
) *driver.Driver {
	gpuDriverBuilder := driver.MakeBuilder().
		WithMagicMemoryCopyMiddleware()

	gpuDriver := gpuDriverBuilder.
		WithEngine(engine).
		WithPageTable(pageTable).
		WithLog2PageSize(b.log2PageSize).
		WithGlobalStorage(storage).
		Build("Driver")

	b.simulation.RegisterComponent(gpuDriver)

	return gpuDriver
}
