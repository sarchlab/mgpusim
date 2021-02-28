package main

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/cache/writeback"
	"gitlab.com/akita/mem/dram"
	"gitlab.com/akita/mem/vm/addresstranslator"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/mem/vm/tlb"
	"gitlab.com/akita/mgpusim"
	"gitlab.com/akita/mgpusim/pagemigrationcontroller"
	"gitlab.com/akita/mgpusim/rdma"
	"gitlab.com/akita/mgpusim/timing/caches/l1v"
	"gitlab.com/akita/mgpusim/timing/caches/rob"
	"gitlab.com/akita/mgpusim/timing/caches/writearound"
	"gitlab.com/akita/mgpusim/timing/cp"
	"gitlab.com/akita/mgpusim/timing/cu"
	"gitlab.com/akita/util/tracing"
)

// R9NanoGPUBuilder can build R9 Nano GPUs.
type R9NanoGPUBuilder struct {
	engine                         akita.Engine
	freq                           akita.Freq
	memAddrOffset                  uint64
	mmu                            *mmu.MMUImpl
	numShaderArray                 int
	numCUPerShaderArray            int
	numMemoryBank                  int
	log2PageSize                   uint64
	log2CacheLineSize              uint64
	log2MemoryBankInterleavingSize uint64

	enableISADebugging bool
	enableMemTracing   bool
	enableVisTracing   bool
	visTracer          tracing.Tracer
	memTracer          tracing.Tracer

	gpuName                 string
	gpu                     *mgpusim.GPU
	cp                      *cp.CommandProcessor
	cus                     []*cu.ComputeUnit
	l1vReorderBuffers       []*rob.ReorderBuffer
	l1iReorderBuffers       []*rob.ReorderBuffer
	l1sReorderBuffers       []*rob.ReorderBuffer
	l1vCaches               []*writearound.Cache
	l1sCaches               []*l1v.Cache
	l1iCaches               []*l1v.Cache
	l2Caches                []*writeback.Cache
	l1vAddrTrans            []*addresstranslator.AddressTranslator
	l1sAddrTrans            []*addresstranslator.AddressTranslator
	l1iAddrTrans            []*addresstranslator.AddressTranslator
	l1vTLBs                 []*tlb.TLB
	l1sTLBs                 []*tlb.TLB
	l1iTLBs                 []*tlb.TLB
	l2TLBs                  []*tlb.TLB
	drams                   []*dram.MemController
	lowModuleFinderForL1    *cache.InterleavedLowModuleFinder
	lowModuleFinderForL2    *cache.InterleavedLowModuleFinder
	lowModuleFinderForPMC   *cache.InterleavedLowModuleFinder
	dmaEngine               *cp.DMAEngine
	rdmaEngine              *rdma.Engine
	pageMigrationController *pagemigrationcontroller.PageMigrationController

	internalConn           *akita.DirectConnection
	l1TLBToL2TLBConnection *akita.DirectConnection
	l1ToL2Connection       *akita.DirectConnection
	l2ToDramConnection     *akita.DirectConnection
}

// MakeR9NanoGPUBuilder provides a GPU builder that can builds the R9Nano GPU.
func MakeR9NanoGPUBuilder() R9NanoGPUBuilder {
	b := R9NanoGPUBuilder{
		freq:                           1 * akita.GHz,
		numShaderArray:                 16,
		numCUPerShaderArray:            4,
		numMemoryBank:                  8,
		log2CacheLineSize:              6,
		log2PageSize:                   12,
		log2MemoryBankInterleavingSize: 12,
	}
	return b
}

