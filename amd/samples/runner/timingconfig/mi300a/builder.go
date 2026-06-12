// Package mi300a contains the configuration of GPUs similar to AMD Instinct
// MI300A.
package mi300a

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writeback"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/emu/cdna3"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/timingconfig/shaderarray"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v4/amd/timing/latencyconn"
	"github.com/sarchlab/mgpusim/v4/amd/timing/mem/simplebankedmemory"
	"github.com/sarchlab/mgpusim/v4/amd/timing/pagemigrationcontroller"
	"github.com/sarchlab/mgpusim/v4/amd/timing/rdma"
	"github.com/sarchlab/mgpusim/v4/domain"
)

// MI300A hardware configuration constants.
const (
	// NumCUPerShaderArray is the number of compute units per shader array.
	NumCUPerShaderArray = 6
	// NumShaderArray is the number of shader arrays in the GPU.
	NumShaderArray = 36
	// NumXCDGroups is the number of XCD groups in the GPU.
	NumXCDGroups = 6
	// NumSAPerXCD is the number of shader arrays per XCD group.
	NumSAPerXCD = NumShaderArray / NumXCDGroups // 6
	// NumL2BanksPerXCD is the number of L2 cache banks per XCD group.
	NumL2BanksPerXCD = 4
)

// Builder builds a hardware platform for timing simulation.
type Builder struct {
	simulation *simulation.Simulation

	gpuID                          uint64
	name                           string
	freq                           sim.Freq
	wfPoolSize                     int
	numCUPerShaderArray            int
	numShaderArray                 int
	l2CacheSize                    uint64
	l2BankLatency                  int
	l1ToL2Latency                  int
	numMemoryBank                  int
	log2CacheLineSize              uint64
	log2PageSize                   uint64
	log2MemoryBankInterleavingSize uint64
	memAddrOffset                  uint64
	dramSize                       uint64
	globalStorage                  *mem.Storage
	mmu                            *modeling.Component[mmu.Spec, mmu.State]
	rdmaAddressMapper              mem.AddressToPortMapper

	gpu                   *domain.Domain
	cp                    *cp.CommandProcessor
	rdmaEngine            *rdma.Comp
	pmc                   *pagemigrationcontroller.PageMigrationController
	dmaEngine             *cp.DMAEngine
	sas                   []*domain.Domain
	l2Caches              []*modeling.Component[writeback.Spec, writeback.State]
	l2TLBs                []*modeling.Component[tlb.Spec, tlb.State]
	drams                 []sim.Component
	internalConn          *directconnection.Comp
	l2ToDramConnection    *latencyconn.Comp
	l1AddressMappers      [NumXCDGroups]*mem.InterleavedAddressPortMapper
	globalL1AddressMapper *mem.InterleavedAddressPortMapper
	l1TLBAddressMapper    *mem.SinglePortMapper
	pmcAddressMapper      mem.AddressToPortMapper
}

