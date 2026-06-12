// Package shaderarray provides a builder for a shader array.
package shaderarray

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/mgpusim/v4/domain"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/timing/cu"
	"github.com/sarchlab/mgpusim/v4/amd/timing/rob"
)

// Builder builds a shader array.
type Builder struct {
	simulation *simulation.Simulation

	gpuID              uint64
	name               string
	numCUs             int
	freq               sim.Freq
	log2CacheLineSize  uint64
	log2PageSize       uint64
	wfPoolSize                int
	vgprCount                 []int
	numSinglePrecisionUnits   int
	vecMemInstPipelineStages   int
	vecMemTransPipelineStages  int
	vecMemTransPipelineWidth   int
	cuMemPipelineBufferSize    int
	l1vCacheSize               uint64
	l1vNumMSHREntry           int
	l1vBankLatency            int
	l1vMissFillExtraLatency   int
	memPipelineBufferSize     int
	inFlightVectorMemAccessLimit int
	maxCoalescingPenalty      int
	scratchLatency            int
	registerScoreboard        bool
	isCDNA3                   bool
	l1AddressMapper           mem.AddressToPortMapper
	l1TLBAddressMapper        mem.AddressToPortMapper
	aluFactory                emu.ALUFactory

	sa        *domain.Domain
	cus       []*cu.ComputeUnit
	l1vROBs   []*rob.ReorderBuffer
	l1sROB    *rob.ReorderBuffer
	l1iROB    *rob.ReorderBuffer
	l1vATs    []*modeling.Component[addresstranslator.Spec, addresstranslator.State]
	l1sAT     *modeling.Component[addresstranslator.Spec, addresstranslator.State]
	l1iAT     *modeling.Component[addresstranslator.Spec, addresstranslator.State]
	l1vCaches []*modeling.Component[writethroughcache.Spec, writethroughcache.State]
	l1sCache  *modeling.Component[writethroughcache.Spec, writethroughcache.State]
	l1iCache  *modeling.Component[writethroughcache.Spec, writethroughcache.State]
	l1vTLBs   []*modeling.Component[tlb.Spec, tlb.State]
	l1sTLB    *modeling.Component[tlb.Spec, tlb.State]
	l1iTLB    *modeling.Component[tlb.Spec, tlb.State]

	// Mapper pointers to allow left-to-right component build order
	// Vector path: ROB -> AT -(mem)-> L1V Cache, AT -(xlate)-> L1V TLB
	l1vMemMappers   []*mem.SinglePortMapper
	l1vTransMappers []*mem.SinglePortMapper

	// Scalar path: ROB -> AT -(mem)-> L1S Cache, AT -(xlate)-> L1S TLB
	l1sMemMapper   *mem.SinglePortMapper
	l1sTransMapper *mem.SinglePortMapper

	// Instruction path: ROB -> L1I Cache -(mem)-> AT -(xlate)-> L1I TLB
	l1iCacheMapper *mem.SinglePortMapper
	l1iTransMapper *mem.SinglePortMapper

	connectionCount int
}

// MakeBuilder creates a new builder.
func MakeBuilder() Builder {
	return Builder{
		numCUs:            4,
		freq:              1 * sim.GHz,
		log2CacheLineSize: 6,
		log2PageSize:      12,
	}
}

// WithSimulation sets the simulation to use.
func (b Builder) WithSimulation(sim *simulation.Simulation) Builder {
	b.simulation = sim
	return b
}

// WithGPUID sets the GPU ID to use.
func (b Builder) WithGPUID(gpuID uint64) Builder {
	b.gpuID = gpuID
	return b
}

// WithNumCUs sets the number of CUs to use.
func (b Builder) WithNumCUs(numCUs int) Builder {
	b.numCUs = numCUs
	return b
}

// WithFreq sets the frequency to use.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithLog2CacheLineSize sets the log2 cache line size to use.
func (b Builder) WithLog2CacheLineSize(log2CacheLineSize uint64) Builder {
	b.log2CacheLineSize = log2CacheLineSize
	return b
}