// WithEngine sets the engine that the GPU use.
func (b R9NanoGPUBuilder) WithEngine(engine akita.Engine) R9NanoGPUBuilder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency that the GPU works at.
func (b R9NanoGPUBuilder) WithFreq(freq akita.Freq) R9NanoGPUBuilder {
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
func (b R9NanoGPUBuilder) WithMMU(mmu *mmu.MMUImpl) R9NanoGPUBuilder {
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

// Build creates a pre-configure GPU similar to the AMD R9 Nano GPU.
func (b R9NanoGPUBuilder) Build(name string, id uint64) *mgpusim.GPU {
	b.createGPU(name, id)
	b.buildSAs()
	b.buildL2Caches()
	b.buildDRAMControllers()
	b.buildCP()
	b.buildL2TLB()

	b.connectCP()
	b.connectL2AndDRAM()
	b.connectL1ToL2()
	b.connectL1TLBToL2TLB()

	b.gpu.InternalConnection = b.internalConn

	return b.gpu
}

func (b *R9NanoGPUBuilder) createGPU(name string, id uint64) {
	b.gpuName = name

	b.gpu = mgpusim.NewGPU(b.gpuName)

	b.gpu.GPUID = id
}

func (b *R9NanoGPUBuilder) connectCP() {
	b.internalConn = akita.NewDirectConnection(
		b.gpuName+"InternalConn", b.engine, b.freq)
	b.gpu.InternalConnection = b.internalConn

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

	b.cp.PMC = b.pageMigrationController.CtrlPort
	b.internalConn.PlugIn(b.pageMigrationController.CtrlPort, 1)

	b.connectCPWithCUs()
	b.connectCPWithAddressTranslators()
	b.connectCPWithTLBs()
	b.connectCPWithCaches()
}

func (b *R9NanoGPUBuilder) connectL1ToL2() {
	lowModuleFinder := cache.NewInterleavedLowModuleFinder(
		1 << b.log2MemoryBankInterleavingSize)
	lowModuleFinder.ModuleForOtherAddresses = b.rdmaEngine.ToL1
	lowModuleFinder.UseAddressSpaceLimitation = true
	lowModuleFinder.LowAddress = b.memAddrOffset
	lowModuleFinder.HighAddress = b.memAddrOffset + 4*mem.GB

	l1ToL2Conn := akita.NewDirectConnection(b.gpuName+".L1-L2",
		b.engine, b.freq)

	b.rdmaEngine.SetLocalModuleFinder(lowModuleFinder)
	l1ToL2Conn.PlugIn(b.rdmaEngine.ToL1, 64)
	l1ToL2Conn.PlugIn(b.rdmaEngine.ToL2, 64)

	for _, l2 := range b.l2Caches {
		lowModuleFinder.LowModules = append(lowModuleFinder.LowModules,
			l2.TopPort)
		l1ToL2Conn.PlugIn(l2.TopPort, 64)
	}

	for _, l1v := range b.l1vCaches {
		l1v.SetLowModuleFinder(lowModuleFinder)
		l1ToL2Conn.PlugIn(l1v.BottomPort, 16)
	}

	for _, l1s := range b.l1sCaches {
		l1s.SetLowModuleFinder(lowModuleFinder)
		l1ToL2Conn.PlugIn(l1s.BottomPort, 16)
	}

	for _, l1iAT := range b.l1iAddrTrans {
		l1iAT.SetLowModuleFinder(lowModuleFinder)
		l1ToL2Conn.PlugIn(l1iAT.BottomPort, 16)
	}
}

func (b *R9NanoGPUBuilder) connectL2AndDRAM() {
	b.l2ToDramConnection = akita.NewDirectConnection(
		b.gpuName+"L2-DRAM", b.engine, b.freq)

	lowModuleFinder := cache.NewInterleavedLowModuleFinder(
		1 << b.log2MemoryBankInterleavingSize)

	for i, l2 := range b.l2Caches {
		b.l2ToDramConnection.PlugIn(l2.BottomPort, 64)
		l2.SetLowModuleFinder(&cache.SingleLowModuleFinder{
			LowModule: b.drams[i].TopPort,
		})
	}

	for _, dram := range b.drams {
		b.l2ToDramConnection.PlugIn(dram.TopPort, 64)
		lowModuleFinder.LowModules = append(lowModuleFinder.LowModules,
			dram.TopPort)
	}

	b.dmaEngine.SetLocalDataSource(lowModuleFinder)
	b.l2ToDramConnection.PlugIn(b.dmaEngine.ToMem, 64)

	b.pageMigrationController.MemCtrlFinder = lowModuleFinder
	b.l2ToDramConnection.PlugIn(b.pageMigrationController.LocalMemPort, 16)
}

func (b *R9NanoGPUBuilder) connectL1TLBToL2TLB() {
	tlbConn := akita.NewDirectConnection(b.gpuName+"L1TLB-L2TLB",
		b.engine, b.freq)

	tlbConn.PlugIn(b.l2TLBs[0].TopPort, 64)

	for _, l1vTLB := range b.l1vTLBs {
		l1vTLB.LowModule = b.l2TLBs[0].TopPort
		tlbConn.PlugIn(l1vTLB.BottomPort, 16)
	}

	for _, l1iTLB := range b.l1iTLBs {
		l1iTLB.LowModule = b.l2TLBs[0].TopPort
		tlbConn.PlugIn(l1iTLB.BottomPort, 16)
	}

	for _, l1sTLB := range b.l1sTLBs {
		l1sTLB.LowModule = b.l2TLBs[0].TopPort
		tlbConn.PlugIn(l1sTLB.BottomPort, 16)
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
		b.cp.AddressTranslators = append(b.cp.AddressTranslators, at.CtrlPort)
		b.internalConn.PlugIn(at.CtrlPort, 1)
	}

	for _, at := range b.l1sAddrTrans {
		b.cp.AddressTranslators = append(b.cp.AddressTranslators, at.CtrlPort)
		b.internalConn.PlugIn(at.CtrlPort, 1)
	}

	for _, at := range b.l1iAddrTrans {
		b.cp.AddressTranslators = append(b.cp.AddressTranslators, at.CtrlPort)
		b.internalConn.PlugIn(at.CtrlPort, 1)
	}

	for _, rob := range b.l1vReorderBuffers {
		b.cp.AddressTranslators = append(
			b.cp.AddressTranslators, rob.ControlPort)
		b.internalConn.PlugIn(rob.ControlPort, 1)
	}

	for _, rob := range b.l1iReorderBuffers {
		b.cp.AddressTranslators = append(
			b.cp.AddressTranslators, rob.ControlPort)
		b.internalConn.PlugIn(rob.ControlPort, 1)
	}

	for _, rob := range b.l1sReorderBuffers {
		b.cp.AddressTranslators = append(
			b.cp.AddressTranslators, rob.ControlPort)
		b.internalConn.PlugIn(rob.ControlPort, 1)
	}
}

func (b *R9NanoGPUBuilder) connectCPWithTLBs() {
	for _, tlb := range b.l2TLBs {
		b.cp.TLBs = append(b.cp.TLBs, tlb.ControlPort)
		b.internalConn.PlugIn(tlb.ControlPort, 1)
	}

	for _, tlb := range b.l1vTLBs {
		b.cp.TLBs = append(b.cp.TLBs, tlb.ControlPort)
		b.internalConn.PlugIn(tlb.ControlPort, 1)
	}

	for _, tlb := range b.l1sTLBs {
		b.cp.TLBs = append(b.cp.TLBs, tlb.ControlPort)
		b.internalConn.PlugIn(tlb.ControlPort, 1)
	}

	for _, tlb := range b.l1iTLBs {
		b.cp.TLBs = append(b.cp.TLBs, tlb.ControlPort)
		b.internalConn.PlugIn(tlb.ControlPort, 1)
	}
}

func (b *R9NanoGPUBuilder) connectCPWithCaches() {
	for _, c := range b.l1iCaches {
		b.cp.L1ICaches = append(b.cp.L1ICaches, c.ControlPort)
		b.internalConn.PlugIn(c.ControlPort, 1)
	}

	for _, c := range b.l1vCaches {
		b.cp.L1VCaches = append(b.cp.L1VCaches, c.ControlPort)
		b.internalConn.PlugIn(c.ControlPort, 1)
	}

	for _, c := range b.l1sCaches {
		b.cp.L1SCaches = append(b.cp.L1SCaches, c.ControlPort)
		b.internalConn.PlugIn(c.ControlPort, 1)
	}

	for _, c := range b.l2Caches {
		b.cp.L2Caches = append(b.cp.L2Caches, c.ControlPort)
		b.internalConn.PlugIn(c.ControlPort, 1)
	}
}

func (b *R9NanoGPUBuilder) buildSAs() {
	saBuilder := makeShaderArrayBuilder().
		withEngine(b.engine).
		withFreq(b.freq).
		withGPUID(b.gpu.GPUID).
		withLog2CachelineSize(b.log2CacheLineSize).
		withLog2PageSize(b.log2PageSize).
		withNumCU(b.numCUPerShaderArray)

	if b.enableVisTracing {
		saBuilder = saBuilder.withVisTracer(b.visTracer)
	}

	if b.enableMemTracing {
		saBuilder = saBuilder.withMemTracer(b.memTracer)
	}

	for i := 0; i < b.numShaderArray; i++ {
		saName := fmt.Sprintf("%s.SA_%02d", b.gpuName, i)
		b.buildSA(saBuilder, saName)
	}
}

func (b *R9NanoGPUBuilder) buildL2Caches() {
	l2Builder := writeback.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(16).
		WithByteSize(256 * mem.KB).
		WithNumMSHREntry(64).
		WithNumReqPerCycle(1)

	for i := 0; i < b.numMemoryBank; i++ {
		cacheName := fmt.Sprintf("%s.L2_%d", b.gpuName, i)
		l2 := l2Builder.Build(cacheName)
		b.l2Caches = append(b.l2Caches, l2)
		b.gpu.L2Caches = append(b.gpu.L2Caches, l2)

		if b.enableVisTracing {
			tracing.CollectTrace(l2, b.visTracer)
		}

		if b.enableMemTracing {
			tracing.CollectTrace(l2, b.memTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) buildDRAMControllers() {
	memCtrlBuilder := b.createDramControllerBuilder()

	for i := 0; i < b.numMemoryBank; i++ {
		dramName := fmt.Sprintf("%s.DRAM_%d", b.gpuName, i)
		dram := memCtrlBuilder.
			WithInterleavingAddrConversion(
				1<<b.log2MemoryBankInterleavingSize,
				b.numMemoryBank,
				i, b.memAddrOffset, b.memAddrOffset+4*mem.GB,
			).
			Build(dramName)
		// dram := idealmemcontroller.New(
		// 	fmt.Sprintf("%s.DRAM_%d", b.gpuName, i),
		// 	b.engine, 512*mem.MB)
		b.drams = append(b.drams, dram)
		b.gpu.MemoryControllers = append(b.gpu.MemoryControllers, dram)

		if b.enableVisTracing {
			tracing.CollectTrace(dram, b.visTracer)
		}

		if b.enableMemTracing {
			tracing.CollectTrace(dram, b.memTracer)
		}
	}
}

func (b *R9NanoGPUBuilder) createDramControllerBuilder() dram.Builder {
	memBankSize := 4 * mem.GB / uint64(b.numMemoryBank)
	if 4*mem.GB%uint64(b.numMemoryBank) != 0 {
		panic("GPU memory size is not a multiple of the number of memory banks")
	}
	dramCol := 64
	dramRow := 4096
	dramDeviceWidth := 32
	dramBankSize := dramCol * dramRow * dramDeviceWidth
	dramBank := 4
	dramBankGroup := 1
	dramBusWidth := 256
	dramDevicePerRank := dramBusWidth / dramDeviceWidth
	dramRankSize := dramBankSize * dramDevicePerRank * dramBank
	dramRank := int(memBankSize) / dramRankSize

	memCtrlBuilder := dram.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(500 * akita.MHz).
		WithProtocol(dram.GDDR5).
		WithBurstLength(8).
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
		WithTCL(24).
		WithTCWL(7).
		WithTRCDRD(18).
		WithTRCDWR(15).
		WithTRP(18).
		WithTRAS(42).
		WithTREFI(11699).
		WithTRRDS(9).
		WithTRRDL(9).
		WithTWTRS(8).
		WithTWTRL(8).
		WithTWR(18).
		WithTCCDS(2).
		WithTCCDL(3).
		WithTRTRS(0).
		WithTRTP(3).
		WithTPPD(2)

	if b.visTracer != nil {
		memCtrlBuilder = memCtrlBuilder.WithAdditionalTracer(b.visTracer)
	}

	return memCtrlBuilder
}

func (b *R9NanoGPUBuilder) buildSA(
	saBuilder shaderArrayBuilder,
	saName string,
) {
	sa := saBuilder.Build(saName)

	for _, cu := range sa.cus {
		b.gpu.CUs = append(b.gpu.CUs, cu)
		b.cus = append(b.cus, cu)
	}

	for _, rob := range sa.l1vROBs {
		b.l1vReorderBuffers = append(b.l1vReorderBuffers, rob)
		b.gpu.L1VROBs = append(b.gpu.L1VROBs, rob)
	}

	for _, tlb := range sa.l1vTLBs {
		b.l1vTLBs = append(b.l1vTLBs, tlb)
		b.gpu.L1VTLBs = append(b.gpu.L1VTLBs, tlb)
	}

	for _, l1v := range sa.l1vCaches {
		b.l1vCaches = append(b.l1vCaches, l1v)
		b.gpu.L1VCaches = append(b.gpu.L1VCaches, l1v)
	}

	for _, at := range sa.l1vATs {
		b.l1vAddrTrans = append(b.l1vAddrTrans, at)
		b.gpu.L1VAddrTranslator = append(b.gpu.L1VAddrTranslator, at)
	}

	b.l1sAddrTrans = append(b.l1sAddrTrans, sa.l1sAT)
	b.gpu.L1SAddrTranslator = append(b.gpu.L1SAddrTranslator, sa.l1sAT)
	b.l1sReorderBuffers = append(b.l1sReorderBuffers, sa.l1sROB)
	b.gpu.L1SROBs = append(b.gpu.L1SROBs, sa.l1sROB)
	b.l1sCaches = append(b.l1sCaches, sa.l1sCache)
	b.gpu.L1SCaches = append(b.gpu.L1SCaches, sa.l1sCache)
	b.l1sTLBs = append(b.l1sTLBs, sa.l1sTLB)
	b.gpu.L1STLBs = append(b.gpu.L1STLBs, sa.l1sTLB)

	b.l1iAddrTrans = append(b.l1iAddrTrans, sa.l1iAT)
	b.gpu.L1IAddrTranslator = append(b.gpu.L1IAddrTranslator, sa.l1iAT)
	b.l1iReorderBuffers = append(b.l1iReorderBuffers, sa.l1iROB)
	b.gpu.L1IROBs = append(b.gpu.L1IROBs, sa.l1iROB)
	b.l1iCaches = append(b.l1iCaches, sa.l1iCache)
	b.gpu.L1ICaches = append(b.gpu.L1ICaches, sa.l1iCache)
	b.l1iTLBs = append(b.l1iTLBs, sa.l1iTLB)
	b.gpu.L1ITLBs = append(b.gpu.L1ITLBs, sa.l1iTLB)
}

func (b *R9NanoGPUBuilder) buildRDMAEngine() {
	b.rdmaEngine = rdma.NewEngine(
		fmt.Sprintf("%s.RDMA", b.gpuName),
		b.engine,
		b.lowModuleFinderForL1,
		nil,
	)
	b.gpu.RDMAEngine = b.rdmaEngine
}

func (b *R9NanoGPUBuilder) buildPageMigrationController() {
	b.pageMigrationController =
		pagemigrationcontroller.NewPageMigrationController(
			fmt.Sprintf("%s.PMC", b.gpuName),
			b.engine,
			b.lowModuleFinderForPMC,
			nil)

	b.gpu.PMC = b.pageMigrationController
}

func (b *R9NanoGPUBuilder) buildDMAEngine() {
	b.dmaEngine = cp.NewDMAEngine(
		fmt.Sprintf("%s.DMA", b.gpuName),
		b.engine,
		nil)

	if b.enableVisTracing {
		tracing.CollectTrace(b.dmaEngine, b.visTracer)
	}
}

func (b *R9NanoGPUBuilder) buildCP() {
	builder := cp.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq)

	if b.enableVisTracing {
		builder = builder.WithVisTracer(b.visTracer)
	}

	b.cp = builder.Build(b.gpuName + ".CommandProcessor")
	b.gpu.CommandProcessor = b.cp

	b.buildDMAEngine()
	b.buildRDMAEngine()
	b.buildPageMigrationController()
}

func (b *R9NanoGPUBuilder) buildL2TLB() {
	builder := tlb.MakeBuilder().
		WithEngine(b.engine).
		WithFreq(b.freq).
		WithNumWays(64).
		WithNumSets(64).
		WithNumMSHREntry(64).
		WithNumReqPerCycle(1024).
		WithPageSize(1 << b.log2PageSize).
		WithLowModule(b.mmu.ToTop)

	l2TLB := builder.Build(fmt.Sprintf("%s.L2TLB", b.gpuName))
	b.l2TLBs = append(b.l2TLBs, l2TLB)
	b.gpu.L2TLBs = append(b.gpu.L2TLBs, l2TLB)

	if b.enableVisTracing {
		tracing.CollectTrace(l2TLB, b.visTracer)
	}
}

func (b *R9NanoGPUBuilder) numCU() int {
	return b.numCUPerShaderArray * b.numShaderArray
}

func (b *R9NanoGPUBuilder) connectWithDirectConnection(
	port1, port2 akita.Port,
	bufferSize int,
) {
	conn := akita.NewDirectConnection(
		port1.Name()+"-"+port2.Name(),
		b.engine, b.freq,
	)
	conn.PlugIn(port1, bufferSize)
	conn.PlugIn(port2, bufferSize)
}
