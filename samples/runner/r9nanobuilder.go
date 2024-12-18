package runner

import (
	"fmt"

	rob2 "github.com/sarchlab/mgpusim/v3/timing/rob"

	"github.com/sarchlab/akita/v3/analysis"
	"github.com/sarchlab/akita/v3/mem/cache/writearound"
	"github.com/sarchlab/akita/v3/mem/cache/writeback"
	"github.com/sarchlab/akita/v3/mem/cache/writethrough"
	"github.com/sarchlab/akita/v3/mem/dram"
	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/mem/vm"
	"github.com/sarchlab/akita/v3/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v3/mem/vm/gmmu"
	"github.com/sarchlab/akita/v3/mem/vm/mmu"
	"github.com/sarchlab/akita/v3/mem/vm/tlb"
	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/timing/cp"
	"github.com/sarchlab/mgpusim/v3/timing/cu"
	"github.com/sarchlab/mgpusim/v3/timing/pagemigrationcontroller"
	"github.com/sarchlab/mgpusim/v3/timing/rdma"
)

// R9NanoGPUBuilder can build R9 Nano GPUs.
type R9NanoGPUBuilder struct {
	engine                         sim.Engine
	freq                           sim.Freq
	memAddrOffset                  uint64
	mmu                            *mmu.MMU
	numShaderArray                 int
	numCUPerShaderArray            int
	numMemoryBank                  int
	dramSize                       uint64
	l2CacheSize                    uint64
	log2PageSize                   uint64
	log2CacheLineSize              uint64
	log2MemoryBankInterleavingSize uint64

	enableISADebugging bool
	enableMemTracing   bool
	enableVisTracing   bool
	visTracer          tracing.Tracer
	memTracer          tracing.Tracer
	monitor            *monitoring.Monitor
	perfAnalyzer       *analysis.PerfAnalyzer

	gpuName                 string
	gpu                     *GPU
	gpuID                   uint64
	cp                      *cp.CommandProcessor
	cus                     []*cu.ComputeUnit
	l1vReorderBuffers       []*rob2.ReorderBuffer
	l1iReorderBuffers       []*rob2.ReorderBuffer
	l1sReorderBuffers       []*rob2.ReorderBuffer
	l1vCaches               []*writearound.Cache
	l1sCaches               []*writethrough.Cache
	l1iCaches               []*writethrough.Cache
	l2Caches                []*writeback.Cache
	l1vAddrTrans            []*addresstranslator.AddressTranslator
	l1sAddrTrans            []*addresstranslator.AddressTranslator
	l1iAddrTrans            []*addresstranslator.AddressTranslator
	l1vTLBs                 []*tlb.TLB
	l1sTLBs                 []*tlb.TLB
	l1iTLBs                 []*tlb.TLB
	l2TLBs                  []*tlb.TLB
	drams                   []*dram.MemController
	lowModuleFinderForL1    *mem.InterleavedLowModuleFinder
	lowModuleFinderForL2    *mem.InterleavedLowModuleFinder
	lowModuleFinderForPMC   *mem.InterleavedLowModuleFinder
	dmaEngine               *cp.DMAEngine
	rdmaEngine              *rdma.Comp
	pageMigrationController *pagemigrationcontroller.PageMigrationController
	globalStorage           *mem.Storage

	internalConn           *sim.DirectConnection
	l1TLBToL2TLBConnection *sim.DirectConnection
	l1ToL2Connection       *sim.DirectConnection
	l2ToDramConnection     *sim.DirectConnection

	gmmuCache *tlb.TLB
	gmmu      *gmmu.Comp
	pageTable vm.PageTable
}

// MakeR9NanoGPUBuilder provides a GPU builder that can builds the R9Nano GPU.
func MakeR9NanoGPUBuilder() R9NanoGPUBuilder {
	b := R9NanoGPUBuilder{
		freq:                           1 * sim.GHz,
		numShaderArray:                 16,
		numCUPerShaderArray:            4,
		numMemoryBank:                  16,
		log2CacheLineSize:              6,
		log2PageSize:                   12,
		log2MemoryBankInterleavingSize: 12,
		l2CacheSize:                    2 * mem.MB,
		dramSize:                       4 * mem.GB,
	}
	return b
}

