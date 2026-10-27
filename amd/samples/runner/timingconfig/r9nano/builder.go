// Package r9nano contains the configuration of GPUs similar to AMD Radeon R9
// Nano.
package r9nano

import (
	"fmt"

	monitoring2 "github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/shaderarray"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cu"
	"github.com/sarchlab/mgpusim/v5/amd/timing/dma"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// Port buffer sizes. The CP and DMA-to-CP ports mirror the v4 4096-deep
// buffers (v4 used 40M for DMA ToCP; 4096 is plenty). The other sizes mirror
// the v4 component builders' internal port sizes.
const (
	cpPortBufSize      = 4096
	dmaToCPBufSize     = 4096
	dmaToMemBufSize    = 64
	rdmaPortBufSize    = 128
	memCtrlPortBufSize = 16
	l2TLBPortBufSize   = 1024
	ctrlPortBufSize    = 1
	l2CachePortBufSize = 32 // v4 writeback: 2 * NumReqPerCycle(16)
)

// Builder builds a hardware platform for timing simulation.
type Builder struct {
	sampler    *sampling.SampledEngine
	simulation *simulation.Simulation
	monitor    *monitoring2.Monitor

	gpuID                          uint64
	name                           string
	freq                           timing.Freq
	numCUPerShaderArray            int
	numShaderArray                 int
	l2CacheSize                    uint64
	numMemoryBank                  int
	log2CacheLineSize              uint64
	log2PageSize                   uint64
	log2MemoryBankInterleavingSize uint64
	memAddrOffset                  uint64
	dramSize                       uint64
	globalStorage                  *mem.Storage
	mmu                            *mmu.Comp
	rdmaAddressMapper              mem.AddressMapper
	driverPort                     messaging.RemotePort

	gpu                *gpubuilder.GPU
	cp                 *cp.Comp
	rdmaEngine         *rdma.Comp
	dmaEngine          *dma.Comp
	sas                []*shaderarray.ShaderArray
	l2Caches           []*writeback.Comp
	l2TLBs             []*tlb.Comp
	drams              []*idealmemcontroller.Comp
	internalConn       *directconnection.Connection
	l2ToDramConnection *directconnection.Connection
	l1AddressMapper    *mem.AddressMapper
	l1TLBAddressMapper *mem.AddressMapper
	dmaLocalDataSource *mem.AddressMapper
}

// WithSampler shares the simulation's recoverable wavefront sampling state.
func (b Builder) WithSampler(s *sampling.SampledEngine) Builder { b.sampler = s; return b }

// MakeBuilder creates a new builder.
func MakeBuilder() Builder {
	return Builder{
		freq:                           1 * timing.GHz,
		numCUPerShaderArray:            4,
		numShaderArray:                 16,
		l2CacheSize:                    2 * mem.MB,
		numMemoryBank:                  16,
		log2CacheLineSize:              6,
		log2PageSize:                   12,
		log2MemoryBankInterleavingSize: 7,
		memAddrOffset:                  0,
		dramSize:                       4 * mem.GB,
	}
}

// WithMonitor sets the optional dispatch progress monitor.
func (b Builder) WithMonitor(m *monitoring2.Monitor) Builder { b.monitor = m; return b }

// WithSimulation sets the simulation to use.
func (b Builder) WithSimulation(sim *simulation.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithGPUID sets the GPU ID to use.
func (b Builder) WithGPUID(id uint64) gpubuilder.GPUBuilder {
	b.gpuID = id
	return b
}

// WithFreq sets the frequency that the GPU works at.
func (b Builder) WithFreq(freq timing.Freq) Builder {
	b.freq = freq
	return b
}

// WithLog2MemoryBankInterleavingSize sets the log2 memory bank interleaving
// size.
func (b Builder) WithLog2MemoryBankInterleavingSize(size uint64) Builder {
	b.log2MemoryBankInterleavingSize = size
	return b
}

// WithLog2CacheLineSize sets the log2 cache line size.
func (b Builder) WithLog2CacheLineSize(size uint64) Builder {
	b.log2CacheLineSize = size
	return b
}

// WithLog2PageSize sets the log2 page size.
func (b Builder) WithLog2PageSize(size uint64) Builder {
	b.log2PageSize = size
	return b
}

// WithMemAddrOffset sets the memory address offset.
func (b Builder) WithMemAddrOffset(offset uint64) gpubuilder.GPUBuilder {
	b.memAddrOffset = offset
	return b
}

// WithNumCUPerShaderArray sets the number of CUs per shader array.
func (b Builder) WithNumCUPerShaderArray(numCUPerShaderArray int) Builder {
	b.numCUPerShaderArray = numCUPerShaderArray
	return b
}

// WithNumShaderArray sets the number of shader arrays.
func (b Builder) WithNumShaderArray(numShaderArray int) Builder {
	b.numShaderArray = numShaderArray
	return b
}

// WithL2CacheSize sets the size of the L2 cache.
func (b Builder) WithL2CacheSize(size uint64) Builder {
	b.l2CacheSize = size
	return b
}

// WithNumMemoryBank sets the number of memory banks.
func (b Builder) WithNumMemoryBank(numMemoryBank int) Builder {
	b.numMemoryBank = numMemoryBank
	return b
}

// WithDramSize sets the size of the DRAM.
func (b Builder) WithDramSize(size uint64) Builder {
	b.dramSize = size
	return b
}

// WithMMU sets the MMU that can provide the ultimate address translation.
func (b Builder) WithMMU(mmu *mmu.Comp) Builder {
	b.mmu = mmu
	return b
}

// WithGlobalStorage sets the global storage that backs the memories of all
// the devices.
func (b Builder) WithGlobalStorage(
	globalStorage *mem.Storage,
) Builder {
	b.globalStorage = globalStorage
	return b
}

// WithRDMAAddressMapper sets the RDMA address mapper.
func (b Builder) WithRDMAAddressMapper(
	mapper mem.AddressMapper,
) gpubuilder.GPUBuilder {
	b.rdmaAddressMapper = mapper
	return b
}

// WithDriverPort sets the driver port that the command processor responds
// to.
func (b Builder) WithDriverPort(
	port messaging.RemotePort,
) gpubuilder.GPUBuilder {
	b.driverPort = port
	return b
}

// Build builds the hardware platform.
func (b Builder) Build(name string) *gpubuilder.GPU {
	b.name = name

	b.l1AddressMapper = &mem.AddressMapper{InterleavingSize: 1 << b.log2MemoryBankInterleavingSize}
	b.l1AddressMapper.RangeLow = b.memAddrOffset
	b.l1AddressMapper.RangeHigh = b.memAddrOffset + b.dramSize

	b.l1TLBAddressMapper = &mem.AddressMapper{}

	// Build order is bottom-up so the mappers the shader arrays snapshot at
	// build time (the v5 caches inline the mapper contents into their Spec)
	// are fully populated before the shader arrays are built.
	b.buildDRAMControllers()
	b.buildL2Caches()
	b.buildDMAEngine()
	b.buildRDMAEngine()
	b.buildL2TLB()
	b.buildSAs()
	b.buildCP()

	b.connectCP()
	b.connectL2AndDRAM()
	b.connectL1ToL2()
	b.connectL1TLBToL2TLB()

	b.populateGPU()

	return b.gpu
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

func (b *Builder) populateGPU() {
	b.gpu = &gpubuilder.GPU{
		Name:                 b.name,
		CommandProcessor:     b.cp,
		CommandProcessorPort: b.cp.Ports.ToDriver,
		RDMARequestPort:      b.rdmaEngine.Ports.RDMARequestOutside,
		RDMADataPort:         b.rdmaEngine.Ports.RDMADataOutside,
	}

	for _, l2TLB := range b.l2TLBs {
		b.gpu.TranslationPorts = append(b.gpu.TranslationPorts,
			l2TLB.Ports.Bottom)
	}
}

func (b *Builder) connectCP() {
	b.internalConn = directconnection.NewConnection(b.name+".InternalConn", b.simulation, b.freq)

	b.internalConn.BindPort(b.cp.Ports.ToDMA)
	b.internalConn.BindPort(b.cp.Ports.ToCaches)
	b.internalConn.BindPort(b.cp.Ports.ToCUs)
	b.internalConn.BindPort(b.cp.Ports.ToRDMA)

	rdmaCtrlPort := b.rdmaEngine.Ports.Ctrl
	b.internalConn.BindPort(rdmaCtrlPort)

	dmaToCPPort := b.dmaEngine.Ports.ToCP
	b.internalConn.BindPort(dmaToCPPort)

	b.connectCPWithCUs()
	b.connectCPWithCaches()
	b.connectCPWithDRAMControllers()
}

func (b *Builder) connectCPWithCUs() {
	for _, sa := range b.sas {
		for _, cuComp := range sa.CUs {
			b.internalConn.BindPort(cuComp.Ports.Top)
		}
	}
}

func (b *Builder) connectCPWithCaches() {
	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			ctrlPort := sa.L1VCaches[i].Ports.Control
			b.internalConn.BindPort(ctrlPort)
		}

		l1sCtrlPort := sa.L1SCache.Ports.Control
		b.internalConn.BindPort(l1sCtrlPort)

		l1iCtrlPort := sa.L1ICache.Ports.Control
		b.internalConn.BindPort(l1iCtrlPort)
	}

	for _, c := range b.l2Caches {
		ctrlPort := c.Ports.Control
		b.internalConn.BindPort(ctrlPort)
	}
}

// connectCPWithDRAMControllers binds unused DRAM control ports for lifecycle validation.
func (b *Builder) connectCPWithDRAMControllers() {
	for _, dramComp := range b.drams {
		ctrlPort := dramComp.Ports.Control
		b.internalConn.BindPort(ctrlPort)
	}
}

func (b *Builder) connectL1ToL2() {
	l1ToL2Conn := directconnection.NewConnection(b.name+".L1ToL2", b.simulation, b.freq)

	l1ToL2Conn.BindPort(b.rdmaEngine.Ports.RDMARequestInside)
	l1ToL2Conn.BindPort(b.rdmaEngine.Ports.RDMADataInside)

	for _, l2 := range b.l2Caches {
		l1ToL2Conn.BindPort(l2.Ports.Top)
	}

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			l1ToL2Conn.BindPort(sa.L1VCaches[i].Ports.Bottom)
		}

		l1ToL2Conn.BindPort(sa.L1SCache.Ports.Bottom)
		// The instruction path egress to L2 is the L1I address translator's
		// bottom port (the L1I cache sits above its AT).
		l1ToL2Conn.BindPort(sa.L1IAT.Ports.Bottom)
	}
}

