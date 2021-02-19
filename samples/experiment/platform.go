package main

import (
	"fmt"
	"log"
	"os"

	memtraces "gitlab.com/akita/mem/trace"
	"gitlab.com/akita/mgpusim"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/gpubuilder"
	"gitlab.com/akita/noc/networking/pcie"
	"gitlab.com/akita/util/tracing"
)

// R9NanoPlatformBuilder can build a platform that equips R9Nano GPU.
type R9NanoPlatformBuilder struct {
	useParallelEngine  bool
	debugISA           bool
	traceVis           bool
	visTraceStartTime  akita.VTimeInSec
	visTraceEndTime    akita.VTimeInSec
	traceMem           bool
	numGPU             int
	log2PageSize       uint64
	disableProgressBar bool
}

// MakeR9NanoBuilder creates a EmuBuilder with default parameters.
func MakeR9NanoBuilder() R9NanoPlatformBuilder {
	b := R9NanoPlatformBuilder{
		numGPU:            4,
		log2PageSize:      12,
		visTraceStartTime: -1,
		visTraceEndTime:   -1,
	}
	return b
}

// WithParallelEngine lets the EmuBuilder to use parallel engine.
func (b R9NanoPlatformBuilder) WithParallelEngine() R9NanoPlatformBuilder {
	b.useParallelEngine = true
	return b
}

// WithISADebugging enables ISA debugging in the simulation.
func (b R9NanoPlatformBuilder) WithISADebugging() R9NanoPlatformBuilder {
	b.debugISA = true
	return b
}

// WithVisTracing lets the platform to record traces for visualization purposes.
func (b R9NanoPlatformBuilder) WithVisTracing() R9NanoPlatformBuilder {
	b.traceVis = true
	return b
}

// WithPartialVisTracing lets the platform to record traces for visualization
// purposes. The trace will only be collected from the start time to the end
// time.
func (b R9NanoPlatformBuilder) WithPartialVisTracing(
	start, end akita.VTimeInSec,
) R9NanoPlatformBuilder {
	b.traceVis = true
	b.visTraceStartTime = start
	b.visTraceEndTime = end

	return b
}

// WithMemTracing lets the platform to trace memory operations.
func (b R9NanoPlatformBuilder) WithMemTracing() R9NanoPlatformBuilder {
	b.traceMem = true
	return b
}

// WithNumGPU sets the number of GPUs to build.
func (b R9NanoPlatformBuilder) WithNumGPU(n int) R9NanoPlatformBuilder {
	b.numGPU = n
	return b
}

// WithoutProgressBar disables the progress bar for kernel execution
func (b R9NanoPlatformBuilder) WithoutProgressBar() R9NanoPlatformBuilder {
	b.disableProgressBar = true
	return b
}

// WithLog2PageSize sets the page size as a power of 2.
func (b R9NanoPlatformBuilder) WithLog2PageSize(
	n uint64,
) R9NanoPlatformBuilder {
	b.log2PageSize = n
	return b
}

// Build builds a platform with R9Nano GPUs.
func (b R9NanoPlatformBuilder) Build() (akita.Engine, *driver.Driver) {
	engine := b.createEngine()

	mmuComponent, pageTable := b.createMMU(engine)
	gpuDriver := driver.NewDriver(engine, pageTable, b.log2PageSize)
	gpuBuilder := b.createGPUBuilder(engine, gpuDriver, mmuComponent)
	pcieConnector, rootComplexID :=
		b.createConnection(engine, gpuDriver, mmuComponent)

	mmuComponent.MigrationServiceProvider = gpuDriver.ToMMU

	rdmaAddressTable := b.createRDMAAddrTable()

	pmcAddressTable := b.createPMCPageTable()

	b.createGPUs(
		rootComplexID, pcieConnector,
		gpuBuilder, gpuDriver,
		rdmaAddressTable, pmcAddressTable)

	return engine, gpuDriver
}

func (b R9NanoPlatformBuilder) createGPUs(
	rootComplexID int,
	pcieConnector *pcie.Connector,
	gpuBuilder R9NanoGPUBuilder,
	gpuDriver *driver.Driver,
	rdmaAddressTable *cache.BankedLowModuleFinder,
	pmcAddressTable *cache.BankedLowModuleFinder,
) {
	lastSwitchID := rootComplexID
	for i := 1; i < b.numGPU+1; i++ {
		if i%2 == 1 {
			lastSwitchID = pcieConnector.AddSwitch(lastSwitchID)
		}

		b.createGPU(i, gpuBuilder, gpuDriver,
			rdmaAddressTable, pmcAddressTable,
			pcieConnector, lastSwitchID)
	}
}

func (b R9NanoPlatformBuilder) createPMCPageTable() *cache.BankedLowModuleFinder {
	pmcAddressTable := new(cache.BankedLowModuleFinder)
	pmcAddressTable.BankSize = 4 * mem.GB
	pmcAddressTable.LowModules = append(pmcAddressTable.LowModules, nil)
	return pmcAddressTable
}

func (b R9NanoPlatformBuilder) createRDMAAddrTable() *cache.BankedLowModuleFinder {
	rdmaAddressTable := new(cache.BankedLowModuleFinder)
	rdmaAddressTable.BankSize = 4 * mem.GB
	rdmaAddressTable.LowModules = append(rdmaAddressTable.LowModules, nil)
	return rdmaAddressTable
}