// WithEngine sets the engine that the GPU use.
func (b R9NanoGPUBuilder) WithEngine(engine sim.Engine) R9NanoGPUBuilder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the GPU works at.
func (b R9NanoGPUBuilder) WithFreq(freq sim.Freq) R9NanoGPUBuilder {
	b.freq = freq
	return b
}

// WithMemAddrOffset sets the address of the first byte of the GPU to build.
func (b R9NanoGPUBuilder) WithMemAddrOffset(
	offset uint64,
) R9NanoGPUBuilder {
	b.memAddrOffset = offset
	return b
}

// WithMMU sets the MMU component that provides the address translation service
// for the GPU.
func (b R9NanoGPUBuilder) WithMMU(mmu *mmu.MMU) R9NanoGPUBuilder {
	b.mmu = mmu
	return b
}

// WithNumMemoryBank sets the number of L2 cache modules and number of memory
// controllers in each GPU.
func (b R9NanoGPUBuilder) WithNumMemoryBank(n int) R9NanoGPUBuilder {
	b.numMemoryBank = n
	return b
}

// WithNumShaderArray sets the number of shader arrays in each GPU. Each shader
// array contains a certain number of CUs, a certain number of L1V caches, 1
// L1S cache, and 1 L1V cache.
func (b R9NanoGPUBuilder) WithNumShaderArray(n int) R9NanoGPUBuilder {
	b.numShaderArray = n
	return b
}

// WithNumCUPerShaderArray sets the number of CU and number of L1V caches in
// each Shader Array.
func (b R9NanoGPUBuilder) WithNumCUPerShaderArray(n int) R9NanoGPUBuilder {
	b.numCUPerShaderArray = n
	return b
}

// WithLog2MemoryBankInterleavingSize sets the number of consecutive bytes that
// are guaranteed to be on a memory bank.
func (b R9NanoGPUBuilder) WithLog2MemoryBankInterleavingSize(
	n uint64,
) R9NanoGPUBuilder {
	b.log2MemoryBankInterleavingSize = n
	return b
}

// WithVisTracer applies a tracer to trace all the tasks of all the GPU
// components
func (b R9NanoGPUBuilder) WithVisTracer(t tracing.Tracer) R9NanoGPUBuilder {
	b.enableVisTracing = true
	b.visTracer = t
	return b
}

// WithMemTracer applies a tracer to trace the memory transactions.
func (b R9NanoGPUBuilder) WithMemTracer(t tracing.Tracer) R9NanoGPUBuilder {
	b.enableMemTracing = true
	b.memTracer = t
	return b
}

// WithISADebugging enables the GPU to dump instruction execution information.
func (b R9NanoGPUBuilder) WithISADebugging() R9NanoGPUBuilder {
	b.enableISADebugging = true
	return b
}

// WithLog2CacheLineSize sets the cache line size with the power of 2.
func (b R9NanoGPUBuilder) WithLog2CacheLineSize(
	log2CacheLine uint64,
) R9NanoGPUBuilder {
	b.log2CacheLineSize = log2CacheLine
	return b
}

// WithLog2PageSize sets the page size with the power of 2.
func (b R9NanoGPUBuilder) WithLog2PageSize(log2PageSize uint64) R9NanoGPUBuilder {
	b.log2PageSize = log2PageSize
	return b
}

// WithMonitor sets the monitor to use.
func (b R9NanoGPUBuilder) WithMonitor(m *monitoring.Monitor) R9NanoGPUBuilder {
	b.monitor = m
	return b
}

// WithPerfAnalyzer sets the buffer analyzer to use.
func (b R9NanoGPUBuilder) WithPerfAnalyzer(
	a *analysis.PerfAnalyzer,
) R9NanoGPUBuilder {
	b.perfAnalyzer = a
	return b
}

// WithL2CacheSize set the total L2 cache size. The size of the L2 cache is
// split between memory banks.
func (b R9NanoGPUBuilder) WithL2CacheSize(size uint64) R9NanoGPUBuilder {
	b.l2CacheSize = size
	return b
}