// MakeBuilder creates a new builder with MI300A default configuration.
func MakeBuilder() Builder {
	return Builder{
		freq:                           2100 * sim.MHz, // 2.1 GHz (MI300A peak engine clock)
		wfPoolSize:                     64,             // MI300A: 64 wavefronts per SIMD (4× increase for latency hiding)
		numCUPerShaderArray:            NumCUPerShaderArray,
		numShaderArray:                 NumShaderArray,
		l2CacheSize:                    24 * mem.MB, // 24 MB L2 cache (4 MB per XCD x 6 XCDs)
		l2BankLatency:                  1,           // Calibrated L2 bank access latency in cycles
		l1ToL2Latency:                  8,           // CDNA3: L1→L2 hop latency
		numMemoryBank:                  24,
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
func (b Builder) WithFreq(freq sim.Freq) Builder {
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

// WithL1ToL2Latency sets the per-hop latency in cycles for the L1→L2
// interconnect. Each message traverses the connection once in each direction,
// so the round-trip penalty is 2× this value.
func (b Builder) WithL1ToL2Latency(latency int) Builder {
	b.l1ToL2Latency = latency
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
func (b Builder) WithMMU(mmu *modeling.Component[mmu.Spec, mmu.State]) Builder {
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

// Build builds the hardware platform.
func (b Builder) Build(name string) *domain.Domain {
	b.name = name
	b.gpu = domain.NewDomain(name)

	// Create per-XCD L1 address mappers (each routes to 4 local L2 banks)
	for g := range NumXCDGroups {
		m := mem.NewInterleavedAddressPortMapper(
			1 << b.log2MemoryBankInterleavingSize,
		)
		m.LowAddress = b.memAddrOffset
		m.HighAddress = b.memAddrOffset + b.dramSize
		m.UseAddressSpaceLimitation = true
		b.l1AddressMappers[g] = m
	}

	// Global mapper for RDMA (routes to all 16 L2 banks)
	b.globalL1AddressMapper = mem.NewInterleavedAddressPortMapper(
		1 << b.log2MemoryBankInterleavingSize,
	)
	b.globalL1AddressMapper.LowAddress = b.memAddrOffset
	b.globalL1AddressMapper.HighAddress = b.memAddrOffset + b.dramSize
	b.globalL1AddressMapper.UseAddressSpaceLimitation = true

	b.l1TLBAddressMapper = &mem.SinglePortMapper{}

	// Build DRAMs and L2 caches before SAs so that the L1→L2 address
	// mapper (b.l1AddressMapper) is populated at the time the L1 caches
	// are built.  V5 resolves mapper specs at build time.
	b.buildDRAMControllers()
	b.buildL2Caches()
	b.buildCP()
	b.buildL2TLB()
	b.buildSAs()

	b.connectCP()
	b.connectL2AndDRAM()
	b.connectL1ToL2()
	b.connectL1TLBToL2TLB()

	b.populateExternalPorts()

	return b.gpu
}

func (b *Builder) populateExternalPorts() {
	b.gpu.AddPort("CommandProcessor", b.cp.ToDriver)
	b.gpu.AddPort("RDMARequest", b.rdmaEngine.RDMARequestOutside)
	b.gpu.AddPort("RDMAData", b.rdmaEngine.RDMADataOutside)

	b.gpu.AddPort("PageMigrationController",
		b.pmc.GetPortByName("Remote"))

	for i, l2TLB := range b.l2TLBs {
		name := fmt.Sprintf("Translation_%02d", i)
		b.gpu.AddPort(name, l2TLB.GetPortByName("Bottom"))
	}
}

func (b *Builder) connectCP() {
	b.internalConn = directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(b.name + ".InternalConn")
	b.simulation.RegisterComponent(b.internalConn)

	b.internalConn.PlugIn(b.cp.ToDMA)
	b.internalConn.PlugIn(b.cp.ToCaches)
	b.internalConn.PlugIn(b.cp.ToCUs)
	b.internalConn.PlugIn(b.cp.ToTLBs)
	b.internalConn.PlugIn(b.cp.ToAddressTranslators)
	b.internalConn.PlugIn(b.cp.ToRDMA)
	b.internalConn.PlugIn(b.cp.ToPMC)

	b.cp.RDMA = b.rdmaEngine.CtrlPort
	b.internalConn.PlugIn(b.cp.RDMA)

	b.cp.DMAEngine = b.dmaEngine.ToCP
	b.internalConn.PlugIn(b.dmaEngine.ToCP)

	pmcControlPort := b.pmc.GetPortByName("Control")
	b.cp.PMC = pmcControlPort
	b.internalConn.PlugIn(pmcControlPort)

	b.connectCPWithCUs()
	b.connectCPWithAddressTranslators()
	b.connectCPWithTLBs()
	b.connectCPWithCaches()
}

func (b *Builder) connectL1ToL2() {
	l1ToL2Conn := latencyconn.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithLatency(b.l1ToL2Latency).
		WithCrossGroupPenalty(1). // reduced from 150
		Build(b.name + ".L1ToL2")
	b.simulation.RegisterComponent(l1ToL2Conn)

	// RDMA uses global mapper (all 16 L2 banks)
	b.rdmaEngine.SetLocalModuleFinder(b.globalL1AddressMapper)
	b.globalL1AddressMapper.ModuleForOtherAddresses = b.rdmaEngine.RDMARequestInside.AsRemote()
	l1ToL2Conn.PlugIn(b.rdmaEngine.RDMARequestInside)
	l1ToL2Conn.PlugIn(b.rdmaEngine.RDMADataInside)

	// Set RDMA fallback on each per-XCD mapper
	for g := range NumXCDGroups {
		b.l1AddressMappers[g].ModuleForOtherAddresses = b.rdmaEngine.RDMARequestInside.AsRemote()
	}

	// Plug in all L2 top ports and assign XCD groups
	for i, l2 := range b.l2Caches {
		port := l2.GetPortByName("Top")
		l1ToL2Conn.PlugIn(port)
		l1ToL2Conn.SetPortXCDGroup(port, i/NumL2BanksPerXCD)
	}

	// Plug in all SA L1 ports and assign XCD groups
	for saIdx, sa := range b.sas {
		xcdGroup := saIdx / NumSAPerXCD
		for i := range b.numCUPerShaderArray {
			port := sa.GetPortByName(fmt.Sprintf("L1VCacheBottom[%d]", i))
			l1ToL2Conn.PlugIn(port)
			l1ToL2Conn.SetPortXCDGroup(port, xcdGroup)
		}
		l1sPort := sa.GetPortByName("L1SCacheBottom")
		l1ToL2Conn.PlugIn(l1sPort)
		l1ToL2Conn.SetPortXCDGroup(l1sPort, xcdGroup)

		l1iPort := sa.GetPortByName("L1ICacheBottom")
		l1ToL2Conn.PlugIn(l1iPort)
		l1ToL2Conn.SetPortXCDGroup(l1iPort, xcdGroup)
	}
}

func (b *Builder) connectL2AndDRAM() {
	b.l2ToDramConnection = latencyconn.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithLatency(1). // reduced from 5
		Build(b.name + ".L2ToDRAM")
	b.simulation.RegisterComponent(b.l2ToDramConnection)

	lowModuleFinder := mem.NewInterleavedAddressPortMapper(
		1 << b.log2MemoryBankInterleavingSize)

	for i, l2 := range b.l2Caches {
		b.l2ToDramConnection.PlugIn(l2.GetPortByName("Bottom"))
		// TODO: V5 migration - SetAddressToPortMapper no longer exists on
		// modeling.Component. Address mapper must be configured at build time
		// via the builder's WithAddressToPortMapper. Needs refactoring to pass
		// the mapper before building each L2 cache.
		_ = i
		_ = l2
	}

	for _, dram := range b.drams {
		b.l2ToDramConnection.PlugIn(dram.GetPortByName("Top"))
		lowModuleFinder.LowModules = append(lowModuleFinder.LowModules,
			dram.GetPortByName("Top").AsRemote())
	}

	b.dmaEngine.SetLocalDataSource(lowModuleFinder)
	b.l2ToDramConnection.PlugIn(b.dmaEngine.ToMem)

	b.pmc.MemCtrlFinder = lowModuleFinder
	b.l2ToDramConnection.PlugIn(
		b.pmc.GetPortByName("LocalMem"))
}

func (b *Builder) connectL1TLBToL2TLB() {
	tlbConn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(b.name + ".L1TLBToL2TLB")
	b.simulation.RegisterComponent(tlbConn)

	tlbConn.PlugIn(b.l2TLBs[0].GetPortByName("Top"))

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			tlbConn.PlugIn(
				sa.GetPortByName(fmt.Sprintf("L1VTLBBottom[%d]", i)))
		}

		tlbConn.PlugIn(sa.GetPortByName("L1STLBBottom"))
		tlbConn.PlugIn(sa.GetPortByName("L1ITLBBottom"))
	}
}

type cuInterfaceForCP struct {
	ctrlPort        sim.RemotePort
	dispatchingPort sim.RemotePort
	wfPoolSizes     []int
	vRegCounts      []int
	sRegCount       int
	ldsBytes        int
}

func (cu cuInterfaceForCP) ControlPort() sim.RemotePort {
	return cu.ctrlPort
}

func (cu cuInterfaceForCP) DispatchingPort() sim.RemotePort {
	return cu.dispatchingPort
}

func (cu cuInterfaceForCP) WfPoolSizes() []int {
	return cu.wfPoolSizes
}

func (cu cuInterfaceForCP) VRegCounts() []int {
	return cu.vRegCounts
}

func (cu cuInterfaceForCP) SRegCount() int {
	return cu.sRegCount
}

func (cu cuInterfaceForCP) LDSBytes() int {
	return cu.ldsBytes
}

func (b *Builder) connectCPWithCUs() {
	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			cuDispatchingPort := sa.GetPortByName(
				fmt.Sprintf("CU[%d]", i))
			cuCtrlPort := sa.GetPortByName(
				fmt.Sprintf("CUCtrl[%d]", i))
			wfPS := b.wfPoolSize
			cu := cuInterfaceForCP{
				ctrlPort:        cuCtrlPort.AsRemote(),
				dispatchingPort: cuDispatchingPort.AsRemote(),
				wfPoolSizes:     []int{wfPS, wfPS, wfPS, wfPS},
				vRegCounts:      []int{32768, 32768, 32768, 32768},
				sRegCount:       3200,
				ldsBytes:        64 * 1024,
			}

			b.cp.RegisterCU(cu)

			b.internalConn.PlugIn(cuDispatchingPort)
			b.internalConn.PlugIn(cuCtrlPort)
		}
	}
}

func (b *Builder) connectCPWithAddressTranslators() {
	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			at := sa.GetPortByName(fmt.Sprintf("L1VAddrTransCtrl[%d]", i))
			b.cp.AddressTranslators = append(b.cp.AddressTranslators, at)
			b.internalConn.PlugIn(at)
		}

		l1sAT := sa.GetPortByName("L1SAddrTransCtrl")
		b.cp.AddressTranslators = append(b.cp.AddressTranslators, l1sAT)
		b.internalConn.PlugIn(l1sAT)

		l1iAT := sa.GetPortByName("L1IAddrTransCtrl")
		b.cp.AddressTranslators = append(b.cp.AddressTranslators, l1iAT)
		b.internalConn.PlugIn(l1iAT)
	}
}

