// Package r9nano contains the configuration of GPUs similar to AMD Radeon R9
// Nano.
package r9nano

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/dram"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/shaderarray"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cu"
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
	simulation *simulation.Simulation

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
	rdmaAddressMapper              mem.AddressToPortMapper
	driverPort                     messaging.RemotePort

	gpu                *gpubuilder.GPU
	cp                 *cp.Comp
	rdmaEngine         *rdma.Comp
	dmaEngine          *cp.DMAComp
	sas                []*shaderarray.ShaderArray
	l2Caches           []*writeback.Comp
	l2TLBs             []*tlb.Comp
	drams              []messaging.Component
	internalConn       *directconnection.Comp
	l2ToDramConnection *directconnection.Comp
	l1AddressMapper    *mem.InterleavedAddressPortMapper
	l1TLBAddressMapper *mem.SinglePortMapper
	dmaLocalDataSource *mem.InterleavedAddressPortMapper
}

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
	mapper mem.AddressToPortMapper,
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

	b.l1AddressMapper = mem.NewInterleavedAddressPortMapper(
		1 << b.log2MemoryBankInterleavingSize,
	)
	b.l1AddressMapper.LowAddress = b.memAddrOffset
	b.l1AddressMapper.HighAddress = b.memAddrOffset + b.dramSize
	b.l1AddressMapper.UseAddressSpaceLimitation = true

	b.l1TLBAddressMapper = &mem.SinglePortMapper{}

	// Build order is bottom-up so the mappers the shader arrays snapshot at
	// build time (the v5 caches inline the mapper contents into their Spec)
	// are fully populated before the shader arrays are built.
	b.buildDRAMControllers()
	b.buildL2Caches()
	b.buildCP()
	b.buildL2TLB()
	b.buildSAs()

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

func (b *Builder) populateGPU() {
	b.gpu = &gpubuilder.GPU{
		Name:                 b.name,
		CommandProcessor:     b.cp,
		CommandProcessorPort: b.cp.GetPortByName("ToDriver"),
		RDMARequestPort:      b.rdmaEngine.GetPortByName("RDMARequestOutside"),
		RDMADataPort:         b.rdmaEngine.GetPortByName("RDMADataOutside"),
	}

	for _, l2TLB := range b.l2TLBs {
		b.gpu.TranslationPorts = append(b.gpu.TranslationPorts,
			l2TLB.GetPortByName("Bottom"))
	}
}

func (b *Builder) connectCP() {
	b.internalConn = directconnection.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(b.name + ".InternalConn")

	b.internalConn.PlugIn(b.cp.GetPortByName("ToDMA"))
	b.internalConn.PlugIn(b.cp.GetPortByName("ToCaches"))
	b.internalConn.PlugIn(b.cp.GetPortByName("ToCUs"))
	b.internalConn.PlugIn(b.cp.GetPortByName("ToTLBs"))
	b.internalConn.PlugIn(b.cp.GetPortByName("ToAddressTranslators"))
	b.internalConn.PlugIn(b.cp.GetPortByName("ToRDMA"))

	rdmaCtrlPort := b.rdmaEngine.GetPortByName("Ctrl")
	b.cp.State.RDMA = rdmaCtrlPort.AsRemote()
	b.internalConn.PlugIn(rdmaCtrlPort)

	dmaToCPPort := b.dmaEngine.GetPortByName("ToCP")
	b.cp.State.DMAEngine = dmaToCPPort.AsRemote()
	b.internalConn.PlugIn(dmaToCPPort)

	b.connectCPWithCUs()
	b.connectCPWithAddressTranslators()
	b.connectCPWithTLBs()
	b.connectCPWithCaches()
	b.connectCPWithDRAMControllers()
}

func (b *Builder) connectCPWithCUs() {
	for _, sa := range b.sas {
		for _, cuComp := range sa.CUs {
			cp.RegisterCU(b.cp, cu.DispatcherView{CU: cuComp})

			b.internalConn.PlugIn(cuComp.GetPortByName(cu.DispatchPortName))
			b.internalConn.PlugIn(cuComp.GetPortByName(cu.CtrlPortName))
		}
	}
}

// connectCPWithAddressTranslators wires the Control ports of the address
// translators and the reorder buffers to the CP. The ROB list is new in v5:
// the CP requires the ROB control ports split from the AT control ports
// (v4 mixed them into the AT list).
func (b *Builder) connectCPWithAddressTranslators() {
	addAT := func(at messaging.Component) {
		ctrlPort := at.GetPortByName("Control")
		b.cp.State.AddressTranslators = append(
			b.cp.State.AddressTranslators, ctrlPort.AsRemote())
		b.internalConn.PlugIn(ctrlPort)
	}
	addROB := func(robComp messaging.Component) {
		ctrlPort := robComp.GetPortByName("Control")
		b.cp.State.ROBs = append(b.cp.State.ROBs, ctrlPort.AsRemote())
		b.internalConn.PlugIn(ctrlPort)
	}

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			addAT(sa.L1VATs[i])
			addROB(sa.L1VROBs[i])
		}

		addAT(sa.L1SAT)
		addROB(sa.L1SROB)

		addAT(sa.L1IAT)
		addROB(sa.L1IROB)
	}
}

