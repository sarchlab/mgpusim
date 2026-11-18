package timingconfig

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/vm/pagetablemmu"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
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

func (b *Builder) buildCPUMemory(conn *direct.Connection) {
	cpu := modeling.MakeBuilder[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).
		WithSimulation(b.simulation).WithResources(idealmemcontroller.Resources{Storage: b.globalStorage}).
		Build("CPU.Memory")
	conn.BindPort(b.buildPort(cpu, "Top", mmuTopPortBufSize))
	b.buildPort(cpu, "Control", ctrlPortBufSize)
}

func (b *Builder) buildIOMMU(conn *direct.Connection) *pagetablemmu.Comp {
	spec := b.pageTable.IOMMU
	if spec.FilterBuckets != 0 {
		panic("timingconfig: only a GPU walker can use the routing filter")
	}
	spec.PoolBase = 0
	spec.Log2PageSize = uint(b.log2PageSize)
	spec.Memory = mem.AddressMapper{Destinations: []messaging.RemotePort{cpuMemoryTop}}
	spec.FaultHandler = "Driver.PageFault[0]"
	spec.ConsultRemote = true
	io := gpubuilder.BuildPageTableMMU(b.simulation, "IOMMU", spec)
	for _, p := range []messaging.Port{
		io.Ports.Top, io.Ports.Memory, io.Ports.Control, io.Ports.PageFault, io.Ports.FaultIn,
	} {
		conn.BindPort(p)
	}
	return io
}

func pageTableTarget(c *pagetablemmu.Comp) driver.PageTableTarget {
	return driver.PageTableTarget{
		Control: c.Ports.Control.AsRemote(), PoolBase: c.Spec.PoolBase, PoolNodes: c.Spec.PoolNodes,
	}
}