func (b *Builder) connectCPWithTLBs() {
	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			tlb := sa.GetPortByName(fmt.Sprintf("L1VTLBCtrl[%d]", i))
			b.cp.TLBs = append(b.cp.TLBs, tlb)
			b.internalConn.PlugIn(tlb)
		}

		l1sTLB := sa.GetPortByName("L1STLBCtrl")
		b.cp.TLBs = append(b.cp.TLBs, l1sTLB)
		b.internalConn.PlugIn(l1sTLB)

		l1iTLB := sa.GetPortByName("L1ITLBCtrl")
		b.cp.TLBs = append(b.cp.TLBs, l1iTLB)
		b.internalConn.PlugIn(l1iTLB)
	}

	for _, tlb := range b.l2TLBs {
		ctrlPort := tlb.GetPortByName("Control")
		b.cp.TLBs = append(b.cp.TLBs, ctrlPort)
		b.internalConn.PlugIn(ctrlPort)
	}
}

func (b *Builder) connectCPWithCaches() {
	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			cache := sa.GetPortByName(fmt.Sprintf("L1VCacheCtrl[%d]", i))
			b.cp.L1VCaches = append(b.cp.L1VCaches, cache)
			b.internalConn.PlugIn(cache)
		}

		l1sCache := sa.GetPortByName("L1SCacheCtrl")
		b.cp.L1SCaches = append(b.cp.L1SCaches, l1sCache)
		b.internalConn.PlugIn(l1sCache)

		l1iCache := sa.GetPortByName("L1ICacheCtrl")
		b.cp.L1ICaches = append(b.cp.L1ICaches, l1iCache)
		b.internalConn.PlugIn(l1iCache)
	}

	for _, c := range b.l2Caches {
		ctrlPort := c.GetPortByName("Control")
		b.cp.L2Caches = append(b.cp.L2Caches, ctrlPort)
		b.internalConn.PlugIn(ctrlPort)
	}
}

