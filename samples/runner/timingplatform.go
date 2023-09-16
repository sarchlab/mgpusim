package runner

import (
	"fmt"
	"log"
	"os"

	memtraces "github.com/sarchlab/akita/v3/mem/trace"

	"github.com/sarchlab/akita/v3/analysis"
	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/mem/vm"
	"github.com/sarchlab/akita/v3/mem/vm/mmu"
	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/noc/networking/pcie"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/driver"
)

// R9NanoPlatformBuilder can build a platform that equips R9Nano GPU.
type R9NanoPlatformBuilder struct {
	useParallelEngine                  bool
	debugISA                           bool
	traceVis                           bool
	traceVisStartTime, traceVisEndTime sim.VTimeInSec
	traceMem                           bool
	numGPU                             int
	numSAPerGPU                        int
	numCUPerSA                         int
	useMagicMemoryCopy                 bool
	log2PageSize                       uint64

	engine               sim.Engine
	monitor              *monitoring.Monitor
	perfAnalysisFileName string
	perfAnalyzingPeriod  float64
	perfAnalyzer         *analysis.PerfAnalyzer
	visTracer            tracing.Tracer

	globalStorage *mem.Storage

	gpus []*GPU
}

// MakeR9NanoBuilder creates a EmuBuilder with default parameters.
func MakeR9NanoBuilder() R9NanoPlatformBuilder {
	b := R9NanoPlatformBuilder{
		numGPU:            4,
		numSAPerGPU:       16,
		numCUPerSA:        4,
		log2PageSize:      12,
		traceVisStartTime: -1,
		traceVisEndTime:   -1,
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
	start, end sim.VTimeInSec,
) R9NanoPlatformBuilder {
	b.traceVis = true
	b.traceVisStartTime = start
	b.traceVisEndTime = end

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

// WithLog2PageSize sets the page size as a power of 2.
func (b R9NanoPlatformBuilder) WithLog2PageSize(
	n uint64,
) R9NanoPlatformBuilder {
	b.log2PageSize = n
	return b
}

// WithMonitor sets the monitor that is used to monitor the simulation
func (b R9NanoPlatformBuilder) WithMonitor(
	m *monitoring.Monitor,
) R9NanoPlatformBuilder {
	b.monitor = m
	return b
}

// WithBufferAnalyzer sets the trace that dumps the buffer levers.
func (b R9NanoPlatformBuilder) WithPerfAnalyzer(
	Name string,
	Period float64,
) R9NanoPlatformBuilder {
	b.perfAnalysisFileName = Name
	b.perfAnalyzingPeriod = Period
	return b
}

// WithMagicMemoryCopy uses global storage as memory components
func (b R9NanoPlatformBuilder) WithMagicMemoryCopy() R9NanoPlatformBuilder {
	b.useMagicMemoryCopy = true
	return b
}

// Build builds a platform with R9Nano GPUs.
func (b R9NanoPlatformBuilder) Build() *Platform {
	b.engine = b.createEngine()
	if b.monitor != nil {
		b.monitor.RegisterEngine(b.engine)
	}

	b.setupBufferLevelTracing()
	b.setupVisTracing()

	b.globalStorage = mem.NewStorage(uint64(1+b.numGPU) * 4 * mem.GB)

	mmuComponent, pageTable := b.createMMU(b.engine)

	gpuDriver := b.buildGPUDriver(pageTable)

	gpuBuilder := b.createGPUBuilder(b.engine, gpuDriver, mmuComponent)
	pcieConnector, rootComplexID :=
		b.createConnection(b.engine, gpuDriver, mmuComponent)

	mmuComponent.MigrationServiceProvider = gpuDriver.GetPortByName("MMU")

	rdmaAddressTable := b.createRDMAAddrTable()
	pmcAddressTable := b.createPMCPageTable()

	b.createGPUs(
		rootComplexID, pcieConnector,
		gpuBuilder, gpuDriver,
		rdmaAddressTable, pmcAddressTable)

	pcieConnector.EstablishRoute()

	return &Platform{
		Engine: b.engine,
		Driver: gpuDriver,
		GPUs:   b.gpus,
	}
}

func (b R9NanoPlatformBuilder) buildGPUDriver(
	pageTable vm.PageTable,
) *driver.Driver {
	gpuDriverBuilder := driver.MakeBuilder()
	if b.useMagicMemoryCopy {
		gpuDriverBuilder = gpuDriverBuilder.WithMagicMemoryCopyMiddleware()
	}
	gpuDriver := gpuDriverBuilder.
		WithEngine(b.engine).
		WithPageTable(pageTable).
		WithLog2PageSize(b.log2PageSize).
		WithGlobalStorage(b.globalStorage).
		Build("Driver")
	if b.visTracer != nil {
		tracing.CollectTrace(gpuDriver, b.visTracer)
	}

	if b.monitor != nil {
		b.monitor.RegisterComponent(gpuDriver)
	}

	if b.perfAnalyzer != nil {
		b.perfAnalyzer.RegisterComponent(gpuDriver)
	}

	// file, err := os.Create("driver_comm.csv")
	// if err != nil {
	// 	panic(err)
	// }
	// gpuDriver.GetPortByName("GPU").AcceptHook(
	// 	sim.NewPortMsgLogger(log.New(file, "", 0)))

	return gpuDriver
}

func (b *R9NanoPlatformBuilder) setupVisTracing() {
	if !b.traceVis {
		return
	}

	var backend tracing.TracerBackend
	switch *visTracerDB {
	case "sqlite":
		be := tracing.NewSQLiteTraceWriter(*visTracerDBFileName)
		be.Init()
		backend = be
	case "csv":
		be := tracing.NewCSVTraceWriter(*visTracerDBFileName)
		be.Init()
		backend = be
	case "mysql":
		be := tracing.NewMySQLTraceWriter()
		be.Init()
		backend = be
	default:
		panic(fmt.Sprintf(
			"Tracer database type must be [sqlite|csv|mysql]. "+
				"Provided value %s is not supported.",
			*visTracerDB))
	}

	visTracer := tracing.NewDBTracer(b.engine, backend)
	visTracer.SetTimeRange(b.traceVisStartTime, b.traceVisEndTime)

	b.visTracer = visTracer
}

func (b *R9NanoPlatformBuilder) setupBufferLevelTracing() {
	// name := fmt.Sprintf("test.csv")
	// period := float64(1e-6)
	// b.perfAnalysisFileName = name
	// b.perfAnalyzingPeriod = period

	if b.perfAnalysisFileName != "" {
		b.perfAnalyzer = analysis.MakePerfAnalyzerBuilder().
			WithPeriod(sim.VTimeInSec(b.perfAnalyzingPeriod)).
			WithDBFilename(b.perfAnalysisFileName).
			Build()
	}
}

func (b *R9NanoPlatformBuilder) createGPUs(
	rootComplexID int,
	pcieConnector *pcie.Connector,
	gpuBuilder R9NanoGPUBuilder,
	gpuDriver *driver.Driver,
	rdmaAddressTable *mem.BankedLowModuleFinder,
	pmcAddressTable *mem.BankedLowModuleFinder,
) {
	lastSwitchID := rootComplexID
	for i := 1; i < b.numGPU+1; i++ {
		if i%2 == 1 {
			lastSwitchID = pcieConnector.AddSwitch(rootComplexID)
		}

		b.createGPU(i, gpuBuilder, gpuDriver,
			rdmaAddressTable, pmcAddressTable,
			pcieConnector, lastSwitchID)
	}
}

func (b R9NanoPlatformBuilder) createPMCPageTable() *mem.BankedLowModuleFinder {
	pmcAddressTable := new(mem.BankedLowModuleFinder)
	pmcAddressTable.BankSize = 4 * mem.GB
	pmcAddressTable.LowModules = append(pmcAddressTable.LowModules, nil)
	return pmcAddressTable
}

func (b R9NanoPlatformBuilder) createRDMAAddrTable() *mem.BankedLowModuleFinder {
	rdmaAddressTable := new(mem.BankedLowModuleFinder)
	rdmaAddressTable.BankSize = 4 * mem.GB
	rdmaAddressTable.LowModules = append(rdmaAddressTable.LowModules, nil)
	return rdmaAddressTable
}

func (b R9NanoPlatformBuilder) createConnection(
	engine sim.Engine,
	gpuDriver *driver.Driver,
	mmuComponent *mmu.MMU,
) (*pcie.Connector, int) {
	//connection := sim.NewDirectConnection(engine)
	// connection := noc.NewFixedBandwidthConnection(32, engine, 1*sim.GHz)
	// connection.SrcBufferCapacity = 40960000
	pcieConnector := pcie.NewConnector().
		WithEngine(engine).
		WithVersion(3, 16).
		WithSwitchLatency(140)

	if b.visTracer != nil {
		pcieConnector = pcieConnector.WithVisTracer(b.visTracer)
	}

	pcieConnector.CreateNetwork("PCIe")
	rootComplexID := pcieConnector.AddRootComplex(
		[]sim.Port{
			gpuDriver.GetPortByName("GPU"),
			gpuDriver.GetPortByName("MMU"),
			mmuComponent.GetPortByName("Migration"),
			mmuComponent.GetPortByName("Top"),
		})
	return pcieConnector, rootComplexID
}

func (b R9NanoPlatformBuilder) createEngine() sim.Engine {
	var engine sim.Engine

	if b.useParallelEngine {
		engine = sim.NewParallelEngine()
	} else {
		engine = sim.NewSerialEngine()
	}
	// engine.AcceptHook(sim.NewEventLogger(log.New(os.Stdout, "", 0)))

	return engine
}

func (b R9NanoPlatformBuilder) createMMU(
	engine sim.Engine,
) (*mmu.MMU, vm.PageTable) {
	pageTable := vm.NewPageTable(b.log2PageSize)
	mmuBuilder := mmu.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithPageWalkingLatency(100).
		WithLog2PageSize(b.log2PageSize).
		WithPageTable(pageTable)

	mmuComponent := mmuBuilder.Build("MMU")

	if b.monitor != nil {
		b.monitor.RegisterComponent(mmuComponent)
	}

	return mmuComponent, pageTable
}

func (b *R9NanoPlatformBuilder) createGPUBuilder(
	engine sim.Engine,
	gpuDriver *driver.Driver,
	mmuComponent *mmu.MMU,
) R9NanoGPUBuilder {
	gpuBuilder := MakeR9NanoGPUBuilder().
		WithEngine(engine).
		WithMMU(mmuComponent).
		WithNumCUPerShaderArray(b.numCUPerSA).
		WithNumShaderArray(b.numSAPerGPU).
		WithNumMemoryBank(16).
		WithLog2MemoryBankInterleavingSize(7).
		WithLog2PageSize(b.log2PageSize).
		WithGlobalStorage(b.globalStorage)

	if b.monitor != nil {
		gpuBuilder = gpuBuilder.WithMonitor(b.monitor)
	}

	if b.perfAnalyzer != nil {
		gpuBuilder = gpuBuilder.WithPerfAnalyzer(b.perfAnalyzer)
	}

	if b.visTracer != nil {
		gpuBuilder = gpuBuilder.WithVisTracer(b.visTracer)
	}

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
	memTracer := memtraces.NewTracer(logger, b.engine)
	gpuBuilder = gpuBuilder.WithMemTracer(memTracer)
	return gpuBuilder
}

func (b *R9NanoPlatformBuilder) createGPU(
	index int,
	gpuBuilder R9NanoGPUBuilder,
	gpuDriver *driver.Driver,
	rdmaAddressTable *mem.BankedLowModuleFinder,
	pmcAddressTable *mem.BankedLowModuleFinder,
	pcieConnector *pcie.Connector,
	pcieSwitchID int,
) *GPU {
	name := fmt.Sprintf("GPU[%d]", index)
	memAddrOffset := uint64(index) * 4 * mem.GB
	gpu := gpuBuilder.
		WithMemAddrOffset(memAddrOffset).
		Build(name, uint64(index))
	gpuDriver.RegisterGPU(
		gpu.Domain.GetPortByName("CommandProcessor"),
		driver.DeviceProperties{
			CUCount:  b.numCUPerSA * b.numSAPerGPU,
			DRAMSize: 4 * mem.GB,
		},
	)
	gpu.CommandProcessor.Driver = gpuDriver.GetPortByName("GPU")

	b.configRDMAEngine(gpu, rdmaAddressTable)
	b.configPMC(gpu, gpuDriver, pmcAddressTable)

	pcieConnector.PlugInDevice(pcieSwitchID, gpu.Domain.Ports())

	b.gpus = append(b.gpus, gpu)

	return gpu
}

func (b *R9NanoPlatformBuilder) configRDMAEngine(
	gpu *GPU,
	addrTable *mem.BankedLowModuleFinder,
) {
	gpu.RDMAEngine.RemoteRDMAAddressTable = addrTable
	addrTable.LowModules = append(
		addrTable.LowModules,
		gpu.RDMAEngine.ToOutside)
}

func (b *R9NanoPlatformBuilder) configPMC(
	gpu *GPU,
	gpuDriver *driver.Driver,
	addrTable *mem.BankedLowModuleFinder,
) {
	gpu.PMC.RemotePMCAddressTable = addrTable
	addrTable.LowModules = append(
		addrTable.LowModules,
		gpu.PMC.GetPortByName("Remote"))
	gpuDriver.RemotePMCPorts = append(
		gpuDriver.RemotePMCPorts, gpu.PMC.GetPortByName("Remote"))
}
