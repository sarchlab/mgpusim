// Package timingconfig contains the configuration for the timing simulation.
package timingconfig

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/mi300a"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/r9nano"
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

// WithGPUType sets the GPU type for timing simulation (r9nano or mi300a).
func (b Builder) WithGPUType(gpuType string) Builder {
	b.gpuType = gpuType
	return b
}

// Build builds the hardware platform and returns the driver. The driver, the
// GPUs, and all the connections register themselves with the simulation.
func (b Builder) Build() *driver.Driver {
	b.adjustConfigForGPUType()
	b.cpuGPUMemSizeMustEqual()

	b.globalStorage = mem.NewStorage(
		uint64(b.numGPUs)*b.gpuMemSize + b.cpuMemSize)

	mmuComp, pageTable := b.createMMU()
	gpuDriver := b.buildGPUDriver(pageTable)

	gpuBuilder := b.createGPUBuilder(mmuComp, gpuDriver)
	interDeviceConn := b.createConnection(gpuDriver, mmuComp)

	b.createGPUs(interDeviceConn, gpuBuilder, gpuDriver)

	return gpuDriver
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
		b.d2hCycles = 150    // MI300A Infinity Fabric latency (~83ns)
		b.h2dCycles = 250    // MI300A command processing (~139ns)
	default:
		// Keep defaults for r9nano
	}
}

func (b *Builder) createMMU() (*mmu.Comp, vm.PageTable) {
	pageTable := vm.NewPageTable(b.log2PageSize)

	spec := mmu.DefaultSpec()
	spec.Freq = 1 * timing.GHz
	spec.Latency = 100 // v4: page walking latency
	spec.Log2PageSize = b.log2PageSize

	mmuComponent := mmu.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("MMU")

	b.buildPort(mmuComponent, "Top", mmuTopPortBufSize)
	b.buildPort(mmuComponent, "Control", ctrlPortBufSize)

	return mmuComponent, pageTable
}

func (b *Builder) buildGPUDriver(
	pageTable vm.PageTable,
) *driver.Driver {
	spec := driver.DefaultSpec()
	spec.Log2PageSize = b.log2PageSize
	spec.UseMagicMemoryCopy = b.useMagicMemoryCopy
	spec.D2HCycles = b.d2hCycles
	spec.H2DCycles = b.h2dCycles

	gpuDriver := driver.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		WithResources(driver.Resources{
			PageTable:     pageTable,
			GlobalStorage: b.globalStorage,
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

// buildPort creates a port instance for a declared port and assigns it to
// the component.
func (b *Builder) buildPort(
	comp messaging.Component,
	name string,
	bufSize int,
) messaging.Port {
	port := modeling.MakePortBuilder().
		WithRegistrar(b.simulation).
		WithComponent(comp).
		WithSpec(modeling.PortSpec{BufSize: bufSize}).
		Build(name)
	comp.AssignPort(name, port)

	return port
}

func (b *Builder) createGPUBuilder(
	mmuComponent *mmu.Comp,
	gpuDriver *driver.Driver,
) gpubuilder.GPUBuilder {
	b.createRDMAAddressMapper()

	driverPort := gpuDriver.GetPortByName(driver.GPUPortName).AsRemote()

	switch b.gpuType {
	case "mi300a":
		return mi300a.MakeBuilder().
			WithSimulation(b.simulation).
			WithMMU(mmuComponent).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage).
			WithDriverPort(driverPort)
	default:
		return r9nano.MakeBuilder().
			WithSimulation(b.simulation).
			WithMMU(mmuComponent).
			WithLog2PageSize(b.log2PageSize).
			WithGlobalStorage(b.globalStorage).
			WithDriverPort(driverPort)
	}
}

func (b *Builder) createGPUs(
	interDeviceConn *directconnection.Comp,
	gpuBuilder gpubuilder.GPUBuilder,
	gpuDriver *driver.Driver,
) {
	for i := 1; i < b.numGPUs+1; i++ {
		b.createGPU(i, gpuBuilder, gpuDriver, interDeviceConn)
	}
}

// createConnection creates the inter-device connection that links the
// driver, the MMU, and the GPUs' external ports.
//
// NOTE(akita5): v4 used the PCIe network here. The Akita v5.0.0-beta.2
// switching network (pcie included) is traffic-only — endpoints deliver
// metadata-only packetization.AssembledMsg values instead of the original
// messages — so it cannot carry MGPUSim's protocol messages. Until upstream
// provides payload delivery, the platform uses a direct connection, which
// delivers real messages but does not model PCIe/switch latency.
func (b *Builder) createConnection(
	gpuDriver *driver.Driver,
	mmuComponent *mmu.Comp,
) *directconnection.Comp {
	conn := directconnection.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(directconnection.Spec{Freq: 1 * timing.GHz}).
		Build("InterDeviceConn")

	conn.PlugIn(gpuDriver.GetPortByName(driver.GPUPortName))
	conn.PlugIn(mmuComponent.GetPortByName("Top"))

	return conn
}

func (b *Builder) createRDMAAddressMapper() {
	b.rdmaAddressMapper = new(mem.BankedAddressPortMapper)
	b.rdmaAddressMapper.BankSize = b.gpuMemSize
	b.rdmaAddressMapper.LowModules = append(b.rdmaAddressMapper.LowModules,
		messaging.RemotePort("CPU"))
}

func (b *Builder) createGPU(
	index int,
	gpuBuilder gpubuilder.GPUBuilder,
	gpuDriver *driver.Driver,
	interDeviceConn *directconnection.Comp,
) *gpubuilder.GPU {
	name := fmt.Sprintf("GPU[%d]", index)
	memAddrOffset := uint64(index) * b.gpuMemSize
	gpu := gpuBuilder.
		WithGPUID(uint64(index)).
		WithMemAddrOffset(memAddrOffset).
		WithRDMAAddressMapper(b.rdmaAddressMapper).
		Build(name)

	gpuDriver.RegisterGPU(
		gpu.CommandProcessorPort.AsRemote(),
		driver.DeviceProperties{
			CUCount:  b.numCUPerSA * b.numSAPerGPU,
			DRAMSize: b.gpuMemSize,
		},
	)

	b.configRDMAEngine(gpu)

	for _, port := range gpu.ExternalPorts() {
		interDeviceConn.PlugIn(port)
	}

	return gpu
}

func (b *Builder) configRDMAEngine(
	gpu *gpubuilder.GPU,
) {
	b.rdmaAddressMapper.LowModules = append(
		b.rdmaAddressMapper.LowModules,
		gpu.RDMADataPort.AsRemote())
}