func (b *Builder) buildSAs() {
	// Use CDNA3 ALU for MI300A timing simulation
	aluFactory := func(sa emu.StorageAccessor) emu.ALU {
		return cdna3.NewALU(sa)
	}

	saBuilder := shaderarray.MakeBuilder().
		WithSimulation(b.simulation).
		WithFreq(b.freq).
		WithGPUID(b.gpuID).
		WithNumCUs(b.numCUPerShaderArray).
		WithLog2CacheLineSize(b.log2CacheLineSize).
		WithLog2PageSize(b.log2PageSize).
		WithL1TLBAddressMapper(b.l1TLBAddressMapper).
		WithALUFactory(aluFactory).
		WithWfPoolSize(b.wfPoolSize).
		WithVGPRCount([]int{32768, 32768, 32768, 32768}).
		WithNumSinglePrecisionUnits(64).
		WithVecMemInstPipelineStages(1).  // reduced from 2
		WithVecMemTransPipelineStages(1). // reduced from 4
		WithVecMemTransPipelineWidth(8).
		WithCUMemPipelineBufferSize(64).
		WithL1VCacheSize(32 * mem.KB).
		WithL1VNumMSHREntry(1024).
		WithL1VBankLatency(1).          // Calibrated L1V cache bank latency
		WithL1VMissFillExtraLatency(1). // reduced from 3
		WithMemPipelineBufferSize(64).
		WithMaxCoalescingPenalty(2).
		WithScratchLatency(30).
		WithRegisterScoreboard(true).
		WithInFlightVectorMemAccessLimit(4096).
		WithIsCDNA3(true)

	for i := 0; i < b.numShaderArray; i++ {
		xcdGroup := i / NumSAPerXCD
		saName := fmt.Sprintf("%s.SA[%d]", b.name, i)
		sa := saBuilder.
			WithL1AddressMapper(b.l1AddressMappers[xcdGroup]).
			Build(saName)
		b.sas = append(b.sas, sa)
	}
}

