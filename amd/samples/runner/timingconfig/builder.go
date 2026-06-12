// Package timingconfig contains the configuration for the timing simulation.
package timingconfig

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/timingconfig/mi300a"
	"github.com/sarchlab/mgpusim/v4/domain"
)

// Builder builds a hardware platform for timing simulation.
type Builder struct {
	simulation *simulation.Simulation

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

	platform          *domain.Domain
	globalStorage     *mem.Storage
	rdmaAddressMapper *mem.BankedAddressPortMapper
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
		gpuType:            "mi300a",
		switchLatency:      140, // default PCIe Gen4
		d2hCycles:          300,
		h2dCycles:          500,
	}
}

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

// WithGPUType sets the GPU type for timing simulation (mi300a).
func (b Builder) WithGPUType(gpuType string) Builder {
	b.gpuType = gpuType
	return b
}

// Build builds the hardware platform.
func (b Builder) Build() *domain.Domain {
	b.adjustConfigForGPUType()
	b.cpuGPUMemSizeMustEqual()

	b.platform = domain.NewDomain("")

	b.globalStorage = mem.NewStorage(
		uint64(b.numGPUs)*b.gpuMemSize + b.cpuMemSize)

	mmuComp, pageTable := b.createMMU()
	gpuDriver := b.buildGPUDriver(pageTable)

	gpuBuilder := b.createGPUBuilder(mmuComp)
	externalConn := b.createConnection(gpuDriver, mmuComp)

	// TODO: V5 migration - MigrationServiceProvider is now in mmu.Spec,
	// should be set via builder's WithMigrationServiceProvider before Build().
	// Needs refactoring to pass the driver port to the MMU builder.
	_ = mmuComp

	b.createRDMAAddrTable()
	pmcAddressTable := b.createPMCPageTable()

	b.createGPUs(
		externalConn,
		gpuBuilder, gpuDriver,
		pmcAddressTable)

	return b.platform
}

func (b *Builder) cpuGPUMemSizeMustEqual() {
	if b.cpuMemSize != b.gpuMemSize {
		panic("currently only support cpuMemSize == gpuMemSize")
	}
}

func (b *Builder) adjustConfigForGPUType() {
	switch b.gpuType {
	case "mi300a":
		b.numCUPerSA = mi300a.NumCUPerShaderArray
		b.numSAPerGPU = mi300a.NumShaderArray
		b.switchLatency = 15 // MI300A uses on-die Infinity Fabric, not PCIe
		b.d2hCycles = 150  // MI300A Infinity Fabric latency (~83ns)
		b.h2dCycles = 250  // MI300A command processing (~139ns)
	default:
		// Default to mi300a configuration
		b.numCUPerSA = mi300a.NumCUPerShaderArray
		b.numSAPerGPU = mi300a.NumShaderArray
		b.switchLatency = 15
		b.d2hCycles = 150
		b.h2dCycles = 250
	}
}

func (b *Builder) createMMU() (*modeling.Component[mmu.Spec, mmu.State], vm.PageTable) {
	pageTable := vm.NewPageTable(b.log2PageSize)
	topPort := sim.NewPort(nil, 64, 64, "MMU.Top")
	migrationPort := sim.NewPort(nil, 64, 64, "MMU.Migration")

	mmuBuilder := mmu.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(1 * sim.GHz).
		WithPageWalkingLatency(100).
		WithLog2PageSize(b.log2PageSize).
		WithPageTable(pageTable).
		WithTopPort(topPort).
		WithMigrationPort(migrationPort).
		WithAutoPageAllocation(true)

	mmuComponent := mmuBuilder.Build("MMU")

	b.simulation.RegisterComponent(mmuComponent)

	return mmuComponent, pageTable
}

func (b *Builder) buildGPUDriver(
	pageTable vm.PageTable,
) *driver.Driver {
	gpuDriverBuilder := driver.MakeBuilder()

	if b.useMagicMemoryCopy {
		gpuDriverBuilder = gpuDriverBuilder.WithMagicMemoryCopyMiddleware()
	}

	gpuDriver := gpuDriverBuilder.
		WithEngine(b.simulation.GetEngine()).
		WithPageTable(pageTable).
		WithLog2PageSize(b.log2PageSize).
		WithGlobalStorage(b.globalStorage).
		WithD2HCycles(b.d2hCycles).
		WithH2DCycles(b.h2dCycles).
		Build("Driver")

	b.simulation.RegisterComponent(gpuDriver)

	return gpuDriver
}