// WithLog2PageSize sets the log2 page size to use.
func (b Builder) WithLog2PageSize(log2PageSize uint64) Builder {
	b.log2PageSize = log2PageSize
	return b
}

// WithL1AddressMapper sets the L1 address mapper to use.
func (b Builder) WithL1AddressMapper(
	l1AddressMapper mem.AddressToPortMapper,
) Builder {
	b.l1AddressMapper = l1AddressMapper
	return b
}

// WithL1TLBAddressMapper sets the L1 TLB address mapper to use.
func (b Builder) WithL1TLBAddressMapper(
	l1TLBAddressMapper mem.AddressToPortMapper,
) Builder {
	b.l1TLBAddressMapper = l1TLBAddressMapper
	return b
}

// WithWfPoolSize sets the wavefront pool size for the CU builder.
func (b Builder) WithWfPoolSize(n int) Builder {
	b.wfPoolSize = n
	return b
}

// WithVGPRCount sets the VGPR counts for the CU builder.
func (b Builder) WithVGPRCount(counts []int) Builder {
	b.vgprCount = counts
	return b
}

// WithALUFactory sets the ALU factory for creating compute unit ALUs.
// This allows using different ALU implementations for different architectures.
func (b Builder) WithALUFactory(factory emu.ALUFactory) Builder {
	b.aluFactory = factory
	return b
}

// WithNumSinglePrecisionUnits sets the number of single-precision units per
// SIMD in each CU.
func (b Builder) WithNumSinglePrecisionUnits(n int) Builder {
	b.numSinglePrecisionUnits = n
	return b
}

// WithVecMemInstPipelineStages sets the vector memory instruction pipeline
// depth for each CU.
func (b Builder) WithVecMemInstPipelineStages(n int) Builder {
	b.vecMemInstPipelineStages = n
	return b
}

// WithVecMemTransPipelineStages sets the vector memory transaction pipeline
// depth for each CU.
func (b Builder) WithVecMemTransPipelineStages(n int) Builder {
	b.vecMemTransPipelineStages = n
	return b
}

// WithVecMemTransPipelineWidth sets the width (items per cycle) of the
// vector memory transaction pipeline for each CU. Default is 1.
func (b Builder) WithVecMemTransPipelineWidth(n int) Builder {
	b.vecMemTransPipelineWidth = n
	return b
}

// WithCUMemPipelineBufferSize sets the CU-internal post-pipeline buffer
// size for vector memory transactions. Default is 8.
func (b Builder) WithCUMemPipelineBufferSize(n int) Builder {
	b.cuMemPipelineBufferSize = n
	return b
}

// WithL1VCacheSize sets the L1V cache size per CU in bytes.
func (b Builder) WithL1VCacheSize(size uint64) Builder {
	b.l1vCacheSize = size
	return b
}

// WithL1VNumMSHREntry sets the number of MSHR entries for each L1V cache.
// If not set (or set to 0), the default of 32 is used.
func (b Builder) WithL1VNumMSHREntry(n int) Builder {
	b.l1vNumMSHREntry = n
	return b
}

// WithL1VBankLatency sets the L1V cache bank latency in cycles.
func (b Builder) WithL1VBankLatency(latency int) Builder {
	b.l1vBankLatency = latency
	return b
}

// WithL1VMissFillExtraLatency sets extra cycles added to L1V miss-fill
// read transactions after bank pipeline exit.
func (b Builder) WithL1VMissFillExtraLatency(n int) Builder {
	b.l1vMissFillExtraLatency = n
	return b
}

// WithMemPipelineBufferSize sets the buffer size for memory pipeline
// connections (CU→ROB→AT→L1V). Larger values allow more concurrent
// memory transactions, improving throughput for bandwidth-limited workloads.
func (b Builder) WithMemPipelineBufferSize(size int) Builder {
	b.memPipelineBufferSize = size
	return b
}