func (b *Builder) connectCPWithTLBs() {
	addTLB := func(tlbComp messaging.Component) {
		ctrlPort := tlbComp.GetPortByName("Control")
		b.cp.State.TLBs = append(b.cp.State.TLBs, ctrlPort.AsRemote())
		b.internalConn.PlugIn(ctrlPort)
	}

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			addTLB(sa.L1VTLBs[i])
		}

		addTLB(sa.L1STLB)
		addTLB(sa.L1ITLB)
	}

	for _, l2TLB := range b.l2TLBs {
		addTLB(l2TLB)
	}
}

func (b *Builder) connectCPWithCaches() {
	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			ctrlPort := sa.L1VCaches[i].GetPortByName("Control")
			b.cp.State.L1VCaches = append(
				b.cp.State.L1VCaches, ctrlPort.AsRemote())
			b.internalConn.PlugIn(ctrlPort)
		}

		l1sCtrlPort := sa.L1SCache.GetPortByName("Control")
		b.cp.State.L1SCaches = append(
			b.cp.State.L1SCaches, l1sCtrlPort.AsRemote())
		b.internalConn.PlugIn(l1sCtrlPort)

		l1iCtrlPort := sa.L1ICache.GetPortByName("Control")
		b.cp.State.L1ICaches = append(
			b.cp.State.L1ICaches, l1iCtrlPort.AsRemote())
		b.internalConn.PlugIn(l1iCtrlPort)
	}

	for _, c := range b.l2Caches {
		ctrlPort := c.GetPortByName("Control")
		b.cp.State.L2Caches = append(b.cp.State.L2Caches, ctrlPort.AsRemote())
		b.internalConn.PlugIn(ctrlPort)
	}
}

// connectCPWithDRAMControllers records the Control ports of the DRAM
// controllers in the CP state. The CP currently sends no commands to them,
// but the ports must be connected so the control protocol is reachable.
func (b *Builder) connectCPWithDRAMControllers() {
	for _, dramComp := range b.drams {
		ctrlPort := dramComp.GetPortByName("Control")
		b.cp.State.DRAMControllers = append(
			b.cp.State.DRAMControllers, ctrlPort.AsRemote())
		b.internalConn.PlugIn(ctrlPort)
	}
}

func (b *Builder) connectL1ToL2() {
	l1ToL2Conn := directconnection.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(b.name + ".L1ToL2")

	l1ToL2Conn.PlugIn(b.rdmaEngine.GetPortByName("RDMARequestInside"))
	l1ToL2Conn.PlugIn(b.rdmaEngine.GetPortByName("RDMADataInside"))

	for _, l2 := range b.l2Caches {
		l1ToL2Conn.PlugIn(l2.GetPortByName("Top"))
	}

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			l1ToL2Conn.PlugIn(sa.L1VCaches[i].GetPortByName("Bottom"))
		}

		l1ToL2Conn.PlugIn(sa.L1SCache.GetPortByName("Bottom"))
		// The instruction path egress to L2 is the L1I address translator's
		// bottom port (the L1I cache sits above its AT).
		l1ToL2Conn.PlugIn(sa.L1IAT.GetPortByName("Bottom"))
	}
}

func (b *Builder) connectL2AndDRAM() {
	b.l2ToDramConnection = directconnection.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(b.name + ".L2ToDRAM")

	for _, l2 := range b.l2Caches {
		b.l2ToDramConnection.PlugIn(l2.GetPortByName("Bottom"))
	}

	for _, dramComp := range b.drams {
		topPort := dramComp.GetPortByName("Top")
		b.l2ToDramConnection.PlugIn(topPort)
		b.dmaLocalDataSource.LowModules = append(
			b.dmaLocalDataSource.LowModules, topPort.AsRemote())
	}

	b.l2ToDramConnection.PlugIn(b.dmaEngine.GetPortByName("ToMem"))
}

func (b *Builder) connectL1TLBToL2TLB() {
	tlbConn := directconnection.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(b.name + ".L1TLBToL2TLB")

	tlbConn.PlugIn(b.l2TLBs[0].GetPortByName("Top"))

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			tlbConn.PlugIn(sa.L1VTLBs[i].GetPortByName("Bottom"))
		}

		tlbConn.PlugIn(sa.L1STLB.GetPortByName("Bottom"))
		tlbConn.PlugIn(sa.L1ITLB.GetPortByName("Bottom"))
	}
}

