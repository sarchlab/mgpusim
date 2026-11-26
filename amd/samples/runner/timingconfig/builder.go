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
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/mi300x"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/r9nano"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/waferscale"
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
	mi300xFreq         timing.Freq
	switchLatency      int // latency of a PCIe switch or mesh hop in cycles
	rootComplexLatency int // latency of the PCIe root complex in cycles
	d2hCycles          int
	h2dCycles          int
	pageMigration      driver.PageMigrationSpec
	pageTable          PageTableSpec
	hostMemory         *HostMemorySpec
	allocator          gpuruntime.AllocatorType
	waferMesh          waferscale.Mesh

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
		switchLatency:      140,
		rootComplexLatency: 140,
		d2hCycles:          300,
		h2dCycles:          500,
		pageMigration:      driver.DefaultPageMigrationSpec(),
		pageTable:          DefaultPageTableSpec(),
		waferMesh:          waferscale.DefaultMesh,
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

// WithPageMigration sets the policy and the tuning parameters of page
// migration between the GPUs. The default is
// driver.DefaultPageMigrationSpec: pages migrate on faults
// (sarchlab/akita#589). Access-counter migration (sarchlab/akita#590) and no
// migration are other policies of the same specification.
func (b Builder) WithPageMigration(spec driver.PageMigrationSpec) Builder {
	b.pageMigration = spec
	return b
}

// WithAllocator selects the physical-memory allocator of the devices. The
// default is the free list; gpuruntime.AllocatorBuddy reuses freed frames
// first.
func (b Builder) WithAllocator(allocator gpuruntime.AllocatorType) Builder {
	b.allocator = allocator
	return b
}

// WithGPUType sets the GPU type for timing simulation: r9nano, mi300x,
// waferscale (R9 Nano tiles), or waferscale-<tile>. A wafer-scale GPU always
// has one GPU per non-host position of its mesh.
func (b Builder) WithGPUType(gpuType string) Builder {
	b.gpuType = gpuType
	return b
}

// WithMI300XFreq selects the clock for the MI300X GPU clock domain. A zero
// frequency keeps the model's default. Independent clock domains retain
// their configured frequencies.
func (b Builder) WithMI300XFreq(freq timing.Freq) Builder {
	b.mi300xFreq = freq
	return b
}

// WithWaferScaleMesh sets the mesh shape of a wafer-scale GPU. The host takes
// the centre position and every other position holds a GPU tile, so a
// width x height mesh has width*height-1 GPUs. The default is
// waferscale.DefaultMesh (5x5). Other GPU types ignore it.
func (b Builder) WithWaferScaleMesh(width, height int) Builder {
	b.waferMesh = waferscale.Mesh{Width: width, Height: height}
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

	pageTable := vm.MakePageTableBuilder().WithSimulation(b.simulation).WithLog2PageSize(b.log2PageSize).Build("PageTable")
	network := b.createInterDeviceNetwork()
	b.buildCPUMemory(network)
	spec := driver.DefaultSpec()
	runtimeSpec := gpuruntime.DefaultRuntimeSpec()
	legacy := b.configureTranslation(pageTable, network, &spec, &runtimeSpec)
	b.createRDMAAddressMapper()
	runtimeSpec.Log2PageSize = b.log2PageSize
	runtimeSpec.Allocator = b.allocator
	spec.UseMagicMemoryCopy = b.useMagicMemoryCopy
	spec.D2HCycles = b.d2hCycles
	spec.H2DCycles = b.h2dCycles
	spec.PageMigration = b.pageMigration
	for i := 1; i <= b.numGPUs; i++ {
		gpu := b.createGPU(i, b.createGPUBuilder(legacy, i), network)
		if c := gpu.PageTableMMU; c != nil {
			spec.PageTables = append(spec.PageTables, pageTableTarget(c))
		}
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
	b.bindDriverPorts(comp, network, legacy == nil)
	network.Connect()
	return runtime
}

// bindDriverPorts plugs the driver into the network. Detailed translation adds
// a fault port and a page-table port that reaches every walker.
func (b *Builder) bindDriverPorts(comp *driver.Comp, network interDeviceNetwork, detailedTranslation bool) {
	for _, name := range []string{
		driver.GPUPortName, driver.PageMigrationPortName, driver.MigrationNotificationPortName,
	} {
		network.BindHostPort(b.buildPort(comp, name, driverGPUPortBufSize))
	}
	if detailedTranslation {
		network.BindHostPort(b.buildPort(comp, "PageFault[0]", driverGPUPortBufSize))
		network.BindHostPort(b.buildPort(comp, "PageTable[0]", driverGPUPortBufSize))
	}
}

func (b *Builder) configureTranslation(pageTable *vm.PageTable, network interDeviceNetwork,
	spec *driver.Spec, runtimeSpec *gpuruntime.RuntimeSpec) *mmu.Comp {
	var legacy *mmu.Comp
	if b.pageTable.FixedLatency {
		legacy = b.createMMU(pageTable)
		network.BindHostPort(legacy.Ports.Top)
		for _, p := range legacy.Ports.Notification {
			network.BindHostPort(p)
		}
	} else {
		io := b.buildIOMMU(network)
		spec.PageTables = append(spec.PageTables, pageTableTarget(io))
		spec.GlobalGPUPageTable = b.pageTable.GlobalGPU
		runtimeSpec.CPUPageTableBytes = io.Spec.PoolNodes * 4096
		runtimeSpec.GPUPageTableBytes = b.pageTable.GPU.PoolNodes * 4096
	}
	return legacy
}

// migrationNotificationPort is where the MMU reports the faults and the RDMA
// engines the access counters.
const migrationNotificationPort = messaging.RemotePort(
	"Driver." + driver.MigrationNotificationPortName)

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
		// Host-transfer latencies are inherited from the MI300A baseline and are
		// not tuned for the MI300X, a discrete GPU on PCIe Gen5.
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
		// Keep defaults for r9nano and the wafer-scale tiles
	}
	if b.pageTable.Log2PageSize != 0 {
		b.log2PageSize = b.pageTable.Log2PageSize
	}
	// The wafer-scale IOMMU pool depends on the final page size.
	if tile, ok := waferscale.ParseGPUType(b.gpuType); ok {
		b.adjustConfigForWaferScale(waferscale.TileByName(tile))
	}
}