// WithInFlightVectorMemAccessLimit sets the maximum number of in-flight
// vector memory access transactions per CU. Default is 512.
func (b Builder) WithInFlightVectorMemAccessLimit(limit int) Builder {
	b.inFlightVectorMemAccessLimit = limit
	return b
}

// WithMaxCoalescingPenalty sets the maximum coalescing penalty in cycles
// for poorly-coalesced read transactions in each CU.
func (b Builder) WithMaxCoalescingPenalty(n int) Builder {
	b.maxCoalescingPenalty = n
	return b
}

// WithScratchLatency sets the latency in cycles for SCRATCH segment
// memory operations in each CU.
func (b Builder) WithScratchLatency(n int) Builder {
	b.scratchLatency = n
	return b
}

// WithRegisterScoreboard enables or disables the register scoreboard and
// SIMD pipelining feature in each CU.
func (b Builder) WithRegisterScoreboard(enabled bool) Builder {
	b.registerScoreboard = enabled
	return b
}

// WithIsCDNA3 sets whether the CUs in this shader array should use CDNA3
// ISA decoding. When enabled, each CU's disassembler uses CDNA3-specific
// instruction formats.
func (b Builder) WithIsCDNA3(cdna3 bool) Builder {
	b.isCDNA3 = cdna3
	return b
}

// Build builds the shader array.
func (b Builder) Build(name string) *domain.Domain {
	b.name = name
	b.sa = domain.NewDomain(name)

	b.buildComponents()
	b.connectComponents()

	return b.sa
}

func (b *Builder) buildComponents() {
	b.buildCUs()

	// V5 resolves address-mapper specs at Build() time, so every
	// component that a mapper points to must already exist.
	//
	// Vector path:  CU → ROB → AT → L1V cache → L2
	//                         ↘ L1V TLB → L2TLB
	//   AT mapper-mem  → L1V cache top port
	//   AT mapper-xlate → L1V TLB top port
	//   → Build cache & TLB first, then AT.
	b.buildL1VReorderBuffers()
	b.buildL1VCaches()
	b.buildL1VTLBs()
	b.buildL1VAddressTranslators()

	// Scalar path:  same pattern as vector
	b.buildL1SReorderBuffer()
	b.buildL1SCache()
	b.buildL1STLB()
	b.buildL1SAddressTranslator()

	// Instruction path:  CU → ROB → L1I cache → AT → L2
	//                                            ↘ L1I TLB → L2TLB
	//   L1I cache mapper → AT top port
	//   AT mapper-xlate  → L1I TLB top port
	//   → Build TLB first, then AT, then cache.
	b.buildL1IReorderBuffer()
	b.buildL1ITLB()
	b.buildL1IAddressTranslator()
	b.buildL1ICache()

	b.populateExternalPorts()
}