// WithDRAMSize sets the size of DRAMs in the GPU.
func (b R9NanoGPUBuilder) WithDRAMSize(size uint64) R9NanoGPUBuilder {
	b.dramSize = size
	return b
}

// WithGlobalStorage lets the GPU to build to use the externally provided
// storage.
func (b R9NanoGPUBuilder) WithGlobalStorage(
	storage *mem.Storage,
) R9NanoGPUBuilder {
	b.globalStorage = storage
	return b
}

// Build creates a pre-configure GPU similar to the AMD R9 Nano GPU.
func (b R9NanoGPUBuilder) Build(name string, id uint64) *GPU {
	b.createGPU(name, id)
	b.buildSAs()
	b.buildL2Caches()
	b.buildDRAMControllers()
	b.buildCP()
	b.buildL2TLB()
	b.buildGMMU()
	b.buildGMMUCache()

	b.connectCP()
	b.connectL2AndDRAM()
	b.connectL1ToL2()
	b.connectL1TLBToL2TLB()
	b.connectL2TLBToGMMUCache()
	b.connectGMMUCachetoGMMU()

	b.populateExternalPorts()

	return b.gpu
}

func (b *R9NanoGPUBuilder) populateExternalPorts() {
	b.gpu.Domain.AddPort("CommandProcessor", b.cp.ToDriver)
	b.gpu.Domain.AddPort("RDMA", b.rdmaEngine.ToOutside)
	b.gpu.Domain.AddPort("PageMigrationController",
		b.pageMigrationController.GetPortByName("Remote"))

	for i, l2TLB := range b.l2TLBs {
		name := fmt.Sprintf("Translation_%02d", i)
		b.gpu.Domain.AddPort(name, l2TLB.GetPortByName("Bottom"))
	}
	b.gpu.Domain.AddPort("GMMU", b.gmmu.GetPortByName("Bottom"))
}

func (b *R9NanoGPUBuilder) createGPU(name string, id uint64) {
	b.gpuName = name

	b.gpu = &GPU{}
	b.gpu.Domain = sim.NewDomain(b.gpuName)
	b.gpuID = id
}

func (b *R9NanoGPUBuilder) connectCP() {
	b.internalConn = sim.NewDirectConnection(
		b.gpuName+".InternalConn", b.engine, b.freq)

	b.internalConn.PlugIn(b.cp.ToDriver, 1)
	b.internalConn.PlugIn(b.cp.ToDMA, 128)
	b.internalConn.PlugIn(b.cp.ToCaches, 128)
	b.internalConn.PlugIn(b.cp.ToCUs, 128)
	b.internalConn.PlugIn(b.cp.ToTLBs, 128)
	b.internalConn.PlugIn(b.cp.ToAddressTranslators, 128)
	b.internalConn.PlugIn(b.cp.ToRDMA, 4)
	b.internalConn.PlugIn(b.cp.ToPMC, 4)

	b.cp.RDMA = b.rdmaEngine.CtrlPort
	b.internalConn.PlugIn(b.cp.RDMA, 1)

	b.cp.DMAEngine = b.dmaEngine.ToCP
	b.internalConn.PlugIn(b.dmaEngine.ToCP, 1)

	pmcControlPort := b.pageMigrationController.GetPortByName("Control")
	b.cp.PMC = pmcControlPort
	b.internalConn.PlugIn(pmcControlPort, 1)

	b.connectCPWithCUs()
	b.connectCPWithAddressTranslators()
	b.connectCPWithTLBs()
	b.connectCPWithCaches()
}

