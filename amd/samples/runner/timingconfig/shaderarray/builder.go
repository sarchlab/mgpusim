// Package shaderarray provides a builder for a shader array.
package shaderarray

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/mem/rob"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cu"
)

// Port buffer sizes. The CU port sizes mirror the v4 CU builder; the other
// sizes mirror the buffer sizes the v4 Akita component builders used when
// they created their ports internally (caches/ATs/TLBs: NumReqPerCycle;
// ROBs: 2*NumReqPerCycle; control ports: 1).
const (
	cuTopBufSize       = 4
	cuCtrlBufSize      = 4
	cuInstMemBufSize   = 4
	cuScalarMemBufSize = 32
	cuVectorMemBufSize = 64

	ctrlPortBufSize = 1
)

// ShaderArray is the externally visible handle of a built shader array. It
// replaces the v4 sim.Domain: the GPU builders reach the components directly
// and fetch the ports they need with GetPortByName.
type ShaderArray struct {
	Name string

	CUs []*cu.Comp

	L1VROBs   []*rob.Comp
	L1VATs    []*addresstranslator.Comp
	L1VCaches []*writethroughcache.Comp
	L1VTLBs   []*tlb.Comp

	L1SROB   *rob.Comp
	L1SAT    *addresstranslator.Comp
	L1SCache *writethroughcache.Comp
	L1STLB   *tlb.Comp

	L1IROB   *rob.Comp
	L1IAT    *addresstranslator.Comp
	L1ICache *writethroughcache.Comp
	L1ITLB   *tlb.Comp
}

// Builder builds a shader array.
type Builder struct {
	simulation *simulation.Simulation

	gpuID                     uint64
	name                      string
	numCUs                    int
	freq                      timing.Freq
	log2CacheLineSize         uint64
	log2PageSize              uint64
	wfPoolSize                int
	vgprCount                 []int
	numSinglePrecisionUnits   int
	vecMemInstPipelineStages  int
	vecMemTransPipelineStages int
	vecMemTransPipelineWidth  int
	cuMemPipelineBufferSize   int
	l1vCacheSize              uint64
	l1vBankLatency            int
	memPipelineBufferSize     int
	maxCoalescingPenalty      int
	registerScoreboard        bool
	l1AddressMapper           mem.AddressToPortMapper
	l1TLBAddressMapper        mem.AddressToPortMapper
	aluBuilder                func() emu.ALU

	sa *ShaderArray

	connectionCount int
}

