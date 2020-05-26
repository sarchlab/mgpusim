package platform

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/gpubuilder"
)

// EmuBuilder can build a platform for emulation purposes.
type EmuBuilder struct {
	useParallelEngine  bool
	debugISA           bool
	traceVis           bool
	traceMem           bool
	numGPU             int
	log2PageSize       uint64
	disableProgressBar bool
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

// WithoutProgressBar disables the progress bar for kernel execution
func (b EmuBuilder) WithoutProgressBar() EmuBuilder {
	b.disableProgressBar = true
	return b
}

// WithLog2PageSize sets the page size as a power of 2.
func (b EmuBuilder) WithLog2PageSize(n uint64) EmuBuilder {
	b.log2PageSize = n
	return b
}

// Build builds a emulation platform.
func (b EmuBuilder) Build() (akita.Engine, *driver.Driver) {
	var engine akita.Engine
	if b.useParallelEngine {
		engine = akita.NewParallelEngine()
	} else {
		engine = akita.NewSerialEngine()
	}
	// engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	pageTable := vm.NewPageTable(b.log2PageSize)
	gpuDriver := driver.NewDriver(engine, pageTable, b.log2PageSize)
	connection := akita.NewDirectConnection("ExternalConn", engine, 1*akita.GHz)
	storage := mem.NewStorage(uint64(b.numGPU+1) * 4 * mem.GB)

	gpuBuilder := gpubuilder.MakeEmuGPUBuilder().
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

	if b.disableProgressBar {
		gpuBuilder = gpuBuilder.WithoutProgressBar()
	}

	for i := 0; i < b.numGPU; i++ {
		gpu := gpuBuilder.
			WithMemOffset(uint64(i+1) * 4 * mem.GB).
			Build(fmt.Sprintf("GPU_%d", i+1))

		gpuDriver.RegisterGPU(gpu, 4*mem.GB)
		connection.PlugIn(gpu.CommandProcessor.ToDriver, 64)
	}

	connection.PlugIn(gpuDriver.ToGPUs, 4)

	return engine, gpuDriver
}
