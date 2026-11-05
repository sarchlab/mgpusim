// Package timingconfig contains the configuration for the timing simulation.
package timingconfig

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	monitoring2 "github.com/sarchlab/akita/v5/monitoring"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/mi300x"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/r9nano"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"
)

// Port buffer sizes. The driver port mirrors the emulation platform's
// choice (v4 auto-created 40M-deep buffers; 4096 is plenty). The MMU top
// port mirrors the 4096-deep port the v4 MMU builder created.
const (
	driverGPUPortBufSize = 4096
	mmuTopPortBufSize    = 4096
	ctrlPortBufSize      = 1
)

// Builder builds a hardware platform for timing simulation.
type Builder struct {
	sampler    *sampling.SampledEngine
	simulation *simulation.Simulation
	monitor    *monitoring2.Monitor

	numGPUs            int
	numCUPerSA         int
	numSAPerGPU        int
	cpuMemSize         uint64
	gpuMemSize         uint64
	log2PageSize       uint64
	useMagicMemoryCopy bool
	gpuType            string
	switchLatency      int // PCIe/interconnect switch latency in cycles
	d2hCycles          int
	h2dCycles          int
	pageMigration      driver.PageMigrationSpec

	globalStorage     *mem.Storage
	rdmaAddressMapper mem.AddressMapper
}

// MakeBuilder creates a new Builder with default parameters.
func MakeBuilder() Builder {
	return Builder{
		numGPUs:            1,
		numCUPerSA:         4,
		numSAPerGPU:        16,
		cpuMemSize:         4 * mem.GB,
		gpuMemSize:         4 * mem.GB,
		log2PageSize:       12,
		useMagicMemoryCopy: false,
		gpuType:            "r9nano",
		switchLatency:      140, // default PCIe Gen4
		d2hCycles:          300,
		h2dCycles:          500,
		pageMigration:      driver.DefaultPageMigrationSpec(),
	}
}

// WithMonitor sets the optional dispatch progress monitor.
func (b Builder) WithMonitor(m *monitoring2.Monitor) Builder { b.monitor = m; return b }

// WithSimulation sets the simulation to use.
func (b Builder) WithSimulation(sim *simulation.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithNumGPUs sets the number of GPUs to simulate.
func (b Builder) WithNumGPUs(numGPUs int) Builder {
	b.numGPUs = numGPUs
	return b
}

// WithMagicMemoryCopy sets whether to use the magic memory copy middleware.
func (b Builder) WithMagicMemoryCopy() Builder {
	b.useMagicMemoryCopy = true
	return b
}

// WithGPUType sets the GPU type for timing simulation (r9nano or mi300x).
// WithPageMigration configures page migration between the GPUs
// (sarchlab/akita#589). The default policy is none.
func (b Builder) WithPageMigration(spec driver.PageMigrationSpec) Builder {
	b.pageMigration = spec
	return b
}

func (b Builder) WithGPUType(gpuType string) Builder {
	b.gpuType = gpuType
	return b
}

// Build registers the hardware platform and returns its Runtime. The Driver,
// GPUs, and all the connections register themselves with the simulation.
func (b Builder) Build() *gpuruntime.Runtime {
	b.adjustConfigForGPUType()
	b.sampler = sampling.NewSimulationEngine(
		b.simulation,
		*sampling.SampledRunnerGranularyFlag,
		*sampling.SampledRunnerThresholdFlag,
		*sampling.SampledRunnerFlag,
	)
	b.cpuGPUMemSizeMustEqual()

	b.globalStorage = mem.MakeStorageBuilder().
		WithSimulation(b.simulation).
		WithCapacity(uint64(b.numGPUs)*b.gpuMemSize + b.cpuMemSize).
		Build("GlobalStorage")

	mmuComp, pageTable := b.createMMU()
	gpuBuilder := b.createGPUBuilder(mmuComp)
	interDeviceConn := directconnection.NewConnection("InterDeviceConn", b.simulation, timing.GHz)
	interDeviceConn.BindPort(mmuComp.Ports.Top)
	for _, p := range mmuComp.Ports.Notification {
		interDeviceConn.BindPort(p)
	}
	spec := driver.DefaultSpec()
	runtimeSpec := gpuruntime.DefaultRuntimeSpec()
	runtimeSpec.Log2PageSize = b.log2PageSize
	spec.UseMagicMemoryCopy = b.useMagicMemoryCopy
	spec.D2HCycles = b.d2hCycles
	spec.H2DCycles = b.h2dCycles
	spec.PageMigration = b.pageMigration
	for i := 1; i <= b.numGPUs; i++ {
		gpu := b.createGPU(i, gpuBuilder, interDeviceConn)
		spec.GPUs = append(spec.GPUs, gpu.CommandProcessorPort.AsRemote())
		spec.MigrationAgents = append(spec.MigrationAgents,
			gpu.PageMigrationPort.AsRemote())
		runtimeSpec.Devices = append(
			runtimeSpec.Devices,
			gpuruntime.DeviceProperties{CUCount: b.numCUPerSA * b.numSAPerGPU, DRAMSize: b.gpuMemSize},
		)
	}
	runtime := gpuruntime.NewRuntime(b.simulation, "Runtime", runtimeSpec, pageTable, b.globalStorage)
	comp := driver.MakeBuilder().WithSimulation(b.simulation).WithSpec(spec).
		WithResources(driver.Resources{
			Runtime:       runtime,
			PageTable:     pageTable,
			GlobalStorage: b.globalStorage,
		}).Build("Driver")
	port := b.buildPort(comp, driver.GPUPortName, driverGPUPortBufSize)
	interDeviceConn.BindPort(port)
	interDeviceConn.BindPort(
		b.buildPort(comp, driver.PageMigrationPortName, driverGPUPortBufSize))
	return runtime
}

func (b *Builder) cpuGPUMemSizeMustEqual() {
	if b.cpuMemSize != b.gpuMemSize {
		panic("currently only support cpuMemSize == gpuMemSize")
	}
}

func (b *Builder) adjustConfigForGPUType() {
	switch b.gpuType {
	case "mi300x":
		b.numCUPerSA = mi300x.NumCUPerShaderArray
		b.numSAPerGPU = mi300x.NumShaderArray
		// Host-transfer latencies are inherited from the MI300A baseline. MI300X
		// is a DISCRETE GPU (host link is PCIe Gen5, not the APU's on-die Infinity
		// Fabric), so these want re-tuning for MI300X -- but the platform
		// currently uses a direct connection that models no link latency anyway
		// (see createConnection), so they have no effect until that is restored.
		b.switchLatency = 15
		b.d2hCycles = 150
		b.h2dCycles = 250
		// The ROCm/HIP runtime backs large device allocations with 2 MB
		// huge pages, so a pointer chase over hundreds of MB stays
		// TLB-resident and the latency curve is pure cache hierarchy (the
		// real MI300X cache_latency shows no TLB wall). Modeling 4 KB pages
		// would make the L1/L2 TLBs thrash at ~1 MB working sets and inject
		// page-walk latency the hardware never pays. This sets the page size
		// for the MMU/page table and every TLB consistently.
		b.log2PageSize = 21 // 2 MB huge pages
	default:
		// Keep defaults for r9nano
	}
}

func (b *Builder) createMMU() (*mmu.Comp, *vm.PageTable) {
	pageTable := vm.MakePageTableBuilder().
		WithSimulation(b.simulation).
		WithLog2PageSize(b.log2PageSize).
		Build("PageTable")

	spec := mmu.DefaultSpec()
	spec.Freq = 1 * timing.GHz
	spec.Latency = 100 // v4: page walking latency
	spec.Log2PageSize = b.log2PageSize

	// With page migration on fault, a walk of a unified page on another GPU
	// faults and is reported to the driver from the MMU's Notification[0]
	// port.
	faults := b.pageMigration.Policy == driver.PageMigrationFault
	if faults {
		spec.FaultHandler = messaging.RemotePort("Driver.PageMigration")
	}

	mmuComponent := modeling.MakeBuilder[*mmu.Comp](spec).WithSimulation(b.simulation).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("MMU")

	b.buildPort(mmuComponent, "Top", mmuTopPortBufSize)
	b.buildPort(mmuComponent, "Control", ctrlPortBufSize)

	if faults {
		b.buildPort(mmuComponent, "Notification[0]", mmuTopPortBufSize)
	}

	return mmuComponent, pageTable
}

// buildPort creates a port instance for a declared port and assigns it to
// the component.
func (b *Builder) buildPort(
	comp interface{ BindPort(string, messaging.Port) },
	name string,
	bufSize int,
) messaging.Port {
	port := twowaybuffered.MakeBuilder().
		WithSimulation(b.simulation).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: bufSize, OutgoingCapacity: bufSize}).
		Build()
	comp.BindPort(name, port)

	return port
}