func (b *Builder) populateExternalPorts() {
	for i := range b.numCUs {
		cu := b.cus[i]

		b.sa.AddPort(fmt.Sprintf("CU[%d]", i), cu.GetPortByName("Top"))
		b.sa.AddPort(fmt.Sprintf("CUCtrl[%d]", i), cu.GetPortByName("Ctrl"))
		b.sa.AddPort(fmt.Sprintf("L1VROBCtrl[%d]", i), b.l1vROBs[i].
			GetPortByName("Control"))
		b.sa.AddPort(fmt.Sprintf("L1VAddrTransCtrl[%d]", i),
			b.l1vATs[i].GetPortByName("Control"))
		b.sa.AddPort(fmt.Sprintf("L1VTLBCtrl[%d]", i),
			b.l1vTLBs[i].GetPortByName("Control"))
		b.sa.AddPort(fmt.Sprintf("L1VCacheCtrl[%d]", i),
			b.l1vCaches[i].GetPortByName("Control"))
		b.sa.AddPort(fmt.Sprintf("L1VCacheBottom[%d]", i),
			b.l1vCaches[i].GetPortByName("Bottom"))
		b.sa.AddPort(fmt.Sprintf("L1VTLBBottom[%d]", i),
			b.l1vTLBs[i].GetPortByName("Bottom"))
	}

	b.sa.AddPort("L1SROBCtrl", b.l1sROB.GetPortByName("Control"))
	b.sa.AddPort("L1SAddrTransCtrl", b.l1sAT.GetPortByName("Control"))
	b.sa.AddPort("L1STLBCtrl", b.l1sTLB.GetPortByName("Control"))
	b.sa.AddPort("L1SCacheCtrl", b.l1sCache.GetPortByName("Control"))
	b.sa.AddPort("L1SCacheBottom", b.l1sCache.GetPortByName("Bottom"))
	b.sa.AddPort("L1STLBBottom", b.l1sTLB.GetPortByName("Bottom"))

	b.sa.AddPort("L1IROBCtrl", b.l1iROB.GetPortByName("Control"))
	b.sa.AddPort("L1IAddrTransCtrl", b.l1iAT.GetPortByName("Control"))
	b.sa.AddPort("L1ITLBCtrl", b.l1iTLB.GetPortByName("Control"))
	b.sa.AddPort("L1ICacheCtrl", b.l1iCache.GetPortByName("Control"))
	// Expose instruction memory egress to L2 via AT bottom
	b.sa.AddPort("L1ICacheBottom", b.l1iAT.GetPortByName("Bottom"))
	b.sa.AddPort("L1ITLBBottom", b.l1iTLB.GetPortByName("Bottom"))
}

func (b *Builder) connectComponents() {
	b.connectVectorMem()
	b.connectScalarMem()
	b.connectInstMem()
}

func (b *Builder) connectVectorMem() {
	bufSize := 8
	if b.memPipelineBufferSize > 0 {
		bufSize = b.memPipelineBufferSize
	}

	for i := range b.numCUs {
		cu := b.cus[i]
		rob := b.l1vROBs[i]
		at := b.l1vATs[i]
		l1v := b.l1vCaches[i]
		tlb := b.l1vTLBs[i]

		// Mapper ports are set at build time now (V5).

		cu.VectorMemModules = &mem.SinglePortMapper{
			Port: rob.GetPortByName("Top").AsRemote(),
		}
		b.connectWithDirectConnection(cu.ToVectorMem,
			rob.GetPortByName("Top"), bufSize)

		atTopPort := at.GetPortByName("Top")
		rob.BottomUnit = atTopPort.AsRemote()
		b.connectWithDirectConnection(
			rob.GetPortByName("Bottom"), atTopPort, bufSize)

		tlbTopPort := tlb.GetPortByName("Top")
		b.connectWithDirectConnection(
			at.GetPortByName("Translation"), tlbTopPort, bufSize)

		b.connectWithDirectConnection(l1v.GetPortByName("Top"),
			at.GetPortByName("Bottom"), bufSize)
	}
}

func (b *Builder) connectScalarMem() {
	rob := b.l1sROB
	at := b.l1sAT
	l1sTLB := b.l1sTLB
	l1s := b.l1sCache

	// Mapper ports are set at build time now (V5).

	atTopPort := at.GetPortByName("Top")
	rob.BottomUnit = atTopPort.AsRemote()
	b.connectWithDirectConnection(rob.GetPortByName("Bottom"), atTopPort, 32)

	tlbTopPort := l1sTLB.GetPortByName("Top")
	b.connectWithDirectConnection(
		at.GetPortByName("Translation"), tlbTopPort, 32)
	b.connectWithDirectConnection(
		l1s.GetPortByName("Top"), at.GetPortByName("Bottom"), 32)

	conn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(b.name + ".ScalarMemConn")
	b.simulation.RegisterComponent(conn)

	conn.PlugIn(rob.GetPortByName("Top"))
	for i := range b.numCUs {
		cu := b.cus[i]
		cu.ScalarMem = rob.GetPortByName("Top")
		conn.PlugIn(cu.ToScalarMem)
	}
}