func (b *Builder) connectL2AndDRAM() {
	b.l2ToDramConnection = directconnection.NewConnection(b.name+".L2ToDRAM", b.simulation, b.freq)

	for _, l2 := range b.l2Caches {
		b.l2ToDramConnection.BindPort(l2.Ports.Bottom)
	}

	for _, dramComp := range b.drams {
		topPort := dramComp.Ports.Top
		b.l2ToDramConnection.BindPort(topPort)
	}

	b.l2ToDramConnection.BindPort(b.dmaEngine.Ports.ToMem)
}

func (b *Builder) connectL1TLBToL2TLB() {
	tlbConn := directconnection.NewConnection(b.name+".L1TLBToL2TLB", b.simulation, b.freq)

	tlbConn.BindPort(b.l2TLBs[0].Ports.Top)

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			tlbConn.BindPort(sa.L1VTLBs[i].Ports.Bottom)
		}

		tlbConn.BindPort(sa.L1STLB.Ports.Bottom)
		tlbConn.BindPort(sa.L1ITLB.Ports.Bottom)
	}
}

func (b *Builder) buildSAs() {
	saBuilder := shaderarray.MakeBuilder().WithSampler(b.sampler).
		WithSimulation(b.simulation).
		WithFreq(b.freq).
		WithGPUID(b.gpuID).
		WithNumCUs(b.numCUPerShaderArray).
		WithLog2CacheLineSize(b.log2CacheLineSize).
		WithLog2PageSize(b.log2PageSize).
		WithL1AddressMapper(b.l1AddressMapper).
		WithL1TLBAddressMapper(b.l1TLBAddressMapper)

	for i := 0; i < b.numShaderArray; i++ {
		saName := fmt.Sprintf("%s.SA[%d]", b.name, i)
		sa := saBuilder.Build(saName)

		b.sas = append(b.sas, sa)
	}
}