func (b R9NanoPlatformBuilder) createConnection(
	engine akita.Engine,
	gpuDriver *driver.Driver,
	mmuComponent *mmu.MMUImpl,
) (*pcie.Connector, int) {
	//connection := akita.NewDirectConnection(engine)
	// connection := noc.NewFixedBandwidthConnection(32, engine, 1*akita.GHz)
	// connection.SrcBufferCapacity = 40960000
	pcieConnector := pcie.NewConnector().
		WithEngine(engine).
		WithVersion3().
		WithX16().
		WithSwitchLatency(140).
		WithNetworkName("PCIe")
	pcieConnector.CreateNetwork()
	rootComplexID := pcieConnector.CreateRootComplex(
		[]akita.Port{
			gpuDriver.ToGPUs,
			gpuDriver.ToMMU,
			mmuComponent.MigrationPort,
			mmuComponent.ToTop,
		})
	return pcieConnector, rootComplexID
}

func (b R9NanoPlatformBuilder) createEngine() akita.Engine {
	var engine akita.Engine

	if b.useParallelEngine {
		engine = akita.NewParallelEngine()
	} else {
		engine = akita.NewSerialEngine()
	}
	// engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	return engine
}

func (b R9NanoPlatformBuilder) createMMU(
	engine akita.Engine,
) (*mmu.MMUImpl, vm.PageTable) {
	pageTable := vm.NewPageTable(b.log2PageSize)
	mmuBuilder := mmu.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * akita.GHz).
		WithLog2PageSize(b.log2PageSize).
		WithPageTable(pageTable)
	mmuComponent := mmuBuilder.Build("MMU")
	return mmuComponent, pageTable
}

func (b *R9NanoPlatformBuilder) createGPUBuilder(
	engine akita.Engine,
	gpuDriver *driver.Driver,
	mmuComponent *mmu.MMUImpl,
) R9NanoGPUBuilder {
	gpuBuilder := MakeR9NanoGPUBuilder().
		WithEngine(engine).
		WithMMU(mmuComponent).
		WithNumCUPerShaderArray(4).
		WithNumShaderArray(16).
		WithNumMemoryBank(8).
		WithLog2MemoryBankInterleavingSize(7).
		WithLog2PageSize(b.log2PageSize)

	gpuBuilder = b.setVisTracer(gpuDriver, gpuBuilder)
	gpuBuilder = b.setMemTracer(gpuBuilder)
	gpuBuilder = b.setISADebugger(gpuBuilder)

	return gpuBuilder
}

func (b *R9NanoPlatformBuilder) setISADebugger(
	gpuBuilder R9NanoGPUBuilder,
) R9NanoGPUBuilder {
	if !b.debugISA {
		return gpuBuilder
	}

	gpuBuilder = gpuBuilder.WithISADebugging()
	return gpuBuilder
}

func (b *R9NanoPlatformBuilder) setMemTracer(
	gpuBuilder R9NanoGPUBuilder,
) R9NanoGPUBuilder {
	if !b.traceMem {
		return gpuBuilder
	}

	file, err := os.Create("mem.trace")
	if err != nil {
		panic(err)
	}
	logger := log.New(file, "", 0)
	memTracer := memtraces.NewTracer(logger)
	gpuBuilder = gpuBuilder.WithMemTracer(memTracer)
	return gpuBuilder
}

func (b *R9NanoPlatformBuilder) setVisTracer(
	gpuDriver *driver.Driver,
	gpuBuilder R9NanoGPUBuilder,
) R9NanoGPUBuilder {
	if !b.traceVis {
		return gpuBuilder
	}

	tracer := tracing.NewMySQLTracerWithTimeRange(
		b.visTraceStartTime,
		b.visTraceEndTime)
	tracer.Init()
	tracing.CollectTrace(gpuDriver, tracer)

	gpuBuilder = gpuBuilder.WithVisTracer(tracer)
	return gpuBuilder
}

func (b *R9NanoPlatformBuilder) createGPU(
	index int,
	gpuBuilder R9NanoGPUBuilder,
	gpuDriver *driver.Driver,
	rdmaAddressTable *cache.BankedLowModuleFinder,
	pmcAddressTable *cache.BankedLowModuleFinder,
	pcieConnector *pcie.Connector,
	pcieSwitchID int,
) *mgpusim.GPU {
	name := fmt.Sprintf("GPU%d", index)
	memAddrOffset := uint64(index) * 4 * mem.GB
	gpu := gpuBuilder.
		WithMemAddrOffset(memAddrOffset).
		Build(name, uint64(index))
	gpuDriver.RegisterGPU(gpu, 4*mem.GB)
	gpu.CommandProcessor.Driver = gpuDriver.ToGPUs

	b.configRDMAEngine(gpu, rdmaAddressTable)
	b.configPMC(gpu, gpuDriver, pmcAddressTable)

	pcieConnector.PlugInDevice(pcieSwitchID, gpu.ExternalPorts())

	return gpu
}

func (b *R9NanoPlatformBuilder) configRDMAEngine(
	gpu *mgpusim.GPU,
	addrTable *cache.BankedLowModuleFinder,
) {
	gpu.RDMAEngine.RemoteRDMAAddressTable = addrTable
	addrTable.LowModules = append(
		addrTable.LowModules,
		gpu.RDMAEngine.ToOutside)
}

func (b *R9NanoPlatformBuilder) configPMC(
	gpu *mgpusim.GPU,
	gpuDriver *driver.Driver,
	addrTable *cache.BankedLowModuleFinder,
) {
	gpu.PMC.RemotePMCAddressTable = addrTable
	addrTable.LowModules = append(
		addrTable.LowModules,
		gpu.PMC.RemotePort)
	gpuDriver.RemotePMCPorts = append(
		gpuDriver.RemotePMCPorts, gpu.PMC.RemotePort)
}

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