func (b *Builder) adjustConfigForWaferScale(tile waferscale.Tile) {
	b.waferMesh.MustBeValid()
	// The wafer has a fixed number of tiles, whatever GPUs a run uses.
	b.numGPUs = b.waferMesh.NumTiles()
	b.numCUPerSA = tile.NumCUPerShaderArray
	b.numSAPerGPU = tile.NumShaderArray
	b.gpuMemSize = tile.DRAMSize
	b.switchLatency = waferscale.SwitchLatency
	// The default IOMMU pool cannot map the memory of all the tiles. Keep
	// a pool size that WithPageTable set explicitly.
	if b.pageTable.IOMMU.PoolNodes == DefaultPageTableSpec().IOMMU.PoolNodes {
		b.pageTable.IOMMU.PoolNodes = max(b.pageTable.IOMMU.PoolNodes,
			waferscale.IOMMUPoolNodes(b.waferMesh, tile, b.log2PageSize))
	}
	if b.pageTable.IOMMU.PoolNodes*4096 > b.cpuMemSize {
		panic(fmt.Sprintf("timingconfig: the IOMMU pool of a %dx%d wafer does not fit in CPU memory",
			b.waferMesh.Width, b.waferMesh.Height))
	}
}

func (b *Builder) createMMU(pageTable *vm.PageTable) *mmu.Comp {
	spec := mmu.DefaultSpec()
	spec.Freq = 1 * timing.GHz
	spec.Latency = 100 // v4: page walking latency
	spec.Log2PageSize = b.log2PageSize

	// With page migration on fault, a walk of a unified page on another GPU
	// faults and is reported to the driver from the MMU's Notification[0]
	// port.
	faults := b.pageMigration.Policy == driver.PageMigrationFault
	if faults {
		spec.FaultHandler = migrationNotificationPort
	}

	mmuComponent := modeling.MakeBuilder[*mmu.Comp](spec).WithSimulation(b.simulation).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("MMU")

	b.buildPort(mmuComponent, "Top", mmuTopPortBufSize)
	b.buildPort(mmuComponent, "Control", ctrlPortBufSize)

	if faults {
		b.buildPort(mmuComponent, "Notification[0]", mmuTopPortBufSize)
	}

	return mmuComponent
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
	mmuComponent *mmu.Comp, index int,
) gpubuilder.GPUBuilder {
	local := b.pageTable.GPU
	local.PoolBase = uint64(index) * b.gpuMemSize
	local.FaultHandler = "IOMMU.FaultIn"
	local.ConsultRemote = b.pageTable.GlobalGPU
	driverPort := messaging.RemotePort("Driver.GPU")

	var gpuBuilder gpubuilder.GPUBuilder

	kind := b.gpuType
	if tile, ok := waferscale.ParseGPUType(b.gpuType); ok {
		kind = waferscale.GPUType + "-" + tile
	}

	switch kind {
	case "mi300x":
		gpu := mi300x.MakeBuilder().WithSampler(b.sampler).WithMonitor(b.monitor).
			WithSimulation(b.simulation).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage)
		if b.mi300xFreq != 0 {
			gpu = gpu.WithFreq(b.mi300xFreq)
		}
		if mmuComponent != nil {
			gpu = gpu.WithMMU(mmuComponent)
		} else {
			gpu = gpu.WithPageTableMMU(local)
		}
		gpuBuilder = gpu
	case waferscale.GPUType + "-" + waferscale.TileR9Nano:
		gpu := waferscale.MakeR9NanoTileBuilder().WithSampler(b.sampler).WithMonitor(b.monitor).
			WithSimulation(b.simulation).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage)
		if mmuComponent != nil {
			gpu = gpu.WithMMU(mmuComponent)
		} else {
			gpu = gpu.WithPageTableMMU(local)
		}
		gpuBuilder = gpu
	default:
		gpu := r9nano.MakeBuilder().WithSampler(b.sampler).WithMonitor(b.monitor).
			WithSimulation(b.simulation).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage)
		if mmuComponent != nil {
			gpu = gpu.WithMMU(mmuComponent)
		} else {
			gpu = gpu.WithPageTableMMU(local)
		}
		gpuBuilder = gpu
	}

	return gpuBuilder.WithDriverPort(driverPort).WithPageMigration(b.gpuPageMigrationConfig())
}