func (b *R9NanoGPUBuilder) connectL1ToL2() {
	lowModuleFinder := mem.NewInterleavedLowModuleFinder(
		1 << b.log2MemoryBankInterleavingSize)
	lowModuleFinder.ModuleForOtherAddresses = b.rdmaEngine.ToL1
	lowModuleFinder.UseAddressSpaceLimitation = true
	lowModuleFinder.LowAddress = b.memAddrOffset
	lowModuleFinder.HighAddress = b.memAddrOffset + 4*mem.GB

	l1ToL2Conn := sim.NewDirectConnection(b.gpuName+".L1ToL2",
		b.engine, b.freq)

	b.rdmaEngine.SetLocalModuleFinder(lowModuleFinder)
	l1ToL2Conn.PlugIn(b.rdmaEngine.ToL1, 64)
	l1ToL2Conn.PlugIn(b.rdmaEngine.ToL2, 64)

	for _, l2 := range b.l2Caches {
		lowModuleFinder.LowModules = append(lowModuleFinder.LowModules,
			l2.GetPortByName("Top"))
		l1ToL2Conn.PlugIn(l2.GetPortByName("Top"), 64)
	}

	for _, l1v := range b.l1vCaches {
		l1v.SetLowModuleFinder(lowModuleFinder)
		l1ToL2Conn.PlugIn(l1v.GetPortByName("Bottom"), 16)
	}

	for _, l1s := range b.l1sCaches {
		l1s.SetLowModuleFinder(lowModuleFinder)
		l1ToL2Conn.PlugIn(l1s.GetPortByName("Bottom"), 16)
	}

	for _, l1iAT := range b.l1iAddrTrans {
		l1iAT.SetLowModuleFinder(lowModuleFinder)
		l1ToL2Conn.PlugIn(l1iAT.GetPortByName("Bottom"), 16)
	}
}

func (b *R9NanoGPUBuilder) connectL2AndDRAM() {
	b.l2ToDramConnection = sim.NewDirectConnection(
		b.gpuName+".L2ToDRAM", b.engine, b.freq)

	lowModuleFinder := mem.NewInterleavedLowModuleFinder(
		1 << b.log2MemoryBankInterleavingSize)

	for i, l2 := range b.l2Caches {
		b.l2ToDramConnection.PlugIn(l2.GetPortByName("Bottom"), 64)
		l2.SetLowModuleFinder(&mem.SingleLowModuleFinder{
			LowModule: b.drams[i].GetPortByName("Top"),
		})
	}

	for _, dram := range b.drams {
		b.l2ToDramConnection.PlugIn(dram.GetPortByName("Top"), 64)
		lowModuleFinder.LowModules = append(lowModuleFinder.LowModules,
			dram.GetPortByName("Top"))
	}

	b.dmaEngine.SetLocalDataSource(lowModuleFinder)
	b.l2ToDramConnection.PlugIn(b.dmaEngine.ToMem, 64)

	b.pageMigrationController.MemCtrlFinder = lowModuleFinder
	b.l2ToDramConnection.PlugIn(
		b.pageMigrationController.GetPortByName("LocalMem"), 16)
}

func (b *R9NanoGPUBuilder) connectL1TLBToL2TLB() {
	tlbConn := sim.NewDirectConnection(b.gpuName+".L1TLBToL2TLB",
		b.engine, b.freq)

	tlbConn.PlugIn(b.l2TLBs[0].GetPortByName("Top"), 64)

	for _, l1vTLB := range b.l1vTLBs {
		l1vTLB.LowModule = b.l2TLBs[0].GetPortByName("Top")
		tlbConn.PlugIn(l1vTLB.GetPortByName("Bottom"), 16)
	}

	for _, l1iTLB := range b.l1iTLBs {
		l1iTLB.LowModule = b.l2TLBs[0].GetPortByName("Top")
		tlbConn.PlugIn(l1iTLB.GetPortByName("Bottom"), 16)
	}

	for _, l1sTLB := range b.l1sTLBs {
		l1sTLB.LowModule = b.l2TLBs[0].GetPortByName("Top")
		tlbConn.PlugIn(l1sTLB.GetPortByName("Bottom"), 16)
	}
}

func (b *R9NanoGPUBuilder) connectCPWithCUs() {
	for _, cu := range b.cus {
		b.cp.RegisterCU(cu)
		b.internalConn.PlugIn(cu.ToACE, 1)
		b.internalConn.PlugIn(cu.ToCP, 1)
	}
}