func (b *Builder) buildL2Caches() {
	byteSize := b.l2CacheSize / uint64(b.numMemoryBank)

	spec := writeback.DefaultSpec()
	spec.Freq = b.freq
	spec.Log2BlockSize = b.log2CacheLineSize
	spec.NumWays = 16
	spec.NumSets = int((byteSize) / (uint64(spec.NumWays) * (1 << spec.Log2BlockSize)))
	spec.NumMSHREntries = 64
	spec.NumReqPerCycle = 16
	spec.DirectoryWidth = spec.NumReqPerCycle
	spec.BankWidth = spec.NumReqPerCycle

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.L2Cache[%d]", b.name, i)
		spec.Bottom = mem.AddressMapper{Destinations: []messaging.RemotePort{b.drams[i].Ports.Top.AsRemote()}}
		l2 := modeling.MakeBuilder[*writeback.Comp](spec).WithSimulation(b.simulation).
			WithResources(writeback.Resources{
				Storage: mem.MakeStorageBuilder().
					WithSimulation(b.simulation).
					WithCapacity(spec.ByteSize()).
					Build(cacheName + ".Storage"),
			}).
			Build(cacheName)

		b.buildPort(l2, "Top", l2CachePortBufSize)
		b.buildPort(l2, "Bottom", l2CachePortBufSize)
		b.buildPort(l2, "Control", l2CachePortBufSize)

		b.l2Caches = append(b.l2Caches, l2)

		b.l1AddressMapper.Destinations = append(
			b.l1AddressMapper.Destinations,
			l2.Ports.Top.AsRemote(),
		)
	}
}