// gpuPageMigrationConfig derives what each GPU needs from the
// page-migration specification of the system.
func (b *Builder) gpuPageMigrationConfig() gpubuilder.PageMigrationConfig {
	cfg := gpubuilder.PageMigrationConfig{
		DataMoverPages: b.pageMigration.DataMoverPages,
		PeerMemory:     b.createPeerMemoryMapper(),
	}

	// With access-counter migration, the RDMA engines count the remote
	// accesses per physical page frame and notify the driver.
	if b.pageMigration.Policy == driver.PageMigrationCounter {
		cfg.AccessCounterHandler = migrationNotificationPort
		cfg.AccessCounterThreshold = b.pageMigration.AccessCounterThreshold
	}

	return cfg
}

// createPeerMemoryMapper maps a physical address to the port where the data
// movers of the other GPUs reach the memory below the L2 caches of the GPU
// that owns the address.
func (b *Builder) createPeerMemoryMapper() mem.AddressMapper {
	mapper := mem.AddressMapper{InterleavingSize: b.gpuMemSize}
	mapper.Destinations = append(mapper.Destinations, cpuMemoryTop)

	for i := 1; i <= b.numGPUs; i++ {
		mapper.Destinations = append(mapper.Destinations,
			messaging.RemotePort(fmt.Sprintf("GPU[%d].RDMA.MigrationOutside", i)))
	}

	return mapper
}

func (b *Builder) createRDMAAddressMapper() {
	b.rdmaAddressMapper = mem.AddressMapper{}
	b.rdmaAddressMapper.InterleavingSize = b.gpuMemSize
	b.rdmaAddressMapper.Destinations = append(b.rdmaAddressMapper.Destinations,
		cpuMemoryTop)
	for i := 1; i <= b.numGPUs; i++ {
		b.rdmaAddressMapper.Destinations = append(b.rdmaAddressMapper.Destinations,
			messaging.RemotePort(fmt.Sprintf("GPU[%d].RDMA.RDMADataOutside", i)))
	}
}

func (b *Builder) createGPU(
	index int,
	gpuBuilder gpubuilder.GPUBuilder,
	network interDeviceNetwork,
) *gpubuilder.GPU {
	name := fmt.Sprintf("GPU[%d]", index)
	memAddrOffset := uint64(index) * b.gpuMemSize
	gpu := gpuBuilder.
		WithGPUID(uint64(index)).
		WithMemAddrOffset(memAddrOffset).
		WithRDMAAddressMapper(b.rdmaAddressMapper).
		Build(name)

	for _, port := range gpu.ExternalPorts() {
		network.BindGPUPort(index, port)
	}

	return gpu
}