func (b *Builder) connectInstMem() {
	rob := b.l1iROB
	at := b.l1iAT
	l1i := b.l1iCache

	// Mapper ports are set at build time now (V5).

	l1iTopPort := l1i.GetPortByName("Top")
	rob.BottomUnit = l1iTopPort.AsRemote()
	b.connectWithDirectConnection(rob.GetPortByName("Bottom"), l1iTopPort, 8)

	atTopPort := at.GetPortByName("Top")
	b.connectWithDirectConnection(l1i.GetPortByName("Bottom"), atTopPort, 8)

	l1iTLBTopPort := b.l1iTLB.GetPortByName("Top")
	b.connectWithDirectConnection(
		at.GetPortByName("Translation"), l1iTLBTopPort, 8)

	robTopPort := rob.GetPortByName("Top")
	conn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(b.name + ".InstMemConn")
	b.simulation.RegisterComponent(conn)

	conn.PlugIn(robTopPort)
	for i := range b.numCUs {
		cu := b.cus[i]
		cu.InstMem = rob.GetPortByName("Top")
		conn.PlugIn(cu.ToInstMem)
	}
}

func (b *Builder) connectWithDirectConnection(
	port1, port2 sim.Port,
	bufferSize int,
) {
	name := fmt.Sprintf("%s.Conn[%d]", b.name, b.connectionCount)
	b.connectionCount++

	conn := directconnection.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		Build(name)

	b.simulation.RegisterComponent(conn)

	conn.PlugIn(port1)
	conn.PlugIn(port2)
}

func (b *Builder) makeCUBuilder() cu.Builder {
	cuBuilder := cu.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithLog2CachelineSize(b.log2CacheLineSize)

	if b.aluFactory != nil {
		cuBuilder = cuBuilder.WithALUFactory(b.aluFactory)
	}

	if b.wfPoolSize > 0 {
		cuBuilder = cuBuilder.WithWfPoolSize(b.wfPoolSize)
	}

	if b.vgprCount != nil {
		cuBuilder = cuBuilder.WithVGPRCount(b.vgprCount)
	}

	if b.numSinglePrecisionUnits > 0 {
		cuBuilder = cuBuilder.WithNumSinglePrecisionUnits(b.numSinglePrecisionUnits)
	}

	if b.vecMemInstPipelineStages > 0 {
		cuBuilder = cuBuilder.WithVecMemInstPipelineStages(b.vecMemInstPipelineStages)
	}

	if b.vecMemTransPipelineStages > 0 {
		cuBuilder = cuBuilder.WithVecMemTransPipelineStages(b.vecMemTransPipelineStages)
	}

	if b.vecMemTransPipelineWidth > 0 {
		cuBuilder = cuBuilder.WithVecMemTransPipelineWidth(b.vecMemTransPipelineWidth)
	}

	if b.cuMemPipelineBufferSize > 0 {
		cuBuilder = cuBuilder.WithMemPipelineBufferSize(b.cuMemPipelineBufferSize)
	}

	if b.inFlightVectorMemAccessLimit > 0 {
		cuBuilder = cuBuilder.WithInFlightVectorMemAccessLimit(b.inFlightVectorMemAccessLimit)
	}

	if b.maxCoalescingPenalty > 0 {
		cuBuilder = cuBuilder.WithMaxCoalescingPenalty(b.maxCoalescingPenalty)
	}

	if b.scratchLatency > 0 {
		cuBuilder = cuBuilder.WithScratchLatency(b.scratchLatency)
	}

	if b.registerScoreboard {
		cuBuilder = cuBuilder.WithRegisterScoreboard(true)
	}

	if b.isCDNA3 {
		cuBuilder = cuBuilder.WithIsCDNA3(true)
	}

	return cuBuilder
}

func (b *Builder) buildCUs() {
	cuBuilder := b.makeCUBuilder()

	for i := 0; i < b.numCUs; i++ {
		cuName := fmt.Sprintf("%s.CU[%d]", b.name, i)
		computeUnit := cuBuilder.Build(cuName)
		b.cus = append(b.cus, computeUnit)
		b.simulation.RegisterComponent(computeUnit)
	}
}

