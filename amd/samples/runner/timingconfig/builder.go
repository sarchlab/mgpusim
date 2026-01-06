// Package timingconfig contains the configuration for the timing simulation.
package timingconfig

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/mem/vm/mmu"
	"github.com/sarchlab/akita/v4/noc/networking/pcie"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/timingconfig/mi300a"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/timingconfig/r9nano"
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

	platform          *sim.Domain
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
		gpuType:            "r9nano",
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

// WithGPUType sets the GPU type for timing simulation (r9nano or mi300a).
func (b Builder) WithGPUType(gpuType string) Builder {
	b.gpuType = gpuType
	return b
}

// Build builds the hardware platform.
func (b Builder) Build() *sim.Domain {
	b.adjustConfigForGPUType()
	b.cpuGPUMemSizeMustEqual()

	b.platform = &sim.Domain{}

	b.globalStorage = mem.NewStorage(
		uint64(b.numGPUs)*b.gpuMemSize + b.cpuMemSize)

	mmuComp, pageTable := b.createMMU()
	gpuDriver := b.buildGPUDriver(pageTable)

	gpuBuilder := b.createGPUBuilder(mmuComp)
	pcieConnector, rootComplexID :=
		b.createConnection(gpuDriver, mmuComp)

	mmuComp.MigrationServiceProvider = gpuDriver.GetPortByName("MMU").AsRemote()

	b.createRDMAAddrTable()
	pmcAddressTable := b.createPMCPageTable()

	b.createGPUs(
		rootComplexID, pcieConnector,
		gpuBuilder, gpuDriver,
		pmcAddressTable)

	pcieConnector.EstablishRoute()

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
		b.numCUPerSA = 6
		b.numSAPerGPU = 20
	default:
		// Keep defaults for r9nano
	}
}

func (b *Builder) createMMU() (*mmu.Comp, vm.PageTable) {
	pageTable := vm.NewPageTable(b.log2PageSize)
	mmuBuilder := mmu.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(1 * sim.GHz).
		WithPageWalkingLatency(100).
		WithLog2PageSize(b.log2PageSize).
		WithPageTable(pageTable)

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
		WithD2HCycles(8500).
		WithH2DCycles(14500).
		Build("Driver")

	b.simulation.RegisterComponent(gpuDriver)

	return gpuDriver
}

func (b *Builder) createGPUBuilder(
	mmuComponent *mmu.Comp,
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
		return r9nano.MakeBuilder().
			WithSimulation(b.simulation).
			WithMMU(mmuComponent).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage)
	}
}

func (b *Builder) createGPUs(
	rootComplexID int,
	pcieConnector *pcie.Connector,
	gpuBuilder gpubuilder.GPUBuilder,
	gpuDriver *driver.Driver,
	pmcAddressTable *mem.BankedAddressPortMapper,
) {
	lastSwitchID := rootComplexID
	for i := 1; i < b.numGPUs+1; i++ {
		if i%2 == 1 {
			lastSwitchID = pcieConnector.AddSwitch(rootComplexID)
		}

		b.createGPU(i, gpuBuilder, gpuDriver, pmcAddressTable,
			pcieConnector, lastSwitchID)
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
	mmuComponent *mmu.Comp,
) (*pcie.Connector, int) {
	// connection := sim.NewDirectConnection(engine)
	// connection := noc.NewFixedBandwidthConnection(32, engine, 1*sim.GHz)
	// connection.SrcBufferCapacity = 40960000
	pcieConnector := pcie.NewConnector().
		WithEngine(b.simulation.GetEngine()).
		WithVersion(4, 16).
		WithSwitchLatency(140)

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
	pcieConnector *pcie.Connector,
	pcieSwitchID int,
) *sim.Domain {
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
	// gpu.CommandProcessor.Driver = gpuDriver.GetPortByName("GPU")

	b.configRDMAEngine(gpu)
	// b.configPMC(gpu, gpuDriver, pmcAddressTable)

	pcieConnector.PlugInDevice(pcieSwitchID, gpu.Ports())

	// b.gpus = append(b.gpus, gpu)

	return gpu
}

func (b *Builder) configRDMAEngine(
	gpu *sim.Domain,
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