// buildDRAMControllers builds the memory controllers. Like the v4
// configuration, the active model is the ideal memory controller with a
// fixed 100-cycle latency; hbmDRAMSpec provides the detailed
// HBM timing model for experiments (v4 kept the same alternative as dead
// code in createDramControllerBuilder).
func (b *Builder) buildDRAMControllers() {
	b.dmaLocalDataSource = &mem.AddressMapper{InterleavingSize: 1 << b.log2MemoryBankInterleavingSize}

	spec := idealmemcontroller.DefaultSpec()
	spec.Freq = b.freq
	spec.Latency = 100

	for i := 0; i < b.numMemoryBank; i++ {
		dramName := fmt.Sprintf("%s.DRAM[%d]", b.name, i)
		dramComp := modeling.MakeBuilder[*idealmemcontroller.Comp](spec).WithSimulation(b.simulation).
			WithResources(idealmemcontroller.Resources{
				Storage: b.globalStorage,
			}).
			Build(dramName)

		b.buildPort(dramComp, "Top", memCtrlPortBufSize)
		b.buildPort(dramComp, "Control", memCtrlPortBufSize)

		b.drams = append(b.drams, dramComp)
		b.dmaLocalDataSource.Destinations = append(b.dmaLocalDataSource.Destinations, dramComp.Ports.Top.AsRemote())
	}
}

// hbmDRAMSpec returns the spec of a detailed HBM memory controller that
// matches the v4 dram.HBM configuration. It starts from the v5 HBM2Spec
// preset (the closest preset to v4's dram.HBM protocol) and applies the
// exact geometry and timing numbers the v4 configuration used. It is not
// used by default (the v4 configuration used the ideal memory controller),
// but is kept so the detailed model can be swapped in.
//
//nolint:unused
func (b *Builder) hbmDRAMSpec() dram.Spec {
	memBankSize := 4 * mem.GB / uint64(b.numMemoryBank)
	if 4*mem.GB%uint64(b.numMemoryBank) != 0 {
		panic("GPU memory size is not a multiple of the number of memory banks")
	}

	dramCol := 64
	dramRow := 16384
	dramDeviceWidth := 128
	dramBankSize := dramCol * dramRow * dramDeviceWidth
	dramBank := 4
	dramBankGroup := 4
	dramBusWidth := 256
	dramDevicePerRank := dramBusWidth / dramDeviceWidth
	dramRankSize := dramBankSize * dramDevicePerRank * dramBank
	dramRank := int(memBankSize * 8 / uint64(dramRankSize))

	spec := dram.HBM2Spec()
	spec.Freq = 500 * timing.MHz
	spec.BurstLength = 4
	spec.DeviceWidth = dramDeviceWidth
	spec.BusWidth = dramBusWidth
	spec.NumChannel = 1
	spec.NumRank = dramRank
	spec.NumBankGroup = dramBankGroup
	spec.NumBank = dramBank
	spec.NumCol = dramCol
	spec.NumRow = dramRow
	spec.CommandQueueCapacity = 8
	spec.TransactionQueueSize = 32
	applyHBMTimings(&spec)

	return spec
}

// applyHBMTimings applies the v4 dram.HBM timing parameters onto spec.
//
//nolint:unused
func applyHBMTimings(spec *dram.Spec) {
	spec.TCL = 7
	spec.TCWL = 2
	spec.TRCDRD = 7
	spec.TRCDWR = 7
	spec.TRP = 7
	spec.TRAS = 17
	spec.TREFI = 1950
	spec.TRRDS = 2
	spec.TRRDL = 3
	spec.TWTRS = 3
	spec.TWTRL = 4
	spec.TWR = 8
	spec.TCCDS = 1
	spec.TCCDL = 1
	spec.TRTRS = 0
	spec.TRTP = 3
	spec.TPPD = 2
}

