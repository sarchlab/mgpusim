// Package mi300a contains the configuration of GPUs similar to AMD Instinct
// MI300A.
package mi300a

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/simplebankedmemory"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/emu/cdna3"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/shaderarray"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cu"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// MI300A hardware configuration constants.
const (
	// NumCUPerShaderArray is the number of compute units per shader array.
	NumCUPerShaderArray = 6
	// NumShaderArray is the number of shader arrays in the GPU.
	NumShaderArray = 20
)

// Port buffer sizes. The CP and DMA-to-CP ports mirror the v4 4096-deep
// buffers (v4 used 40M for DMA ToCP; 4096 is plenty). The other sizes mirror
// the v4 component builders' internal port sizes.
const (
	cpPortBufSize      = 4096
	dmaToCPBufSize     = 4096
	dmaToMemBufSize    = 64
	rdmaPortBufSize    = 128
	dramTopPortBufSize = 1024 // v4: WithTopPortBufferSize(1024)
	l2TLBPortBufSize   = 1024
	ctrlPortBufSize    = 1
	l2CachePortBufSize = 128 // v4 writeback: 2 * NumReqPerCycle(64)
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
	l2BankLatency                  int
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
	drams              []*simplebankedmemory.Comp
	internalConn       *directconnection.Comp
	l2ToDramConnection *directconnection.Comp
	l1AddressMapper    *mem.InterleavedAddressPortMapper
	l1TLBAddressMapper *mem.SinglePortMapper
	dmaLocalDataSource *mem.InterleavedAddressPortMapper
}

// MakeBuilder creates a new builder with MI300A default configuration.
func MakeBuilder() Builder {
	return Builder{
		freq:                1700 * timing.MHz, // 1.70 GHz (MI300A effective clock)
		numCUPerShaderArray: NumCUPerShaderArray,
		numShaderArray:      NumShaderArray,
		l2CacheSize:         32 * mem.MB, // 32 MB L2 cache
		// L2 bank latency in cycles (MI300A L2 access ~5ns incl overhead)
		l2BankLatency:                  14,
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

// WithL2BankLatency sets the L2 cache bank latency in cycles.
func (b Builder) WithL2BankLatency(latency int) Builder {
	b.l2BankLatency = latency
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

// WithGlobalStorage sets the global storage.
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
		WithL1TLBAddressMapper(b.l1TLBAddressMapper).
		// Use the CDNA3 ALU for MI300A timing simulation.
		WithALUBuilder(func() emu.ALU { return cdna3.NewALU(nil) }).
		WithWfPoolSize(8).
		WithVGPRCount([]int{32768, 32768, 32768, 32768}).
		WithNumSinglePrecisionUnits(16).
		WithVecMemInstPipelineStages(2).
		WithVecMemTransPipelineStages(4).
		WithVecMemTransPipelineWidth(8).
		WithCUMemPipelineBufferSize(64).
		WithL1VCacheSize(32 * mem.KB).
		WithL1VBankLatency(7).
		WithMemPipelineBufferSize(64).
		WithMaxCoalescingPenalty(3).
		WithRegisterScoreboard(true)

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
	spec.NumMSHREntry = 512
	spec.NumReqPerCycle = 64
	spec.BankLatency = b.l2BankLatency
	spec.DirLatency = 2
	spec.MaxInflightFetch = 512
	spec.MaxInflightEviction = 512

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

func (b *Builder) buildDRAMControllers() {
	b.dmaLocalDataSource = mem.NewInterleavedAddressPortMapper(
		1 << b.log2MemoryBankInterleavingSize)

	memBankSize := b.dramSize / uint64(b.numMemoryBank)

	for i := 0; i < b.numMemoryBank; i++ {
		dramName := fmt.Sprintf("%s.DRAM[%d]", b.name, i)

		// Storage is global: all 16 DRAM controllers share one mem.Storage and
		// read/write it at the request's address (no storage conversion).
		//
		// The L2->DRAM mapper interleaves addresses across the 16 controllers
		// at log2MemoryBankInterleavingSize (128 B), so each controller sees a
		// strided address. The bank selector is a contiguous-bit modulo, so to
		// stripe finely across all 16 banks we first strip the inter-controller
		// interleave with the bank-selection conversion (BankAddrConv*);
		// BankSelectorLog2InterleaveSize=6 then stripes the resulting
		// controller-local address at 64 B granularity. The conversion affects
		// bank selection only — storage stays global.
		spec := simplebankedmemory.DefaultSpec()
		spec.Freq = 1 * timing.GHz
		spec.NumBanks = 16
		spec.BankPipelineWidth = 1
		spec.BankPipelineDepth = 5
		spec.StageLatency = 1
		spec.BankSelectorKind = "interleaved"
		spec.BankSelectorLog2InterleaveSize = 6
		spec.PostPipelineBufSize = 128
		spec.BankAddrConvKind = "interleaving"
		spec.BankAddrInterleavingSize = 1 << b.log2MemoryBankInterleavingSize
		spec.BankAddrTotalNumOfElements = b.numMemoryBank
		spec.BankAddrCurrentElementIndex = i
		spec.Capacity = memBankSize

		dramComp := simplebankedmemory.MakeBuilder().
			WithRegistrar(b.simulation).
			WithSpec(spec).
			WithResources(simplebankedmemory.Resources{
				Storage: b.globalStorage,
			}).
			Build(dramName)

		b.buildPort(dramComp, "Top", dramTopPortBufSize)
		b.buildPort(dramComp, "Control", ctrlPortBufSize)

		b.drams = append(b.drams, dramComp)
	}
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
	spec.ConstantKernelLaunchOverhead = 5400
	spec.SubsequentKernelLaunchOverhead = 1800
	spec.ConstantKernelOverhead = 1800

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
	spec.NumSets = 64
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
