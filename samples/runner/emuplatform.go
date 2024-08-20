package runner

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/mgpusim/v4/driver"
)

// EmuBuilder can build a platform for emulation purposes.
type EmuBuilder struct {
	useParallelEngine  bool
	debugISA           bool
	traceVis           bool
	traceMem           bool
	numGPU             int
	log2PageSize       uint64
	useMagicMemoryCopy bool
	gpus               []*GPU
}

// MakeEmuBuilder creates a EmuBuilder with default parameters.
func MakeEmuBuilder() EmuBuilder {
	b := EmuBuilder{
		numGPU:       4,
		log2PageSize: 12,
	}
	return b
}

// WithParallelEngine lets the EmuBuilder to use parallel engine.
func (b EmuBuilder) WithParallelEngine() EmuBuilder {
	b.useParallelEngine = true
	return b
}

// WithISADebugging enables ISA debugging in the simulation.
func (b EmuBuilder) WithISADebugging() EmuBuilder {
	b.debugISA = true
	return b
}

// WithVisTracing lets the platform to record traces for visualization purposes.
func (b EmuBuilder) WithVisTracing() EmuBuilder {
	b.traceVis = true
	return b
}

// WithMemTracing lets the platform to trace memory operations.
func (b EmuBuilder) WithMemTracing() EmuBuilder {
	b.traceMem = true
	return b
}

// WithNumGPU sets the number of GPUs to build.
func (b EmuBuilder) WithNumGPU(n int) EmuBuilder {
	b.numGPU = n
	return b
}

// WithLog2PageSize sets the page size as a power of 2.
func (b EmuBuilder) WithLog2PageSize(n uint64) EmuBuilder {
	b.log2PageSize = n
	return b
}

// WithMagicMemoryCopy uses global storage as memory components
func (b EmuBuilder) WithMagicMemoryCopy() EmuBuilder {
	b.useMagicMemoryCopy = true
	return b
}

// Build builds a emulation platform.
func (b EmuBuilder) Build() *Platform {
	var engine sim.Engine
	if b.useParallelEngine {
		engine = sim.NewParallelEngine()
	} else {
		engine = sim.NewSerialEngine()
	}
	// engine.AcceptHook(sim.NewEventLogger(log.New(os.Stdout, "", 0)))

	storage := mem.NewStorage(uint64(b.numGPU+1) * 4 * mem.GB)
	pageTable := vm.NewPageTable(b.log2PageSize)
	gpuDriver := b.buildGPUDriver(engine, pageTable, storage)
	connection := directconnection.MakeBuilder().
		WithEngine(engine).
		WithFreq(1*sim.GHz).
		Build("ExternalConn")

	gpuBuilder := b.createGPUBuilder(engine, gpuDriver, pageTable, storage)

	for i := 0; i < b.numGPU; i++ {
		gpu := gpuBuilder.
			WithMemOffset(uint64(i+1) * 4 * mem.GB).
			Build(fmt.Sprintf("GPU[%d]", i+1))

		cpPort := gpu.Domain.GetPortByName("CommandProcessor")
		gpuDriver.RegisterGPU(cpPort, driver.DeviceProperties{
			DRAMSize: 4 * mem.GB,
			CUCount:  64,
		})
		connection.PlugIn(cpPort, 64)

		b.gpus = append(b.gpus, gpu)
	}

	connection.PlugIn(gpuDriver.GetPortByName("GPU"), 4)

	return &Platform{
		Engine: engine,
		Driver: gpuDriver,
		GPUs:   b.gpus,
	}
}

func (b *EmuBuilder) createGPUBuilder(
	engine sim.Engine,
	gpuDriver *driver.Driver,
	pageTable vm.PageTable,
	storage *mem.Storage,
) EmuGPUBuilder {
	gpuBuilder := MakeEmuGPUBuilder().
		WithEngine(engine).
		WithDriver(gpuDriver).
		WithPageTable(pageTable).
		WithLog2PageSize(b.log2PageSize).
		WithMemCapacity(4 * mem.GB).
		WithStorage(storage)

	if b.debugISA {
		gpuBuilder = gpuBuilder.WithISADebugging()
	}

	if b.traceMem {
		gpuBuilder = gpuBuilder.WithMemTracing()
	}
	return gpuBuilder
}

func (b *EmuBuilder) buildGPUDriver(
	engine sim.Engine,
	pageTable vm.PageTable,
	storage *mem.Storage,
) *driver.Driver {
	gpuDriverBuilder := driver.MakeBuilder()

	if b.useMagicMemoryCopy {
		gpuDriverBuilder = gpuDriverBuilder.WithMagicMemoryCopyMiddleware()
	}

	gpuDriver := gpuDriverBuilder.
		WithEngine(engine).
		WithPageTable(pageTable).
		WithLog2PageSize(b.log2PageSize).
		WithGlobalStorage(storage).
		Build("Driver")

	return gpuDriver
}