func (b *R9NanoGPUBuilder) connectCPWithAddressTranslators() {
	for _, at := range b.l1vAddrTrans {
		ctrlPort := at.GetPortByName("Control")
		b.cp.AddressTranslators = append(b.cp.AddressTranslators, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, at := range b.l1sAddrTrans {
		ctrlPort := at.GetPortByName("Control")
		b.cp.AddressTranslators = append(b.cp.AddressTranslators, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, at := range b.l1iAddrTrans {
		ctrlPort := at.GetPortByName("Control")
		b.cp.AddressTranslators = append(b.cp.AddressTranslators, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, rob := range b.l1vReorderBuffers {
		ctrlPort := rob.GetPortByName("Control")
		b.cp.AddressTranslators = append(
			b.cp.AddressTranslators, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, rob := range b.l1iReorderBuffers {
		ctrlPort := rob.GetPortByName("Control")
		b.cp.AddressTranslators = append(
			b.cp.AddressTranslators, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, rob := range b.l1sReorderBuffers {
		ctrlPort := rob.GetPortByName("Control")
		b.cp.AddressTranslators = append(
			b.cp.AddressTranslators, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}
}

func (b *R9NanoGPUBuilder) connectCPWithTLBs() {
	for _, tlb := range b.l2TLBs {
		ctrlPort := tlb.GetPortByName("Control")
		b.cp.TLBs = append(b.cp.TLBs, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, tlb := range b.l1vTLBs {
		ctrlPort := tlb.GetPortByName("Control")
		b.cp.TLBs = append(b.cp.TLBs, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, tlb := range b.l1sTLBs {
		ctrlPort := tlb.GetPortByName("Control")
		b.cp.TLBs = append(b.cp.TLBs, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, tlb := range b.l1iTLBs {
		ctrlPort := tlb.GetPortByName("Control")
		b.cp.TLBs = append(b.cp.TLBs, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}
}

func (b *R9NanoGPUBuilder) connectCPWithCaches() {
	for _, c := range b.l1iCaches {
		ctrlPort := c.GetPortByName("Control")
		b.cp.L1ICaches = append(b.cp.L1ICaches, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, c := range b.l1vCaches {
		ctrlPort := c.GetPortByName("Control")
		b.cp.L1VCaches = append(b.cp.L1VCaches, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, c := range b.l1sCaches {
		ctrlPort := c.GetPortByName("Control")
		b.cp.L1SCaches = append(b.cp.L1SCaches, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}

	for _, c := range b.l2Caches {
		ctrlPort := c.GetPortByName("Control")
		b.cp.L2Caches = append(b.cp.L2Caches, ctrlPort)
		b.internalConn.PlugIn(ctrlPort, 1)
	}
}

func (b *R9NanoGPUBuilder) buildSAs() {
	saBuilder := makeShaderArrayBuilder().
		withEngine(b.engine).
		withFreq(b.freq).
		withGPUID(b.gpuID).
		withLog2CachelineSize(b.log2CacheLineSize).
		withLog2PageSize(b.log2PageSize).
		withNumCU(b.numCUPerShaderArray)

	if b.enableISADebugging {
		saBuilder = saBuilder.withIsaDebugging()
	}

	if b.enableVisTracing {
		saBuilder = saBuilder.withVisTracer(b.visTracer)
	}

	if b.enableMemTracing {
		saBuilder = saBuilder.withMemTracer(b.memTracer)
	}

	for i := 0; i < b.numShaderArray; i++ {
		saName := fmt.Sprintf("%s.SA[%d]", b.gpuName, i)
		b.buildSA(saBuilder, saName)
	}
}

func (b *R9NanoGPUBuilder) buildL2Caches() {
	byteSize := b.l2CacheSize / uint64(b.numMemoryBank)
	l2Builder := writeback.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(16).
		WithByteSize(byteSize).
		WithNumMSHREntry(64).
		WithNumReqPerCycle(16)

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.L2[%d]", b.gpuName, i)
		l2 := l2Builder.WithInterleaving(
			1<<(b.log2MemoryBankInterleavingSize-b.log2CacheLineSize),
			b.numMemoryBank,
			i,
		).Build(cacheName)
		b.l2Caches = append(b.l2Caches, l2)
		b.gpu.L2Caches = append(b.gpu.L2Caches, l2)

		if b.enableVisTracing {
			tracing.CollectTrace(l2, b.visTracer)
		}

		if b.enableMemTracing {
			tracing.CollectTrace(l2, b.memTracer)
		}

		if b.monitor != nil {
			b.monitor.RegisterComponent(l2)
		}
	}
}

func (b *R9NanoGPUBuilder) buildDRAMControllers() {
	memCtrlBuilder := b.createDramControllerBuilder()

	for i := 0; i < b.numMemoryBank; i++ {
		dramName := fmt.Sprintf("%s.DRAM[%d]", b.gpuName, i)
		dram := memCtrlBuilder.
			Build(dramName)
		// dram := idealmemcontroller.New(
		// 	fmt.Sprintf("%s.DRAM_%d", b.gpuName, i),
		// 	b.engine, 512*mem.MB)
		b.drams = append(b.drams, dram)
		b.gpu.MemControllers = append(b.gpu.MemControllers, dram)

		if b.enableMemTracing {
			tracing.CollectTrace(dram, b.memTracer)
		}

		if b.monitor != nil {
			b.monitor.RegisterComponent(dram)
		}
	}
}

func (b *R9NanoGPUBuilder) createDramControllerBuilder() dram.Builder {
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
		WithEngine(b.engine).
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

	if b.visTracer != nil {
		memCtrlBuilder = memCtrlBuilder.WithAdditionalTracer(b.visTracer)
	}

	if b.globalStorage != nil {
		memCtrlBuilder = memCtrlBuilder.WithGlobalStorage(b.globalStorage)
	}

	return memCtrlBuilder
}

func (b *R9NanoGPUBuilder) buildSA(
	saBuilder shaderArrayBuilder,
	saName string,
) {
	sa := saBuilder.Build(saName)

	b.populateCUs(&sa)
	b.populateROBs(&sa)
	b.populateTLBs(&sa)
	b.populateL1VAddressTranslators(&sa)
	b.populateL1Vs(&sa)
	b.populateScalerMemoryHierarchy(&sa)
	b.populateInstMemoryHierarchy(&sa)
}

func (b *R9NanoGPUBuilder) populateCUs(sa *shaderArray) {
	for _, cu := range sa.cus {
		b.cus = append(b.cus, cu)
		b.gpu.CUs = append(b.gpu.CUs, cu)

		if b.monitor != nil {
			b.monitor.RegisterComponent(cu)
		}

		if b.perfAnalyzer != nil {
			b.perfAnalyzer.RegisterComponent(cu)
		}
	}
	for _, cu := range sa.cus {
		for _, simd := range cu.SIMDUnit {
			b.gpu.SIMDs = append(b.gpu.SIMDs, simd.(TraceableComponent))
		}
	}
}

func (b *R9NanoGPUBuilder) populateROBs(sa *shaderArray) {
	for _, rob := range sa.l1vROBs {
		b.l1vReorderBuffers = append(b.l1vReorderBuffers, rob)

		if b.monitor != nil {
			b.monitor.RegisterComponent(rob)
		}

		if b.perfAnalyzer != nil {
			b.perfAnalyzer.RegisterComponent(rob)
		}
	}
}

func (b *R9NanoGPUBuilder) populateTLBs(sa *shaderArray) {
	for _, tlb := range sa.l1vTLBs {
		b.l1vTLBs = append(b.l1vTLBs, tlb)
		b.gpu.L1VTLBs = append(b.gpu.L1VTLBs, tlb)

		if b.monitor != nil {
			b.monitor.RegisterComponent(tlb)
		}

		if b.perfAnalyzer != nil {
			b.perfAnalyzer.RegisterComponent(tlb)
		}
	}
}

func (b *R9NanoGPUBuilder) populateL1Vs(sa *shaderArray) {
	for _, l1v := range sa.l1vCaches {
		b.l1vCaches = append(b.l1vCaches, l1v)
		b.gpu.L1VCaches = append(b.gpu.L1VCaches, l1v)

		if b.monitor != nil {
			b.monitor.RegisterComponent(l1v)
		}
	}
}

func (b *R9NanoGPUBuilder) populateL1VAddressTranslators(sa *shaderArray) {
	for _, at := range sa.l1vATs {
		b.l1vAddrTrans = append(b.l1vAddrTrans, at)

		if b.monitor != nil {
			b.monitor.RegisterComponent(at)
		}
	}
}

func (b *R9NanoGPUBuilder) populateScalerMemoryHierarchy(sa *shaderArray) {
	b.l1sAddrTrans = append(b.l1sAddrTrans, sa.l1sAT)
	b.l1sReorderBuffers = append(b.l1sReorderBuffers, sa.l1sROB)
	b.l1sCaches = append(b.l1sCaches, sa.l1sCache)
	b.gpu.L1SCaches = append(b.gpu.L1SCaches, sa.l1sCache)
	b.l1sTLBs = append(b.l1sTLBs, sa.l1sTLB)
	b.gpu.L1STLBs = append(b.gpu.L1STLBs, sa.l1sTLB)

	if b.monitor != nil {
		b.monitor.RegisterComponent(sa.l1sAT)
		b.monitor.RegisterComponent(sa.l1sROB)
		b.monitor.RegisterComponent(sa.l1sCache)
		b.monitor.RegisterComponent(sa.l1sTLB)
	}
}

func (b *R9NanoGPUBuilder) populateInstMemoryHierarchy(sa *shaderArray) {
	b.l1iAddrTrans = append(b.l1iAddrTrans, sa.l1iAT)
	b.l1iReorderBuffers = append(b.l1iReorderBuffers, sa.l1iROB)
	b.l1iCaches = append(b.l1iCaches, sa.l1iCache)
	b.gpu.L1ICaches = append(b.gpu.L1ICaches, sa.l1iCache)
	b.l1iTLBs = append(b.l1iTLBs, sa.l1iTLB)
	b.gpu.L1ITLBs = append(b.gpu.L1ITLBs, sa.l1iTLB)

	if b.monitor != nil {
		b.monitor.RegisterComponent(sa.l1iAT)
		b.monitor.RegisterComponent(sa.l1iROB)
		b.monitor.RegisterComponent(sa.l1iCache)
		b.monitor.RegisterComponent(sa.l1iTLB)
	}
}

func (b *R9NanoGPUBuilder) buildRDMAEngine() {
	name := fmt.Sprintf("%s.RDMA", b.gpuName)
	b.rdmaEngine = rdma.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(1 * sim.GHz).
		WithLocalModules(b.lowModuleFinderForL1).
		Build(name)
	b.gpu.RDMAEngine = b.rdmaEngine

	if b.monitor != nil {
		b.monitor.RegisterComponent(b.rdmaEngine)
	}

	if b.enableVisTracing {
		tracing.CollectTrace(b.rdmaEngine, b.visTracer)
	}
}

func (b *R9NanoGPUBuilder) buildPageMigrationController() {
	b.pageMigrationController =
		pagemigrationcontroller.NewPageMigrationController(
			fmt.Sprintf("%s.PMC", b.gpuName),
			b.engine,
			b.lowModuleFinderForPMC,
			nil)
	b.gpu.PMC = b.pageMigrationController

	if b.monitor != nil {
		b.monitor.RegisterComponent(b.pageMigrationController)
	}
}

func (b *R9NanoGPUBuilder) buildDMAEngine() {
	b.dmaEngine = cp.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.gpuName),
		b.engine,
		nil)

	if b.enableVisTracing {
		tracing.CollectTrace(b.dmaEngine, b.visTracer)
	}

	if b.monitor != nil {
		b.monitor.RegisterComponent(b.dmaEngine)
	}
}

func (b *R9NanoGPUBuilder) buildCP() {
	builder := cp.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithMonitor(b.monitor).
		WithPerfAnalyzer(b.perfAnalyzer)

	if b.enableVisTracing {
		builder = builder.WithVisTracer(b.visTracer)
	}

	b.cp = builder.Build(b.gpuName + ".CommandProcessor")
	b.gpu.CommandProcessor = b.cp

	if b.monitor != nil {
		b.monitor.RegisterComponent(b.cp)
	}

	b.buildDMAEngine()
	b.buildRDMAEngine()
	b.buildPageMigrationController()
}

func (b *R9NanoGPUBuilder) buildL2TLB() {
	numWays := 64
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumWays(numWays).
		WithNumSets(int(b.dramSize / (1 << b.log2PageSize) / uint64(numWays))).
		WithNumMSHREntry(64).
		WithNumReqPerCycle(1024).
		WithPageSize(1 << b.log2PageSize).
		WithLowModule(b.gmmuCache.GetPortByName("Top"))

	l2TLB := builder.Build(fmt.Sprintf("%s.L2TLB", b.gpuName))
	b.l2TLBs = append(b.l2TLBs, l2TLB)
	b.gpu.L2TLBs = append(b.gpu.L2TLBs, l2TLB)

	if b.enableVisTracing {
		tracing.CollectTrace(l2TLB, b.visTracer)
	}

	if b.monitor != nil {
		b.monitor.RegisterComponent(l2TLB)
	}
}

func (b *R9NanoGPUBuilder) numCU() int {
	return b.numCUPerShaderArray * b.numShaderArray
}

func (b *R9NanoGPUBuilder) connectWithDirectConnection(
	port1, port2 sim.Port,
	bufferSize int,
) {
	conn := sim.NewDirectConnection(
		port1.Name()+"-"+port2.Name(),
		b.engine, b.freq,
	)
	conn.PlugIn(port1, bufferSize)
	conn.PlugIn(port2, bufferSize)
}

func (b R9NanoGPUBuilder) WithGMMUPageTable(
	pageTable vm.PageTable,
) R9NanoGPUBuilder {
	b.pageTable = pageTable
	return b
}

func (b *R9NanoGPUBuilder) buildGMMUCache() {
	// numWays := 128
	// test:= int(b.dramSize / (1 << b.log2PageSize) / uint64(numWays))
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumWays(8).
		WithNumSets(16).
		WithNumMSHREntry(32).
		WithNumReqPerCycle(32).
		WithPageSize(1 << b.log2PageSize).
		WithLowModule(b.gmmu.GetPortByName("Top"))

	gmmuCache := builder.Build(fmt.Sprintf("%s.GMMUCache", b.gpuName))
	b.gmmuCache = gmmuCache
	b.gpu.GMMUCache = append(b.gpu.GMMUCache, gmmuCache)
	// b.gpu.L2TLBs = append(b.gpu.L2TLBs, l2TLB)

	if b.enableVisTracing {
		tracing.CollectTrace(b.gmmuCache, b.visTracer)
	}

	if b.monitor != nil {
		b.monitor.RegisterComponent(b.gmmuCache)
	}

	if b.perfAnalyzer != nil {
		b.perfAnalyzer.RegisterComponent(b.gmmuCache)
	}
}

func (b *R9NanoGPUBuilder) buildGMMU() {
	gmmu := gmmu.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithDeviceID(b.gpuID).
		WithLog2PageSize(b.log2PageSize).
		WithMaxNumReqInFlight(8).
		WithPageTable(b.pageTable).WithPageWalkingLatency(100).
		WithLowModule(b.mmu.GetPortByName("Top")).
		Build(fmt.Sprintf("%s.GMMU", b.gpuName))

	b.gmmu = gmmu
	b.gpu.GMMUEngine = b.gmmu

	if b.enableVisTracing {
		tracing.CollectTrace(b.gmmu, b.visTracer)
	}

	if b.monitor != nil {
		b.monitor.RegisterComponent(b.gmmu)
	}
}

func (b *R9NanoGPUBuilder) connectL2TLBToGMMUCache() {
	conn := sim.NewDirectConnection(
		b.gpuName+".L2TLBtoGMMUCache",
		b.engine, b.freq,
	)
	conn.PlugIn(b.gmmuCache.GetPortByName("Top"), 64)

	for _, l2TLB := range b.l2TLBs {
		conn.PlugIn(l2TLB.GetPortByName("Bottom"), 64)
	}
}

func (b *R9NanoGPUBuilder) connectGMMUCachetoGMMU() {
	conn := sim.NewDirectConnection(
		b.gpuName+".GMMUCacheToGMMU",
		b.engine, b.freq,
	)
	conn.PlugIn(b.gmmu.GetPortByName("Top"), 64)
	conn.PlugIn(b.gmmuCache.GetPortByName("Bottom"), 64)
}