func (b *Builder) createGPUBuilder(
	mmuComponent *mmu.Comp,
) gpubuilder.GPUBuilder {
	b.createRDMAAddressMapper()

	driverPort := messaging.RemotePort("Driver.GPU")

	var gpuBuilder gpubuilder.GPUBuilder

	switch b.gpuType {
	case "mi300x":
		gpuBuilder = mi300x.MakeBuilder().WithSampler(b.sampler).WithMonitor(b.monitor).
			WithSimulation(b.simulation).
			WithMMU(mmuComponent).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage).
			WithDriverPort(driverPort)
	default:
		gpuBuilder = r9nano.MakeBuilder().WithSampler(b.sampler).WithMonitor(b.monitor).
			WithSimulation(b.simulation).
			WithMMU(mmuComponent).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage).
			WithDriverPort(driverPort)
	}

	// With access-counter migration, the RDMA engines count the remote L1
	// misses per migration unit and notify the driver.
	if b.pageMigration.Policy == driver.PageMigrationCounter {
		gpuBuilder = gpuBuilder.WithAccessCounter(
			messaging.RemotePort("Driver."+driver.PageMigrationPortName),
			b.pageMigration.AccessCounterThreshold,
			b.pageMigration.Log2Unit)
	}

	return gpuBuilder
}

func (b *Builder) createRDMAAddressMapper() {
	b.rdmaAddressMapper = mem.AddressMapper{}
	b.rdmaAddressMapper.InterleavingSize = b.gpuMemSize
	b.rdmaAddressMapper.Destinations = append(b.rdmaAddressMapper.Destinations,
		messaging.RemotePort("CPU"))
	for i := 1; i <= b.numGPUs; i++ {
		b.rdmaAddressMapper.Destinations = append(b.rdmaAddressMapper.Destinations,
			messaging.RemotePort(fmt.Sprintf("GPU[%d].RDMA.RDMADataOutside", i)))
	}
}

func (b *Builder) createGPU(
	index int,
	gpuBuilder gpubuilder.GPUBuilder,
	interDeviceConn *directconnection.Connection,
) *gpubuilder.GPU {
	name := fmt.Sprintf("GPU[%d]", index)
	memAddrOffset := uint64(index) * b.gpuMemSize
	gpu := gpuBuilder.
		WithGPUID(uint64(index)).
		WithMemAddrOffset(memAddrOffset).
		WithRDMAAddressMapper(b.rdmaAddressMapper).
		Build(name)

	for _, port := range gpu.ExternalPorts() {
		interDeviceConn.BindPort(port)
	}

	return gpu
}