func (b *Builder) buildL2Caches() {
	byteSize := b.l2CacheSize / uint64(b.numMemoryBank)

	// Collect all DRAM top ports for interleaved L2→DRAM routing
	dramPorts := make([]sim.RemotePort, len(b.drams))
	for i, dram := range b.drams {
		dramPorts[i] = dram.GetPortByName("Top").AsRemote()
	}

	l2Builder := writeback.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(16).
		WithByteSize(byteSize).
		WithNumMSHREntry(2048).
		WithNumReqPerCycle(8).
		WithBankLatency(b.l2BankLatency).
		WithDirectoryLatency(2).
		WithMaxInflightFetch(2048).
		WithMaxInflightEviction(2048)

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.L2Cache[%d]", b.name, i)
		topPort := sim.NewPort(nil, 2048, 2048, cacheName+".Top")
		bottomPort := sim.NewPort(nil, 512, 512, cacheName+".Bottom")
		controlPort := sim.NewPort(nil, 512, 512, cacheName+".Control")
		l2 := l2Builder.WithInterleavingSize(
			1 << b.log2MemoryBankInterleavingSize).
			WithAddressMapperType("interleaved").
			WithRemotePorts(dramPorts...).
			WithTopPort(topPort).
			WithBottomPort(bottomPort).
			WithControlPort(controlPort).
			Build(cacheName)

		b.simulation.RegisterComponent(l2)
		b.l2Caches = append(b.l2Caches, l2)

		// Add to per-XCD mapper
		xcdGroup := i / NumL2BanksPerXCD
		b.l1AddressMappers[xcdGroup].LowModules = append(
			b.l1AddressMappers[xcdGroup].LowModules,
			l2.GetPortByName("Top").AsRemote(),
		)
		// Also add to global mapper (for RDMA)
		b.globalL1AddressMapper.LowModules = append(
			b.globalL1AddressMapper.LowModules,
			l2.GetPortByName("Top").AsRemote(),
		)
	}
}

