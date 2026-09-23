// Package emusystem contains the configuration for emulation.
package emusystem

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem/emugpu"
)

// driverGPUPortBufSize is the buffer size of the driver's "GPU" port. The v4
// driver auto-created 40M-deep buffers; 4096 is plenty for the emulation
// platform's per-cycle traffic.
const driverGPUPortBufSize = 4096

// Builder builds a hardware platform for emulation.
type Builder struct {
	simulation   *simulation.Simulation
	numGPUs      int
	log2PageSize uint64
	debugISA     bool
	archType     arch.Type
}

// MakeBuilder creates a new Builder with default parameters.
func MakeBuilder() Builder {
	return Builder{
		numGPUs:      4,
		log2PageSize: 12,
	}
}

// WithSimulation sets the simulation to use.
func (b Builder) WithSimulation(s *simulation.Simulation) Builder {
	b.simulation = s
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

// WithArchitecture sets the GPU architecture for emulation.
func (b Builder) WithArchitecture(archType arch.Type) Builder {
	b.archType = archType
	return b
}

// Build builds the hardware platform and returns the driver. The driver, the
// GPUs, and all the connections register themselves with the simulation.
func (b Builder) Build() *driver.Driver {
	storage := mem.NewStorage(uint64(b.numGPUs+1) * 4 * mem.GB)
	pageTable := vm.NewPageTable(b.log2PageSize)

	gpuDriver := b.buildDriver(storage, pageTable)

	externalConn := directconnection.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(directconnection.Spec{Freq: 1 * timing.GHz}).
		Build("ExternalConn")
	externalConn.PlugIn(gpuDriver.GetPortByName(driver.GPUPortName))

	gpuBuilder := b.createGPUBuilder(pageTable, storage)

	for i := 0; i < b.numGPUs; i++ {
		gpu := gpuBuilder.Build(fmt.Sprintf("GPU[%d]", i+1))

		cpPort := gpu.CommandProcessorPort
		gpuDriver.RegisterGPU(cpPort.AsRemote(), driver.DeviceProperties{
			DRAMSize: 4 * mem.GB,
			CUCount:  64,
		})
		externalConn.PlugIn(cpPort)
	}

	return gpuDriver
}

func (b Builder) createGPUBuilder(
	pageTable vm.PageTable,
	storage *mem.Storage,
) emugpu.Builder {
	gpuBuilder := emugpu.MakeBuilder().
		WithSimulation(b.simulation).
		WithPageTable(pageTable).
		WithLog2PageSize(b.log2PageSize).
		WithStorage(storage).
		WithArchitecture(b.archType)

	if b.debugISA {
		gpuBuilder = gpuBuilder.WithISADebugging()
	}

	return gpuBuilder
}

func (b Builder) buildDriver(
	storage *mem.Storage,
	pageTable vm.PageTable,
) *driver.Driver {
	spec := driver.DefaultSpec()
	spec.Log2PageSize = b.log2PageSize
	spec.UseMagicMemoryCopy = true

	gpuDriver := driver.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		WithResources(driver.Resources{
			PageTable:     pageTable,
			GlobalStorage: storage,
		}).
		Build("Driver")

	gpuPort := modeling.MakePortBuilder().
		WithRegistrar(b.simulation).
		WithComponent(gpuDriver.Comp).
		WithSpec(modeling.PortSpec{BufSize: driverGPUPortBufSize}).
		Build(driver.GPUPortName)
	gpuDriver.AssignPort(driver.GPUPortName, gpuPort)

	return gpuDriver
}