func (b *Builder) buildL1VReorderBuffers() {
	builder := rob.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBufferSize(512).
		WithNumReqPerCycle(32)

	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VROB[%d]", b.name, i)
		rob := builder.Build(name)
		b.l1vROBs = append(b.l1vROBs, rob)
		b.simulation.RegisterComponent(rob)

		// if b.visTracer != nil {
		// 	tracing.CollectTrace(rob, b.visTracer)
		// }
	}
}

func (b *Builder) buildL1VAddressTranslators() {
	base := addresstranslator.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithDeviceID(b.gpuID).
		WithLog2PageSize(b.log2PageSize).
		WithNumReqPerCycle(32)

	b.l1vMemMappers = make([]*mem.SinglePortMapper, 0, b.numCUs)
	b.l1vTransMappers = make([]*mem.SinglePortMapper, 0, b.numCUs)

	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VAddrTrans[%d]", b.name, i)

		// Mappers must be populated before Build() – V5 freezes them
		// into the Spec at build time.
		memMapper := &mem.SinglePortMapper{
			Port: b.l1vCaches[i].GetPortByName("Top").AsRemote(),
		}
		xlateMapper := &mem.SinglePortMapper{
			Port: b.l1vTLBs[i].GetPortByName("Top").AsRemote(),
		}

		topPort := sim.NewPort(nil, 32, 32, name+".Top")
		bottomPort := sim.NewPort(nil, 32, 32, name+".Bottom")
		translationPort := sim.NewPort(nil, 32, 32, name+".Translation")
		ctrlPort := sim.NewPort(nil, 32, 32, name+".Control")
		curr := base.
			WithMemoryProviderMapper(memMapper).
			WithTranslationProviderMapper(xlateMapper).
			WithTopPort(topPort).
			WithBottomPort(bottomPort).
			WithTranslationPort(translationPort).
			WithCtrlPort(ctrlPort)
		at := curr.Build(name)
		b.l1vATs = append(b.l1vATs, at)
		b.l1vMemMappers = append(b.l1vMemMappers, memMapper)
		b.l1vTransMappers = append(b.l1vTransMappers, xlateMapper)
		b.simulation.RegisterComponent(at)
	}
}

func (b *Builder) buildL1VTLBs() {
	builder := tlb.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithNumMSHREntry(64).
		WithNumSets(4).
		WithNumWays(64).
		WithNumReqPerCycle(32).
		WithLatency(1).
		WithTranslationProviderMapper(b.l1TLBAddressMapper)

	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VTLB[%d]", b.name, i)
		topPort := sim.NewPort(nil, 32, 32, name+".Top")
		bottomPort := sim.NewPort(nil, 32, 32, name+".Bottom")
		controlPort := sim.NewPort(nil, 32, 32, name+".Control")
		tlb := builder.
			WithTopPort(topPort).
			WithBottomPort(bottomPort).
			WithControlPort(controlPort).
			Build(name)
		b.l1vTLBs = append(b.l1vTLBs, tlb)
		b.simulation.RegisterComponent(tlb)
	}
}

func (b *Builder) buildL1VCaches() {
	l1vSize := 16 * mem.KB
	if b.l1vCacheSize > 0 {
		l1vSize = b.l1vCacheSize
	}

	l1vBankLatency := 1 // reduced default from 20
	if b.l1vBankLatency > 0 {
		l1vBankLatency = b.l1vBankLatency
	}

	l1vMSHR := 64
	if b.l1vNumMSHREntry > 0 {
		l1vMSHR = b.l1vNumMSHREntry
	}

	builder := writethroughcache.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBankLatency(l1vBankLatency).
		WithNumBanks(4).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(4).
		WithNumMSHREntry(l1vMSHR).
		WithNumReqsPerCycle(4).
		WithMaxNumConcurrentTrans(4096).
		WithTotalByteSize(l1vSize).
		WithAddressToPortMapper(b.l1AddressMapper)

	if b.l1vMissFillExtraLatency > 0 {
		builder = builder.WithMissFillExtraLatency(b.l1vMissFillExtraLatency)
	}

	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VCache[%d]", b.name, i)
		topPort := sim.NewPort(nil, 64, 64, name+".Top")
		bottomPort := sim.NewPort(nil, 512, 512, name+".Bottom")
		controlPort := sim.NewPort(nil, 16, 16, name+".Control")
		cache := builder.
			WithTopPort(topPort).
			WithBottomPort(bottomPort).
			WithControlPort(controlPort).
			Build(name)
		b.l1vCaches = append(b.l1vCaches, cache)
		b.simulation.RegisterComponent(cache)

		// if b.memTracer != nil {
		// 	tracing.CollectTrace(cache, b.memTracer)
		// }
	}
}