func (b *Builder) buildRDMAEngine() {
	name := fmt.Sprintf("%s.RDMA", b.name)

	spec := rdma.DefaultSpec()
	spec.Freq = 1 * timing.GHz
	b.l1AddressMapper.Fallback = messaging.RemotePort(name + ".RDMARequestInside")
	spec.LocalModules = *b.l1AddressMapper
	spec.RemoteRDMAAddressTable = b.rdmaAddressMapper

	b.rdmaEngine = rdma.MakeBuilder().
		WithSimulation(b.simulation).
		WithSpec(spec).
		Build(name)

	b.buildPort(b.rdmaEngine, "RDMARequestInside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "RDMARequestOutside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "RDMADataInside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "RDMADataOutside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "Ctrl", rdmaPortBufSize)
}

func (b *Builder) buildDMAEngine() {
	spec := dma.DefaultSpec()
	spec.LocalDataSource = *b.dmaLocalDataSource
	b.dmaEngine = dma.MakeBuilder().
		WithSimulation(b.simulation).
		WithSpec(spec).
		Build(fmt.Sprintf("%s.DMA", b.name))

	b.buildPort(b.dmaEngine, "ToCP", dmaToCPBufSize)
	b.buildPort(b.dmaEngine, "ToMem", dmaToMemBufSize)
}

func (b *Builder) buildCP() {
	spec := cp.DefaultSpec()
	spec.Freq = b.freq

	spec.Driver = b.driverPort
	spec.DMAEngine = b.dmaEngine.Ports.ToCP.AsRemote()
	spec.RDMA = b.rdmaEngine.Ports.Ctrl.AsRemote()
	for _, sa := range b.sas {
		for _, c := range sa.CUs {
			spec.CUs = append(spec.CUs, cp.DescribeCU(cu.DispatcherView{CU: c}))
		}
		for _, c := range sa.L1VCaches {
			spec.L1VCaches = append(spec.L1VCaches, c.Ports.Control.AsRemote())
		}
		spec.L1SCaches = append(spec.L1SCaches, sa.L1SCache.Ports.Control.AsRemote())
		spec.L1ICaches = append(spec.L1ICaches, sa.L1ICache.Ports.Control.AsRemote())
	}
	for _, c := range b.l2Caches {
		spec.L2Caches = append(spec.L2Caches, c.Ports.Control.AsRemote())
	}

	b.cp = cp.MakeBuilder().WithSampler(b.sampler).
		WithSimulation(b.simulation).
		WithSpec(spec).
		WithMonitor(b.monitor).
		Build(b.name + ".CommandProcessor")

	b.buildPort(b.cp, "ToDriver", cpPortBufSize)
	b.buildPort(b.cp, "ToDMA", cpPortBufSize)
	b.buildPort(b.cp, "ToCUs", cpPortBufSize)
	b.buildPort(b.cp, "ToCaches", cpPortBufSize)
	b.buildPort(b.cp, "ToRDMA", cpPortBufSize)
}

func (b *Builder) buildL2TLB() {
	numWays := 64

	spec := tlb.DefaultSpec()
	spec.Freq = b.freq
	spec.NumWays = numWays
	spec.NumSets = int(b.dramSize / (1 << b.log2PageSize) / uint64(numWays))
	spec.NumMSHREntries = 64
	spec.NumReqPerCycle = 1024
	spec.Log2PageSize = b.log2PageSize

	spec.Bottom = mem.AddressMapper{Destinations: []messaging.RemotePort{b.mmu.Ports.Top.AsRemote()}}
	l2TLB := modeling.MakeBuilder[*tlb.Comp](spec).WithSimulation(b.simulation).
		Build(fmt.Sprintf("%s.L2TLB", b.name))

	b.buildPort(l2TLB, "Top", l2TLBPortBufSize)
	b.buildPort(l2TLB, "Bottom", l2TLBPortBufSize)
	b.buildPort(l2TLB, "Control", ctrlPortBufSize)

	b.l2TLBs = append(b.l2TLBs, l2TLB)

	b.l1TLBAddressMapper.Destinations = []messaging.RemotePort{l2TLB.Ports.Top.AsRemote()}
}