func (b *Builder) buildDRAMControllers() {
	memBankSize := b.dramSize / uint64(b.numMemoryBank)
	for i := 0; i < b.numMemoryBank; i++ {
		dramName := fmt.Sprintf("%s.DRAM[%d]", b.name, i)
		memBuilder := simplebankedmemory.MakeBuilder().
			WithEngine(b.simulation.GetEngine()).
			WithFreq(2400 * sim.MHz).
			WithNumBanks(80).
			WithBankPipelineWidth(4).
			WithBankPipelineDepth(2). // reduced from 5
			WithStageLatency(1).
			WithRowBufferSizeLog2(11).
			WithRowMissDelay(10). // reduced from 25
			WithRowHitDelay(5).   // reduced from 15
			WithLog2InterleaveSize(6).
			WithTopPortBufferSize(1024).
			WithPostPipelineBufferSize(128).
			WithBankAddressConverter(&mem.InterleavingConverter{
				InterleavingSize:    1 << b.log2MemoryBankInterleavingSize,
				TotalNumOfElements:  b.numMemoryBank,
				CurrentElementIndex: i,
			})
		if b.globalStorage != nil {
			memBuilder = memBuilder.WithStorage(b.globalStorage)
		} else {
			memBuilder = memBuilder.WithNewStorage(memBankSize)
		}
		dram := memBuilder.Build(dramName)
		b.simulation.RegisterComponent(dram)
		b.drams = append(b.drams, dram)
	}
}

func (b *Builder) buildRDMAEngine() {
	name := fmt.Sprintf("%s.RDMA", b.name)
	b.rdmaEngine = rdma.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(1 * sim.GHz).
		WithLocalModules(b.globalL1AddressMapper).
		Build(name)

	b.rdmaEngine.RemoteRDMAAddressTable = b.rdmaAddressMapper

	b.simulation.RegisterComponent(b.rdmaEngine)
}

func (b *Builder) buildPageMigrationController() {
	b.pmc = pagemigrationcontroller.NewPageMigrationController(
		fmt.Sprintf("%s.PMC", b.name),
		b.simulation.GetEngine(),
		b.pmcAddressMapper,
		nil)

	b.simulation.RegisterComponent(b.pmc)
}

func (b *Builder) buildDMAEngine() {
	b.dmaEngine = cp.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.name),
		b.simulation.GetEngine(),
		nil)

	b.simulation.RegisterComponent(b.dmaEngine)
}

func (b *Builder) buildCP() {
	b.cp = cp.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithVisTracer(b.simulation.GetVisTracer()).
		WithFreq(b.freq).
		WithMonitor(b.simulation.GetMonitor()).
		WithConstantKernelLaunchOverhead(0).
		WithSubsequentKernelLaunchOverhead(0).
		WithConstantKernelOverhead(0).
		Build(b.name + ".CommandProcessor")

	b.simulation.RegisterComponent(b.cp)

	b.buildDMAEngine()
	b.buildRDMAEngine()
	b.buildPageMigrationController()
}

func (b *Builder) buildL2TLB() {
	numWays := 64
	builder := tlb.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithNumWays(numWays).
		WithNumSets(1024). // 64 ways × 1024 sets = 65536 entries, covers 256MB vs 16MB
		WithNumMSHREntry(64).
		WithNumReqPerCycle(1024).
		WithLog2PageSize(b.log2PageSize).
		WithTranslationProviderMapper(&mem.SinglePortMapper{
			Port: b.mmu.GetPortByName("Top").AsRemote(),
		})

	l2TLBName := fmt.Sprintf("%s.L2TLB", b.name)
	topPort := sim.NewPort(nil, 1024, 1024, l2TLBName+".Top")
	bottomPort := sim.NewPort(nil, 1024, 1024, l2TLBName+".Bottom")
	controlPort := sim.NewPort(nil, 1024, 1024, l2TLBName+".Control")
	l2TLB := builder.
		WithTopPort(topPort).
		WithBottomPort(bottomPort).
		WithControlPort(controlPort).
		Build(l2TLBName)

	b.simulation.RegisterComponent(l2TLB)
	b.l2TLBs = append(b.l2TLBs, l2TLB)

	b.l1TLBAddressMapper.Port = l2TLB.GetPortByName("Top").AsRemote()
}

func (b *Builder) numCU() int {
	return b.numCUPerShaderArray * b.numShaderArray
}