func (b *Builder) buildL1SReorderBuffer() {
	builder := rob.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBufferSize(512).
		WithNumReqPerCycle(32)

	name := fmt.Sprintf("%s.L1SROB", b.name)
	rob := builder.Build(name)
	b.l1sROB = rob
	b.simulation.RegisterComponent(rob)
}

func (b *Builder) buildL1SAddressTranslator() {
	// Mappers populated from already-built cache/TLB (V5 freezes at build).
	b.l1sMemMapper = &mem.SinglePortMapper{
		Port: b.l1sCache.GetPortByName("Top").AsRemote(),
	}
	b.l1sTransMapper = &mem.SinglePortMapper{
		Port: b.l1sTLB.GetPortByName("Top").AsRemote(),
	}

	name := fmt.Sprintf("%s.L1SAddrTrans", b.name)
	topPort := sim.NewPort(nil, 32, 32, name+".Top")
	bottomPort := sim.NewPort(nil, 32, 32, name+".Bottom")
	translationPort := sim.NewPort(nil, 32, 32, name+".Translation")
	ctrlPort := sim.NewPort(nil, 32, 32, name+".Control")
	builder := addresstranslator.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithDeviceID(b.gpuID).
		WithLog2PageSize(b.log2PageSize).
		WithNumReqPerCycle(32).
		WithMemoryProviderMapper(b.l1sMemMapper).
		WithTranslationProviderMapper(b.l1sTransMapper).
		WithTopPort(topPort).
		WithBottomPort(bottomPort).
		WithTranslationPort(translationPort).
		WithCtrlPort(ctrlPort)

	at := builder.Build(name)
	b.l1sAT = at
	b.simulation.RegisterComponent(at)
}

func (b *Builder) buildL1STLB() {
	builder := tlb.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithNumMSHREntry(64).
		WithNumSets(1).
		WithNumWays(64).
		WithNumReqPerCycle(32).
		WithTranslationProviderMapper(b.l1TLBAddressMapper)

	name := fmt.Sprintf("%s.L1STLB", b.name)
	topPort := sim.NewPort(nil, 32, 32, name+".Top")
	bottomPort := sim.NewPort(nil, 32, 32, name+".Bottom")
	controlPort := sim.NewPort(nil, 32, 32, name+".Control")
	tlb := builder.
		WithTopPort(topPort).
		WithBottomPort(bottomPort).
		WithControlPort(controlPort).
		Build(name)
	b.l1sTLB = tlb
	b.simulation.RegisterComponent(tlb)
}

func (b *Builder) buildL1SCache() {
	builder := writethroughcache.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBankLatency(1).
		WithNumBanks(1).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(4).
		WithNumMSHREntry(128).
		WithNumReqsPerCycle(32).
		WithTotalByteSize(16 * mem.KB).
		WithAddressToPortMapper(b.l1AddressMapper)

	name := fmt.Sprintf("%s.L1SCache", b.name)
	topPort := sim.NewPort(nil, 32, 32, name+".Top")
	bottomPort := sim.NewPort(nil, 32, 32, name+".Bottom")
	controlPort := sim.NewPort(nil, 32, 32, name+".Control")
	cache := builder.
		WithTopPort(topPort).
		WithBottomPort(bottomPort).
		WithControlPort(controlPort).
		Build(name)
	b.l1sCache = cache
	b.simulation.RegisterComponent(cache)

	// if b.memTracer != nil {
	// 	tracing.CollectTrace(cache, b.memTracer)
	// }
}