func (b *Builder) buildSAs() {
	saBuilder := shaderarray.MakeBuilder().
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
	spec.WayAssociativity = 16
	spec.TotalByteSize = byteSize
	spec.NumMSHREntry = 64
	spec.NumReqPerCycle = 16

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.L2Cache[%d]", b.name, i)
		l2 := writeback.MakeBuilder().
			WithRegistrar(b.simulation).
			WithSpec(spec).
			WithResources(writeback.Resources{
				AddressToPortMapper: &mem.SinglePortMapper{
					Port: b.drams[i].GetPortByName("Top").AsRemote(),
				},
			}).
			Build(cacheName)

		b.buildPort(l2, "Top", l2CachePortBufSize)
		b.buildPort(l2, "Bottom", l2CachePortBufSize)
		b.buildPort(l2, "Control", l2CachePortBufSize)

		b.l2Caches = append(b.l2Caches, l2)

		b.l1AddressMapper.LowModules = append(
			b.l1AddressMapper.LowModules,
			l2.GetPortByName("Top").AsRemote(),
		)
	}
}

// buildDRAMControllers builds the memory controllers. Like the v4
// configuration, the active model is the ideal memory controller with a
// fixed 100-cycle latency; hbmDRAMSpec provides the detailed
// HBM timing model for experiments (v4 kept the same alternative as dead
// code in createDramControllerBuilder).
func (b *Builder) buildDRAMControllers() {
	b.dmaLocalDataSource = mem.NewInterleavedAddressPortMapper(
		1 << b.log2MemoryBankInterleavingSize)

	spec := idealmemcontroller.DefaultSpec()
	spec.Freq = b.freq
	spec.Latency = 100

	for i := 0; i < b.numMemoryBank; i++ {
		dramName := fmt.Sprintf("%s.DRAM[%d]", b.name, i)
		dramComp := idealmemcontroller.MakeBuilder().
			WithRegistrar(b.simulation).
			WithSpec(spec).
			WithResources(idealmemcontroller.Resources{
				Storage: b.globalStorage,
			}).
			Build(dramName)

		b.buildPort(dramComp, "Top", memCtrlPortBufSize)
		b.buildPort(dramComp, "Control", memCtrlPortBufSize)

		b.drams = append(b.drams, dramComp)
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

	spec := dram.HBM2Spec
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

	b.rdmaEngine = rdma.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		WithResources(rdma.Resources{
			LocalModules:           b.l1AddressMapper,
			RemoteRDMAAddressTable: b.rdmaAddressMapper,
		}).
		Build(name)

	b.buildPort(b.rdmaEngine, "RDMARequestInside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "RDMARequestOutside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "RDMADataInside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "RDMADataOutside", rdmaPortBufSize)
	b.buildPort(b.rdmaEngine, "Ctrl", rdmaPortBufSize)

	b.l1AddressMapper.ModuleForOtherAddresses =
		b.rdmaEngine.GetPortByName("RDMARequestInside").AsRemote()
}

func (b *Builder) buildDMAEngine() {
	b.dmaEngine = cp.MakeDMAEngineBuilder().
		WithRegistrar(b.simulation).
		WithSpec(cp.DefaultDMASpec()).
		WithResources(cp.DMAResources{
			LocalDataSource: b.dmaLocalDataSource,
		}).
		Build(fmt.Sprintf("%s.DMA", b.name))

	b.buildPort(b.dmaEngine, "ToCP", dmaToCPBufSize)
	b.buildPort(b.dmaEngine, "ToMem", dmaToMemBufSize)
}

func (b *Builder) buildCP() {
	spec := cp.DefaultSpec()
	spec.Freq = b.freq

	b.cp = cp.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		WithVisTracer(b.simulation.GetVisTracer()).
		WithMonitor(b.simulation.GetMonitor()).
		WithDriver(b.driverPort).
		Build(b.name + ".CommandProcessor")

	b.buildPort(b.cp, "ToDriver", cpPortBufSize)
	b.buildPort(b.cp, "ToDMA", cpPortBufSize)
	b.buildPort(b.cp, "ToCUs", cpPortBufSize)
	b.buildPort(b.cp, "ToTLBs", cpPortBufSize)
	b.buildPort(b.cp, "ToAddressTranslators", cpPortBufSize)
	b.buildPort(b.cp, "ToCaches", cpPortBufSize)
	b.buildPort(b.cp, "ToRDMA", cpPortBufSize)

	b.buildDMAEngine()
	b.buildRDMAEngine()
}

func (b *Builder) buildL2TLB() {
	numWays := 64

	spec := tlb.DefaultSpec()
	spec.Freq = b.freq
	spec.NumWays = numWays
	spec.NumSets = int(b.dramSize / (1 << b.log2PageSize) / uint64(numWays))
	spec.MSHRSize = 64
	spec.NumReqPerCycle = 1024
	spec.Log2PageSize = b.log2PageSize

	l2TLB := tlb.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: b.mmu.GetPortByName("Top").AsRemote(),
			},
		}).
		Build(fmt.Sprintf("%s.L2TLB", b.name))

	b.buildPort(l2TLB, "Top", l2TLBPortBufSize)
	b.buildPort(l2TLB, "Bottom", l2TLBPortBufSize)
	b.buildPort(l2TLB, "Control", ctrlPortBufSize)

	b.l2TLBs = append(b.l2TLBs, l2TLB)

	b.l1TLBAddressMapper.Port = l2TLB.GetPortByName("Top").AsRemote()
}
