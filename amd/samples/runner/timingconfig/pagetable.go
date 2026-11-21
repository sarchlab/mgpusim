package timingconfig

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/vm/pagetablemmu"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/waferscale"
)

// PageTableSpec configures detailed translation. PoolNodes reserves a prefix
// of CPU or GPU physical memory in 4 KiB nodes. The platform supplies the
// pool bases, page size, ports and routes. Other walker settings come from
// Akita's uncalibrated default, including the optional GPU SRAM filter.
type PageTableSpec struct {
	// Log2PageSize applies to every walker, AT, TLB and allocator.
	// Zero selects the architecture's default.
	Log2PageSize uint64
	FixedLatency bool
	GlobalGPU    bool
	GPU, IOMMU   pagetablemmu.Spec
}

func DefaultPageTableSpec() PageTableSpec {
	gpu, io := pagetablemmu.DefaultSpec(), pagetablemmu.DefaultSpec()
	gpu.PoolNodes, io.PoolNodes = 4096, 16384
	return PageTableSpec{GPU: gpu, IOMMU: io}
}

// WithPageTable configures the translation hierarchy used by all GPUs.
func (b Builder) WithPageTable(spec PageTableSpec) Builder {
	b.pageTable = spec
	return b
}

const cpuMemoryTop = messaging.RemotePort("CPU.Memory.Top")

func (b *Builder) buildCPUMemory(network interDeviceNetwork) {
	cpu := modeling.MakeBuilder[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).
		WithSimulation(b.simulation).WithResources(idealmemcontroller.Resources{Storage: b.globalStorage}).
		Build("CPU.Memory")
	network.BindHostPort(b.buildPort(cpu, "Top", mmuTopPortBufSize))
	b.buildPort(cpu, "Control", ctrlPortBufSize)
}

// buildHostFabric creates the on-chip path between the IOMMU, its page-table
// memory, and the driver, so that host-internal traffic does not cross the
// root complex. A wafer-scale GPU keeps this traffic at the host mesh position.
func (b *Builder) buildHostFabric() {
	if _, ok := waferscale.ParseGPUType(b.gpuType); ok {
		return
	}
	b.hostFabric = direct.NewConnection("HostFabric", b.simulation, timing.GHz)
}

// bindHostInternalPort plugs in a port that talks only to other host ports.
func (b *Builder) bindHostInternalPort(network interDeviceNetwork, port messaging.Port) {
	if b.hostFabric != nil {
		b.hostFabric.BindPort(port)
		return
	}
	network.BindHostPort(port)
}

// buildPageTableMemory returns where the IOMMU reads the page table. With a
// host fabric, a second controller over the same storage serves the IOMMU,
// because the Top port of the CPU memory is on the network for GPU DMA.
func (b *Builder) buildPageTableMemory() messaging.RemotePort {
	if b.hostFabric == nil {
		return cpuMemoryTop
	}
	memory := modeling.MakeBuilder[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).
		WithSimulation(b.simulation).WithResources(idealmemcontroller.Resources{Storage: b.globalStorage}).
		Build("CPU.PageTableMemory")
	top := b.buildPort(memory, "Top", mmuTopPortBufSize)
	b.buildPort(memory, "Control", ctrlPortBufSize)
	b.hostFabric.BindPort(top)
	return top.AsRemote()
}

func (b *Builder) buildIOMMU(network interDeviceNetwork) *pagetablemmu.Comp {
	b.buildHostFabric()
	spec := b.pageTable.IOMMU
	if spec.FilterBuckets != 0 {
		panic("timingconfig: only a GPU walker can use the routing filter")
	}
	spec.PoolBase = 0
	spec.Log2PageSize = uint(b.log2PageSize)
	spec.Memory = mem.AddressMapper{Destinations: []messaging.RemotePort{b.buildPageTableMemory()}}
	spec.FaultHandler = "Driver.PageFault[0]"
	spec.ConsultRemote = true
	io := gpubuilder.BuildPageTableMMU(b.simulation, "IOMMU", spec)
	for _, p := range []messaging.Port{io.Ports.Top, io.Ports.Control, io.Ports.FaultIn} {
		network.BindHostPort(p)
	}
	b.bindHostInternalPort(network, io.Ports.Memory)
	b.bindHostInternalPort(network, io.Ports.PageFault)
	return io
}

func pageTableTarget(c *pagetablemmu.Comp) driver.PageTableTarget {
	return driver.PageTableTarget{
		Control: c.Ports.Control.AsRemote(), PoolBase: c.Spec.PoolBase, PoolNodes: c.Spec.PoolNodes,
	}
}
