// Package r9nano contains the configuration of GPUs similar to AMD Radeon R9
// Nano.
package r9nano

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/cache/writeback"
	"github.com/sarchlab/akita/v4/mem/dram"
	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm/mmu"
	"github.com/sarchlab/akita/v4/mem/vm/tlb"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/akita/v4/simulation"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner/timingconfig/shaderarray"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v4/amd/timing/pagemigrationcontroller"
	"github.com/sarchlab/mgpusim/v4/amd/timing/rdma"
)

// Builder builds a hardware platform for timing simulation.
type Builder struct {
	simulation *simulation.Simulation

	gpuID                          uint64
	name                           string
	freq                           sim.Freq
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

	gpu                *sim.Domain
	driver             *driver.Driver
	cp                 *cp.CommandProcessor
	rdmaEngine         *rdma.Comp
	pmc                *pagemigrationcontroller.PageMigrationController
	dmaEngine          *cp.DMAEngine
	sas                []*sim.Domain
	l2Caches           []*writeback.Comp
	l2TLBs             []*tlb.Comp
	drams              []sim.Component
	internalConn       *directconnection.Comp
	l2ToDramConnection *directconnection.Comp
	l1AddressMapper    *mem.InterleavedAddressPortMapper
	l1TLBAddressMapper *mem.SinglePortMapper
	pmcAddressMapper   mem.AddressToPortMapper
}