func (b *Builder) buildL1IReorderBuffer() {
	builder := rob.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBufferSize(128).
		WithNumReqPerCycle(4)

	name := fmt.Sprintf("%s.L1IROB", b.name)
	rob := builder.Build(name)
	b.l1iROB = rob
	b.simulation.RegisterComponent(rob)
}

func (b *Builder) buildL1IAddressTranslator() {
	// TLB is already built; populate mapper from its top port.
	b.l1iTransMapper = &mem.SinglePortMapper{
		Port: b.l1iTLB.GetPortByName("Top").AsRemote(),
	}

	name := fmt.Sprintf("%s.L1IAddrTrans", b.name)
	topPort := sim.NewPort(nil, 16, 16, name+".Top")
	bottomPort := sim.NewPort(nil, 16, 16, name+".Bottom")
	translationPort := sim.NewPort(nil, 16, 16, name+".Translation")
	ctrlPort := sim.NewPort(nil, 16, 16, name+".Control")
	builder := addresstranslator.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithDeviceID(b.gpuID).
		WithLog2PageSize(b.log2PageSize).
		WithNumReqPerCycle(16).
		WithMemoryProviderMapper(b.l1AddressMapper).
		WithTranslationProviderMapper(b.l1iTransMapper).
		WithTopPort(topPort).
		WithBottomPort(bottomPort).
		WithTranslationPort(translationPort).
		WithCtrlPort(ctrlPort)

	at := builder.Build(name)
	b.l1iAT = at
	b.simulation.RegisterComponent(at)
}

func (b *Builder) buildL1ITLB() {
	builder := tlb.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithNumMSHREntry(4).
		WithNumSets(1).
		WithNumWays(64).
		WithNumReqPerCycle(4).
		WithTranslationProviderMapper(b.l1TLBAddressMapper)

	name := fmt.Sprintf("%s.L1ITLB", b.name)
	topPort := sim.NewPort(nil, 32, 32, name+".Top")
	bottomPort := sim.NewPort(nil, 32, 32, name+".Bottom")
	controlPort := sim.NewPort(nil, 32, 32, name+".Control")
	tlb := builder.
		WithTopPort(topPort).
		WithBottomPort(bottomPort).
		WithControlPort(controlPort).
		Build(name)
	b.l1iTLB = tlb
	b.simulation.RegisterComponent(tlb)
}

func (b *Builder) buildL1ICache() {
	// AT is already built; point the cache mapper at the AT top port.
	b.l1iCacheMapper = &mem.SinglePortMapper{
		Port: b.l1iAT.GetPortByName("Top").AsRemote(),
	}

	builder := writethroughcache.MakeBuilder().
		WithEngine(b.simulation.GetEngine()).
		WithFreq(b.freq).
		WithBankLatency(1).
		WithNumBanks(1).
		WithLog2BlockSize(b.log2CacheLineSize).
		WithWayAssociativity(4).
		WithNumMSHREntry(16).
		WithTotalByteSize(32 * mem.KB).
		WithNumReqsPerCycle(4).
		WithAddressToPortMapper(b.l1iCacheMapper)

	name := fmt.Sprintf("%s.L1ICache", b.name)
	topPort := sim.NewPort(nil, 32, 32, name+".Top")
	bottomPort := sim.NewPort(nil, 32, 32, name+".Bottom")
	controlPort := sim.NewPort(nil, 32, 32, name+".Control")
	cache := builder.
		WithTopPort(topPort).
		WithBottomPort(bottomPort).
		WithControlPort(controlPort).
		Build(name)
	b.l1iCache = cache
	b.simulation.RegisterComponent(cache)
	// if b.memTracer != nil {
	// 	tracing.CollectTrace(cache, b.memTracer)
	// }
}