// MakeBuilder creates a new builder.
func MakeBuilder() Builder {
	return Builder{
		numCUs:            4,
		freq:              1 * timing.GHz,
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
func (b Builder) WithFreq(freq timing.Freq) Builder {
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

// WithL1AddressMapper sets the L1 address mapper to use. The mapper must be
// fully populated before Build is called: the v5 caches snapshot the mapper
// contents at build time.
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

// WithALUBuilder sets the function used to create the ALU of each compute
// unit. This allows using different ALU implementations (e.g., GCN3 vs
// CDNA3). It replaces the v4 WithALUFactory option.
func (b Builder) WithALUBuilder(f func() emu.ALU) Builder {
	b.aluBuilder = f
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

// WithL1VBankLatency sets the L1V cache bank latency in cycles.
func (b Builder) WithL1VBankLatency(latency int) Builder {
	b.l1vBankLatency = latency
	return b
}

// WithMemPipelineBufferSize sets the buffer size for memory pipeline
// connections (CU→ROB→AT→L1V). Larger values allow more concurrent
// memory transactions, improving throughput for bandwidth-limited workloads.
func (b Builder) WithMemPipelineBufferSize(size int) Builder {
	b.memPipelineBufferSize = size
	return b
}

// WithMaxCoalescingPenalty sets the maximum coalescing penalty in cycles
// for poorly-coalesced read transactions in each CU.
func (b Builder) WithMaxCoalescingPenalty(n int) Builder {
	b.maxCoalescingPenalty = n
	return b
}

// WithRegisterScoreboard enables or disables the register scoreboard and
// SIMD pipelining feature in each CU.
func (b Builder) WithRegisterScoreboard(enabled bool) Builder {
	b.registerScoreboard = enabled
	return b
}

// Build builds the shader array.
func (b Builder) Build(name string) *ShaderArray {
	b.name = name
	b.sa = &ShaderArray{Name: name}

	b.buildComponents()
	b.connectComponents()

	return b.sa
}

// buildPort creates a port instance for a declared port and assigns it to the
// component.
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

// buildComponents builds the components in bottom-up dataflow order so that
// every mapper and Spec destination is known at build time (the v5 caches
// and reorder buffers resolve their downstream targets at Build).
func (b *Builder) buildComponents() {
	b.buildL1VCaches()
	b.buildL1VTLBs()
	b.buildL1VAddressTranslators()
	b.buildL1VReorderBuffers()

	b.buildL1SCache()
	b.buildL1STLB()
	b.buildL1SAddressTranslator()
	b.buildL1SReorderBuffer()

	b.buildL1ITLB()
	b.buildL1IAddressTranslator()
	b.buildL1ICache()
	b.buildL1IReorderBuffer()

	b.buildCUs()
}

func (b *Builder) connectComponents() {
	b.connectVectorMem()
	b.connectScalarMem()
	b.connectInstMem()
}

func (b *Builder) connectVectorMem() {
	for i := range b.numCUs {
		cuComp := b.sa.CUs[i]
		robComp := b.sa.L1VROBs[i]
		atComp := b.sa.L1VATs[i]
		tlbComp := b.sa.L1VTLBs[i]
		l1v := b.sa.L1VCaches[i]

		b.connectWithDirectConnection(
			cuComp.GetPortByName(cu.VectorMemPortName),
			robComp.GetPortByName("Top"))

		b.connectWithDirectConnection(
			robComp.GetPortByName("Bottom"), atComp.GetPortByName("Top"))

		b.connectWithDirectConnection(
			atComp.GetPortByName("Translation"), tlbComp.GetPortByName("Top"))

		b.connectWithDirectConnection(
			l1v.GetPortByName("Top"), atComp.GetPortByName("Bottom"))
	}
}

func (b *Builder) connectScalarMem() {
	robComp := b.sa.L1SROB
	atComp := b.sa.L1SAT
	tlbComp := b.sa.L1STLB
	l1s := b.sa.L1SCache

	b.connectWithDirectConnection(
		robComp.GetPortByName("Bottom"), atComp.GetPortByName("Top"))
	b.connectWithDirectConnection(
		atComp.GetPortByName("Translation"), tlbComp.GetPortByName("Top"))
	b.connectWithDirectConnection(
		l1s.GetPortByName("Top"), atComp.GetPortByName("Bottom"))

	conn := directconnection.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(b.name + ".ScalarMemConn")

	robTopPort := robComp.GetPortByName("Top")
	conn.PlugIn(robTopPort)

	for i := range b.numCUs {
		cuComp := b.sa.CUs[i]
		cuComp.State.ScalarMem = robTopPort.AsRemote()
		conn.PlugIn(cuComp.GetPortByName(cu.ScalarMemPortName))
	}
}

func (b *Builder) connectInstMem() {
	robComp := b.sa.L1IROB
	atComp := b.sa.L1IAT
	tlbComp := b.sa.L1ITLB
	l1i := b.sa.L1ICache

	b.connectWithDirectConnection(
		robComp.GetPortByName("Bottom"), l1i.GetPortByName("Top"))
	b.connectWithDirectConnection(
		l1i.GetPortByName("Bottom"), atComp.GetPortByName("Top"))
	b.connectWithDirectConnection(
		atComp.GetPortByName("Translation"), tlbComp.GetPortByName("Top"))

	conn := directconnection.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(b.name + ".InstMemConn")

	robTopPort := robComp.GetPortByName("Top")
	conn.PlugIn(robTopPort)

	for i := range b.numCUs {
		cuComp := b.sa.CUs[i]
		cuComp.State.InstMem = robTopPort.AsRemote()
		conn.PlugIn(cuComp.GetPortByName(cu.InstMemPortName))
	}
}

func (b *Builder) connectWithDirectConnection(port1, port2 messaging.Port) {
	name := fmt.Sprintf("%s.Conn[%d]", b.name, b.connectionCount)
	b.connectionCount++

	conn := directconnection.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(directconnection.Spec{Freq: b.freq}).
		Build(name)

	conn.PlugIn(port1)
	conn.PlugIn(port2)
}

func (b *Builder) cuSpec() cu.Spec {
	spec := cu.DefaultSpec()
	spec.Freq = b.freq
	spec.Log2CachelineSize = b.log2CacheLineSize

	if b.wfPoolSize > 0 {
		spec.WfPoolSize = b.wfPoolSize
	}

	if b.vgprCount != nil {
		spec.VGPRCounts = b.vgprCount
	}

	if b.numSinglePrecisionUnits > 0 {
		spec.NumSinglePrecisionUnits = b.numSinglePrecisionUnits
	}

	if b.vecMemInstPipelineStages > 0 {
		spec.VecMemInstPipelineStages = b.vecMemInstPipelineStages
	}

	if b.vecMemTransPipelineStages > 0 {
		spec.VecMemTransPipelineStages = b.vecMemTransPipelineStages
	}

	if b.vecMemTransPipelineWidth > 0 {
		spec.VecMemTransPipelineWidth = b.vecMemTransPipelineWidth
	}

	if b.cuMemPipelineBufferSize > 0 {
		spec.MemPipelineBufferSize = b.cuMemPipelineBufferSize
	}

	if b.maxCoalescingPenalty > 0 {
		spec.MaxCoalescingPenalty = b.maxCoalescingPenalty
	}

	spec.RegisterScoreboard = b.registerScoreboard

	return spec
}

func (b *Builder) buildCUs() {
	spec := b.cuSpec()

	for i := 0; i < b.numCUs; i++ {
		cuName := fmt.Sprintf("%s.CU[%d]", b.name, i)

		resources := cu.Resources{
			VectorMemModules: &mem.SinglePortMapper{
				Port: b.sa.L1VROBs[i].GetPortByName("Top").AsRemote(),
			},
		}
		if b.aluBuilder != nil {
			resources.ALU = b.aluBuilder()
		}

		computeUnit := cu.MakeBuilder().
			WithRegistrar(b.simulation).
			WithSpec(spec).
			WithResources(resources).
			Build(cuName)

		b.buildPort(computeUnit, cu.DispatchPortName, cuTopBufSize)
		b.buildPort(computeUnit, cu.CtrlPortName, cuCtrlBufSize)
		b.buildPort(computeUnit, cu.InstMemPortName, cuInstMemBufSize)
		b.buildPort(computeUnit, cu.ScalarMemPortName, cuScalarMemBufSize)
		b.buildPort(computeUnit, cu.VectorMemPortName, cuVectorMemBufSize)

		b.sa.CUs = append(b.sa.CUs, computeUnit)
	}
}

func (b *Builder) buildROB(
	name string,
	bufferSize, numReqPerCycle int,
	bottomUnit messaging.RemotePort,
) *rob.Comp {
	spec := rob.DefaultSpec()
	spec.Freq = b.freq
	spec.BufferSize = bufferSize
	spec.NumReqPerCycle = numReqPerCycle
	spec.BottomUnit = bottomUnit

	robComp := rob.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		Build(name)

	// v4 ROB ports were 2*NumReqPerCycle deep; the control port was 1 deep.
	b.buildPort(robComp, "Top", 2*numReqPerCycle)
	b.buildPort(robComp, "Bottom", 2*numReqPerCycle)
	b.buildPort(robComp, "Control", ctrlPortBufSize)

	return robComp
}

func (b *Builder) buildL1VReorderBuffers() {
	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VROB[%d]", b.name, i)
		robComp := b.buildROB(name, 512, 32,
			b.sa.L1VATs[i].GetPortByName("Top").AsRemote())
		b.sa.L1VROBs = append(b.sa.L1VROBs, robComp)
	}
}

func (b *Builder) buildAT(
	name string,
	numReqPerCycle int,
	memMapper, transMapper mem.AddressToPortMapper,
) *addresstranslator.Comp {
	spec := addresstranslator.DefaultSpec()
	spec.Freq = b.freq
	spec.DeviceID = b.gpuID
	spec.Log2PageSize = b.log2PageSize
	spec.NumReqPerCycle = numReqPerCycle

	at := addresstranslator.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		WithResources(addresstranslator.Resources{
			MemProviderMapper:         memMapper,
			TranslationProviderMapper: transMapper,
		}).
		Build(name)

	// v4 AT data ports were NumReqPerCycle deep; the control port was 1 deep.
	b.buildPort(at, "Top", numReqPerCycle)
	b.buildPort(at, "Bottom", numReqPerCycle)
	b.buildPort(at, "Translation", numReqPerCycle)
	b.buildPort(at, "Control", ctrlPortBufSize)

	return at
}

func (b *Builder) buildL1VAddressTranslators() {
	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VAddrTrans[%d]", b.name, i)
		at := b.buildAT(name, 32,
			&mem.SinglePortMapper{
				Port: b.sa.L1VCaches[i].GetPortByName("Top").AsRemote(),
			},
			&mem.SinglePortMapper{
				Port: b.sa.L1VTLBs[i].GetPortByName("Top").AsRemote(),
			})
		b.sa.L1VATs = append(b.sa.L1VATs, at)
	}
}

func (b *Builder) buildTLB(
	name string,
	numSets, numWays, numMSHREntry, numReqPerCycle, latency int,
) *tlb.Comp {
	spec := tlb.DefaultSpec()
	spec.Freq = b.freq
	spec.NumSets = numSets
	spec.NumWays = numWays
	spec.MSHRSize = numMSHREntry
	spec.NumReqPerCycle = numReqPerCycle
	spec.Latency = latency
	spec.Log2PageSize = b.log2PageSize

	tlbComp := tlb.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		WithResources(tlb.Resources{
			TranslationProviderMapper: b.l1TLBAddressMapper,
		}).
		Build(name)

	// v4 TLB data ports were NumReqPerCycle deep; the control port was 1
	// deep.
	b.buildPort(tlbComp, "Top", numReqPerCycle)
	b.buildPort(tlbComp, "Bottom", numReqPerCycle)
	b.buildPort(tlbComp, "Control", ctrlPortBufSize)

	return tlbComp
}

func (b *Builder) buildL1VTLBs() {
	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VTLB[%d]", b.name, i)
		// v4 used a 1-cycle TLB. The v5 TLB inserts requests into its
		// lookup pipeline with a 1-cycle dwell at stage 0; with a
		// single-stage pipeline (Latency=1) the dwell counter of items
		// already at the last stage is never decremented (akita
		// v5.0.0-beta.2 queueing.Pipeline), so the request deadlocks.
		// Latency=2 is the minimum functional value.
		tlbComp := b.buildTLB(name, 4, 64, 64, 32, 2)
		b.sa.L1VTLBs = append(b.sa.L1VTLBs, tlbComp)
	}
}

func (b *Builder) buildL1Cache(
	name string,
	spec writethroughcache.Spec,
	mapper mem.AddressToPortMapper,
) *writethroughcache.Comp {
	cache := writethroughcache.MakeBuilder().
		WithRegistrar(b.simulation).
		WithSpec(spec).
		WithResources(writethroughcache.Resources{
			AddressMapper: mapper,
		}).
		Build(name)

	// v4 writethrough/writearound cache ports were NumReqPerCycle deep.
	b.buildPort(cache, "Top", spec.NumReqPerCycle)
	b.buildPort(cache, "Bottom", spec.NumReqPerCycle)
	b.buildPort(cache, "Control", spec.NumReqPerCycle)

	return cache
}

func (b *Builder) buildL1VCaches() {
	l1vSize := 16 * mem.KB
	if b.l1vCacheSize > 0 {
		l1vSize = b.l1vCacheSize
	}

	l1vBankLatency := 20
	if b.l1vBankLatency > 0 {
		l1vBankLatency = b.l1vBankLatency
	}

	spec := writethroughcache.DefaultSpec()
	spec.Freq = b.freq
	spec.WritePolicyType = "write-around" // v4: writearound package
	spec.BankLatency = l1vBankLatency
	spec.NumBanks = 4
	spec.Log2BlockSize = b.log2CacheLineSize
	spec.WayAssociativity = 4
	spec.NumMSHREntry = 128
	spec.NumReqPerCycle = 8
	spec.MaxNumConcurrentTrans = 128
	spec.TotalByteSize = l1vSize
	spec.DirLatency = 2 // v4 writearound default

	for i := 0; i < b.numCUs; i++ {
		name := fmt.Sprintf("%s.L1VCache[%d]", b.name, i)
		cache := b.buildL1Cache(name, spec, b.l1AddressMapper)
		b.sa.L1VCaches = append(b.sa.L1VCaches, cache)
	}
}

func (b *Builder) buildL1SReorderBuffer() {
	name := fmt.Sprintf("%s.L1SROB", b.name)
	b.sa.L1SROB = b.buildROB(name, 512, 32,
		b.sa.L1SAT.GetPortByName("Top").AsRemote())
}

func (b *Builder) buildL1SAddressTranslator() {
	name := fmt.Sprintf("%s.L1SAddrTrans", b.name)
	b.sa.L1SAT = b.buildAT(name, 32,
		&mem.SinglePortMapper{
			Port: b.sa.L1SCache.GetPortByName("Top").AsRemote(),
		},
		&mem.SinglePortMapper{
			Port: b.sa.L1STLB.GetPortByName("Top").AsRemote(),
		})
}

func (b *Builder) buildL1STLB() {
	name := fmt.Sprintf("%s.L1STLB", b.name)
	b.sa.L1STLB = b.buildTLB(name, 1, 64, 64, 32, 4)
}

func (b *Builder) buildL1SCache() {
	spec := writethroughcache.DefaultSpec()
	spec.Freq = b.freq
	spec.WritePolicyType = "write-through" // v4: writethrough package
	spec.BankLatency = 1
	spec.NumBanks = 1
	spec.Log2BlockSize = b.log2CacheLineSize
	spec.WayAssociativity = 4
	spec.NumMSHREntry = 128
	spec.NumReqPerCycle = 32
	spec.TotalByteSize = 16 * mem.KB
	spec.MaxNumConcurrentTrans = 16 // v4 writethrough default
	// v4 writethrough had a 0-cycle directory stage. The v5 directory
	// pipeline needs at least 1 stage (a 0-stage pipeline never releases
	// items), so this adds one cycle of directory latency vs v4.
	spec.DirLatency = 1

	name := fmt.Sprintf("%s.L1SCache", b.name)
	b.sa.L1SCache = b.buildL1Cache(name, spec, b.l1AddressMapper)
}

func (b *Builder) buildL1IReorderBuffer() {
	name := fmt.Sprintf("%s.L1IROB", b.name)
	b.sa.L1IROB = b.buildROB(name, 128, 4,
		b.sa.L1ICache.GetPortByName("Top").AsRemote())
}

func (b *Builder) buildL1IAddressTranslator() {
	name := fmt.Sprintf("%s.L1IAddrTrans", b.name)
	b.sa.L1IAT = b.buildAT(name, 16,
		b.l1AddressMapper,
		&mem.SinglePortMapper{
			Port: b.sa.L1ITLB.GetPortByName("Top").AsRemote(),
		})
}

func (b *Builder) buildL1ITLB() {
	name := fmt.Sprintf("%s.L1ITLB", b.name)
	b.sa.L1ITLB = b.buildTLB(name, 1, 64, 4, 4, 4)
}

func (b *Builder) buildL1ICache() {
	spec := writethroughcache.DefaultSpec()
	spec.Freq = b.freq
	spec.WritePolicyType = "write-through" // v4: writethrough package
	spec.BankLatency = 1
	spec.NumBanks = 1
	spec.Log2BlockSize = b.log2CacheLineSize
	spec.WayAssociativity = 4
	spec.NumMSHREntry = 16
	spec.NumReqPerCycle = 4
	spec.TotalByteSize = 32 * mem.KB
	spec.MaxNumConcurrentTrans = 16 // v4 writethrough default
	// v4 writethrough had a 0-cycle directory stage. The v5 directory
	// pipeline needs at least 1 stage (a 0-stage pipeline never releases
	// items), so this adds one cycle of directory latency vs v4.
	spec.DirLatency = 1

	name := fmt.Sprintf("%s.L1ICache", b.name)
	b.sa.L1ICache = b.buildL1Cache(name, spec,
		&mem.SinglePortMapper{
			Port: b.sa.L1IAT.GetPortByName("Top").AsRemote(),
		})
}