// MakeBuilder creates a new builder.
func MakeBuilder() Builder {
	return Builder{
		freq:                           1 * sim.GHz,
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
func (b Builder) WithGPUID(id uint64) Builder {
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
func (b Builder) WithMemAddrOffset(offset uint64) Builder {
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

// WithGlobalStorage sets the global storage that can provide the ultimate address translation.
func (b Builder) WithGlobalStorage(
	globalStorage *mem.Storage,
) Builder {
	b.globalStorage = globalStorage
	return b
}

// WithGPUDriver sets the GPU driver.
func (b Builder) WithGPUDriver(
	driver *driver.Driver,
) Builder {
	b.driver = driver
	return b
}

// WithDRAMSize sets the size of the DRAM.
func (b Builder) WithDRAMSize(size uint64) Builder {
	b.dramSize = size
	return b
}

// WithRDMAAddressMapper sets the RDMA address mapper.
func (b Builder) WithRDMAAddressMapper(mapper mem.AddressToPortMapper) Builder {
	b.rdmaAddressMapper = mapper
	return b
}

// Build builds the hardware platform.
func (b Builder) Build(name string) *sim.Domain {
	b.name = name
	b.gpu = sim.NewDomain(name)

	b.l1AddressMapper = mem.NewInterleavedAddressPortMapper(
		1 << b.log2MemoryBankInterleavingSize,
	)
	b.l1AddressMapper.LowAddress = b.memAddrOffset
	b.l1AddressMapper.HighAddress = b.memAddrOffset + b.dramSize
	b.l1AddressMapper.UseAddressSpaceLimitation = true

	b.l1TLBAddressMapper = &mem.SinglePortMapper{}

	b.buildSAs()
	b.buildDRAMControllers()
	b.buildL2Caches()
	b.buildCP()
	b.buildL2TLB()

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
	b.internalConn.PlugIn(b.cp.ToROBs)
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

	b.cp.Driver = b.driver.GetPortByName("GPU")
}

func (b *Builder) connectL1ToL2() {
	l1ToL2Conn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(b.name + ".L1ToL2")

	b.rdmaEngine.SetLocalModuleFinder(b.l1AddressMapper)
	b.l1AddressMapper.ModuleForOtherAddresses = b.rdmaEngine.RDMARequestInside.AsRemote()
	l1ToL2Conn.PlugIn(b.rdmaEngine.RDMARequestInside)
	l1ToL2Conn.PlugIn(b.rdmaEngine.RDMADataInside)

	for _, l2 := range b.l2Caches {
		l1ToL2Conn.PlugIn(l2.GetPortByName("Top"))
	}

	for _, sa := range b.sas {
		for i := range b.numCUPerShaderArray {
			l1ToL2Conn.PlugIn(
				sa.GetPortByName(fmt.Sprintf("L1VCacheBottom[%d]", i)))
		}

		l1ToL2Conn.PlugIn(sa.GetPortByName("L1SCacheBottom"))
		l1ToL2Conn.PlugIn(sa.GetPortByName("L1ICacheBottom"))
	}
}

func (b *Builder) connectL2AndDRAM() {
	b.l2ToDramConnection = directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(b.name + ".L2ToDRAM")
	b.simulation.RegisterComponent(b.l2ToDramConnection)

	lowModuleFinder := mem.NewInterleavedAddressPortMapper(
		1 << b.log2MemoryBankInterleavingSize)

	for i, l2 := range b.l2Caches {
		b.l2ToDramConnection.PlugIn(l2.GetPortByName("Bottom"))
		l2.SetAddressToPortMapper(&mem.SinglePortMapper{
			Port: b.drams[i].GetPortByName("Top").AsRemote(),
		})
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
			cu := cuInterfaceForCP{
				ctrlPort:        cuCtrlPort.AsRemote(),
				dispatchingPort: cuDispatchingPort.AsRemote(),
				wfPoolSizes:     []int{10, 10, 10, 10},
				vRegCounts:      []int{16384, 16384, 16384, 16384},
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

			rob := sa.GetPortByName(fmt.Sprintf("L1VROBCtrl[%d]", i))
			b.cp.ROBs = append(b.cp.ROBs, rob)
			b.internalConn.PlugIn(rob)
		}

		l1sTLB := sa.GetPortByName("L1STLBCtrl")
		b.cp.TLBs = append(b.cp.TLBs, l1sTLB)
		b.internalConn.PlugIn(l1sTLB)

		rob := sa.GetPortByName("L1SROBCtrl")
		b.cp.ROBs = append(b.cp.ROBs, rob)
		b.internalConn.PlugIn(rob)

		l1iTLB := sa.GetPortByName("L1ITLBCtrl")
		b.cp.TLBs = append(b.cp.TLBs, l1iTLB)
		b.internalConn.PlugIn(l1iTLB)

		rob = sa.GetPortByName("L1IROBCtrl")
		b.cp.ROBs = append(b.cp.ROBs, rob)
		b.internalConn.PlugIn(rob)
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
	saBuilder := shaderarray.MakeBuilder().
		WithSimulation(b.simulation).
		WithFreq(b.freq).
		WithGPUID(b.gpuID).
		WithNumCUs(b.numCUPerShaderArray).
		WithLog2CacheLineSize(b.log2CacheLineSize).
		WithLog2PageSize(b.log2PageSize).
		WithL1AddressMapper(b.l1AddressMapper).
		WithL1TLBAddressMapper(b.l1TLBAddressMapper)

	// if b.enableISADebugging {
	// 	saBuilder = saBuilder.withIsaDebugging()
	// }

	// if b.enableMemTracing {
	// 	saBuilder = saBuilder.withMemTracer(b.memTracer)
	// }

	for i := 0; i < b.numShaderArray; i++ {
		saName := fmt.Sprintf("%s.SA[%d]", b.name, i)
		sa := saBuilder.Build(saName)

		b.sas = append(b.sas, sa)
	}
}

func (b *Builder) buildL2Caches() {
	byteSize := b.l2CacheSize / uint64(b.numMemoryBank)
	l2Builder := writeback.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(16).
		WithByteSize(byteSize).
		WithNumMSHREntry(64).
		WithNumReqPerCycle(16)

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.L2Cache[%d]", b.name, i)
		l2 := l2Builder.WithInterleaving(
			1<<(b.log2MemoryBankInterleavingSize-b.log2CacheLineSize),
			b.numMemoryBank,
			i).
			WithAddressMapperType("single").
			WithRemotePorts(b.drams[i].GetPortByName("Top").AsRemote()).
			Build(cacheName)

		b.simulation.RegisterComponent(l2)
		b.l2Caches = append(b.l2Caches, l2)

		b.l1AddressMapper.LowModules = append(
			b.l1AddressMapper.LowModules,
			l2.GetPortByName("Top").AsRemote(),
		)

		// if b.enableMemTracing {
		// 	tracing.CollectTrace(l2, b.memTracer)
		// }
	}
}

func (b *Builder) buildDRAMControllers() {
	// memCtrlBuilder := b.createDramControllerBuilder()

	for i := 0; i < b.numMemoryBank; i++ {
		dramName := fmt.Sprintf("%s.DRAM[%d]", b.name, i)
		dram := idealmemcontroller.MakeBuilder().
			WithEngine(b.simulation.GetEngine()).
			WithFreq(b.freq).
			WithLatency(100).
			WithStorage(b.globalStorage).
			Build(dramName)
		b.simulation.RegisterComponent(dram)
		b.drams = append(b.drams, dram)

		// if b.enableMemTracing {
		// 	tracing.CollectTrace(dram, b.memTracer)
		// }
	}
}

func (b *Builder) createDramControllerBuilder() dram.Builder {
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

	memCtrlBuilder := dram.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(500 * sim.MHz).
		WithProtocol(dram.HBM).
		WithBurstLength(4).
		WithDeviceWidth(dramDeviceWidth).
		WithBusWidth(dramBusWidth).
		WithNumChannel(1).
		WithNumRank(dramRank).
		WithNumBankGroup(dramBankGroup).
		WithNumBank(dramBank).
		WithNumCol(dramCol).
		WithNumRow(dramRow).
		WithCommandQueueSize(8).
		WithTransactionQueueSize(32).
		WithTCL(7).
		WithTCWL(2).
		WithTRCDRD(7).
		WithTRCDWR(7).
		WithTRP(7).
		WithTRAS(17).
		WithTREFI(1950).
		WithTRRDS(2).
		WithTRRDL(3).
		WithTWTRS(3).
		WithTWTRL(4).
		WithTWR(8).
		WithTCCDS(1).
		WithTCCDL(1).
		WithTRTRS(0).
		WithTRTP(3).
		WithTPPD(2)

	if b.globalStorage != nil {
		memCtrlBuilder = memCtrlBuilder.WithGlobalStorage(b.globalStorage)
	}

	return memCtrlBuilder
}

func (b *Builder) buildRDMAEngine() {
	name := fmt.Sprintf("%s.RDMA", b.name)
	b.rdmaEngine = rdma.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(1 * sim.GHz).
		WithLocalModules(b.l1AddressMapper).
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
		WithNumSets(int(b.dramSize / (1 << b.log2PageSize) / uint64(numWays))).
		WithNumMSHREntry(64).
		WithNumReqPerCycle(1024).
		WithPageSize(1 << b.log2PageSize).
		WithLowModule(b.mmu.GetPortByName("Top").AsRemote()).
		WithTranslationProviderMapper(&mem.SinglePortMapper{
			Port: b.mmu.GetPortByName("Top").AsRemote(),
		})

	l2TLB := builder.Build(fmt.Sprintf("%s.L2TLB", b.name))

	b.simulation.RegisterComponent(l2TLB)
	b.l2TLBs = append(b.l2TLBs, l2TLB)

	b.l1TLBAddressMapper.Port = l2TLB.GetPortByName("Top").AsRemote()
}

func (b *Builder) numCU() int {
	return b.numCUPerShaderArray * b.numShaderArray
}