func (b *Builder) createGPUBuilder(
	mmuComponent *modeling.Component[mmu.Spec, mmu.State],
) gpubuilder.GPUBuilder {
	b.createRDMAAddressMapper()

	switch b.gpuType {
	case "mi300a":
		return mi300a.MakeBuilder().
			WithSimulation(b.simulation).
			WithMMU(mmuComponent).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage)
	default:
		return mi300a.MakeBuilder().
			WithSimulation(b.simulation).
			WithMMU(mmuComponent).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage)
	}
}

func (b *Builder) createGPUs(
	externalConn *directconnection.Comp,
	gpuBuilder gpubuilder.GPUBuilder,
	gpuDriver *driver.Driver,
	pmcAddressTable *mem.BankedAddressPortMapper,
) {
	for i := 1; i < b.numGPUs+1; i++ {
		b.createGPU(i, gpuBuilder, gpuDriver, pmcAddressTable,
			externalConn)
	}
}

func (b *Builder) createPMCPageTable() *mem.BankedAddressPortMapper {
	pmcAddressTable := new(mem.BankedAddressPortMapper)
	pmcAddressTable.BankSize = b.gpuMemSize
	pmcAddressTable.LowModules = append(pmcAddressTable.LowModules, "")
	return pmcAddressTable
}

func (b *Builder) createRDMAAddrTable() *mem.BankedAddressPortMapper {
	rdmaAddressTable := new(mem.BankedAddressPortMapper)
	rdmaAddressTable.BankSize = b.gpuMemSize
	rdmaAddressTable.LowModules = append(rdmaAddressTable.LowModules, "")
	return rdmaAddressTable
}

func (b *Builder) createConnection(
	gpuDriver *driver.Driver,
	mmuComponent *modeling.Component[mmu.Spec, mmu.State],
) *directconnection.Comp {
	// V5 migration: PCIe network endpoint strips message types, delivering
	// only *sim.MsgMeta to receivers. This breaks components that rely on
	// typed messages (MemCopyH2DReq, LaunchKernelReq, etc.).
	// Use a direct connection instead, which preserves full message objects.
	conn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(1 * sim.GHz).
		Build("ExternalConn")
	b.simulation.RegisterComponent(conn)

	conn.PlugIn(gpuDriver.GetPortByName("GPU"))
	conn.PlugIn(gpuDriver.GetPortByName("MMU"))
	conn.PlugIn(mmuComponent.GetPortByName("Migration"))
	conn.PlugIn(mmuComponent.GetPortByName("Top"))

	return conn
}

func (b *Builder) createRDMAAddressMapper() {
	b.rdmaAddressMapper = new(mem.BankedAddressPortMapper)
	b.rdmaAddressMapper.BankSize = b.gpuMemSize
	b.rdmaAddressMapper.LowModules = append(b.rdmaAddressMapper.LowModules,
		sim.RemotePort("CPU"))
}

func (b *Builder) createGPU(
	index int,
	gpuBuilder gpubuilder.GPUBuilder,
	gpuDriver *driver.Driver,
	pmcAddressTable *mem.BankedAddressPortMapper,
	externalConn *directconnection.Comp,
) *domain.Domain {
	name := fmt.Sprintf("GPU[%d]", index)
	memAddrOffset := uint64(index) * b.gpuMemSize
	gpu := gpuBuilder.
		WithGPUID(uint64(index)).
		WithMemAddrOffset(memAddrOffset).
		WithRDMAAddressMapper(b.rdmaAddressMapper).
		Build(name)

	gpuDriver.RegisterGPU(
		gpu.GetPortByName("CommandProcessor"),
		driver.DeviceProperties{
			CUCount:  b.numCUPerSA * b.numSAPerGPU,
			DRAMSize: b.gpuMemSize,
		},
	)

	b.configRDMAEngine(gpu)

	gpuPortMap := gpu.Ports()
	for _, p := range gpuPortMap {
		externalConn.PlugIn(p)
	}

	return gpu
}

func (b *Builder) configRDMAEngine(
	gpu *domain.Domain,
) {
	b.rdmaAddressMapper.LowModules = append(
		b.rdmaAddressMapper.LowModules,
		gpu.GetPortByName("RDMAData").AsRemote())
}

// func (b *Builder) configPMC(
// 	gpu *GPU,
// 	gpuDriver *driver.Driver,
// 	addrTable *mem.BankedAddressPortMapper,
// ) {
// 	gpu.PMC.RemotePMCAddressTable = addrTable
// 	addrTable.LowModules = append(
// 		addrTable.LowModules,
// 		gpu.PMC.GetPortByName("Remote").AsRemote())
// 	gpuDriver.RemotePMCPorts = append(
// 		gpuDriver.RemotePMCPorts, gpu.PMC.GetPortByName("Remote"))
// }
