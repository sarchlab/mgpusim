package cu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/mgpusim/v4/amd/emu"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// A Builder can construct a fully functional Compute Unit.
type Builder struct {
	engine            sim.Engine
	freq              sim.Freq
	name              string
	simdCount         int
	wfPoolSize        int
	vgprCount         []int
	sgprCount         int
	log2CachelineSize uint64

	numSinglePrecisionUnits    int
	vecMemInstPipelineStages   int
	vecMemTransPipelineStages  int
	vecMemTransPipelineWidth   int
	memPipelineBufferSize          int
	inFlightVectorMemAccessLimit   int

	maxCoalescingPenalty int
	scratchLatency       int
	ldsNumBanks          int
	registerScoreboard   bool
	isCDNA3              bool

	decoder    emu.Decoder
	alu        emu.ALU
	aluFactory emu.ALUFactory

	visTracer        tracing.Tracer
	enableVisTracing bool

	instMem          sim.Port
	scalarMem        sim.Port
	vectorMemModules mem.AddressToPortMapper
}

// MakeBuilder returns a default builder object
func MakeBuilder() Builder {
	var b Builder
	b.freq = 1000 * sim.MHz
	b.simdCount = 4
	b.wfPoolSize = 10
	b.sgprCount = 3200
	b.vgprCount = []int{16384, 16384, 16384, 16384}
	b.log2CachelineSize = 6
	b.numSinglePrecisionUnits = 16
	b.vecMemInstPipelineStages = 6
	b.vecMemTransPipelineStages = 10
	b.vecMemTransPipelineWidth = 1
	b.memPipelineBufferSize = 8

	return b
}

// WithEngine sets the engine to use.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency.
func (b Builder) WithFreq(f sim.Freq) Builder {
	b.freq = f
	return b
}

// WithSIMDCount sets the number of SIMD unit in the ComputeUnit.
func (b Builder) WithSIMDCount(n int) Builder {
	b.simdCount = n
	return b
}

// WithWfPoolSize sets the number of wavefronts in each wavefront pool.
func (b Builder) WithWfPoolSize(n int) Builder {
	b.wfPoolSize = n
	return b
}

// WithVGPRCount sets the number of VGPRs associated with each SIMD Unit.
func (b Builder) WithVGPRCount(counts []int) Builder {
	if len(counts) != b.simdCount {
		panic("counts must have a length that equals to the SIMD count")
	}

	b.vgprCount = counts
	return b
}

// WithSGPRCount equals the number of SGPRs in the Compute Unit.
func (b Builder) WithSGPRCount(count int) Builder {
	b.sgprCount = count
	return b
}

// WithLog2CachelineSize sets the cacheline size as a power of 2.
func (b Builder) WithLog2CachelineSize(n uint64) Builder {
	b.log2CachelineSize = n
	return b
}

// WithVisTracer adds a tracer to the builder.
func (b Builder) WithVisTracer(t tracing.Tracer) Builder {
	b.enableVisTracing = true
	b.visTracer = t
	return b
}

func (b Builder) WithInstMem(p sim.Port) Builder {
	b.instMem = p
	return b
}

func (b Builder) WithScalarMem(p sim.Port) Builder {
	b.scalarMem = p
	return b
}

func (b Builder) WithVectorMemModules(m mem.AddressToPortMapper) Builder {
	b.vectorMemModules = m
	return b
}

// WithALUFactory sets the ALU factory function to use for creating the ALU.
// This allows using different ALU implementations for different architectures.
func (b Builder) WithALUFactory(factory emu.ALUFactory) Builder {
	b.aluFactory = factory
	return b
}

// WithNumSinglePrecisionUnits sets the number of single-precision units per
// SIMD. Default is 16.
func (b Builder) WithNumSinglePrecisionUnits(n int) Builder {
	b.numSinglePrecisionUnits = n
	return b
}

// WithVecMemInstPipelineStages sets the number of stages in the vector memory
// instruction pipeline. Default is 6.
func (b Builder) WithVecMemInstPipelineStages(n int) Builder {
	b.vecMemInstPipelineStages = n
	return b
}

// WithVecMemTransPipelineStages sets the number of stages in the vector memory
// transaction pipeline. Default is 10.
func (b Builder) WithVecMemTransPipelineStages(n int) Builder {
	b.vecMemTransPipelineStages = n
	return b
}

// WithVecMemTransPipelineWidth sets the width (items per cycle) of the vector
// memory transaction pipeline. Default is 1.
func (b Builder) WithVecMemTransPipelineWidth(n int) Builder {
	b.vecMemTransPipelineWidth = n
	return b
}

// WithMemPipelineBufferSize sets the post-pipeline buffer size for vector
// memory transactions. Default is 8.
func (b Builder) WithMemPipelineBufferSize(n int) Builder {
	b.memPipelineBufferSize = n
	return b
}

// WithLDSNumBanks sets the number of LDS banks for bank conflict modeling.
// Default is 32. When set to 0, defaults to 32.
func (b Builder) WithLDSNumBanks(n int) Builder {
	b.ldsNumBanks = n
	return b
}

// WithMaxCoalescingPenalty sets the maximum coalescing penalty in cycles
// for poorly-coalesced read transactions. Default is 0 (disabled).
func (b Builder) WithMaxCoalescingPenalty(n int) Builder {
	b.maxCoalescingPenalty = n
	return b
}

// WithScratchLatency sets the latency in cycles for SCRATCH segment
// (FlatSeg=1) memory operations. On CDNA3, scratch accesses go through
// the memory hierarchy and incur 20-40 cycles. Default is 0 (instant).
func (b Builder) WithScratchLatency(n int) Builder {
	b.scratchLatency = n
	return b
}

// WithInFlightVectorMemAccessLimit sets the maximum number of in-flight
// vector memory access transactions per CU. Default is 512.
func (b Builder) WithInFlightVectorMemAccessLimit(limit int) Builder {
	b.inFlightVectorMemAccessLimit = limit
	return b
}

// WithRegisterScoreboard enables or disables the register scoreboard and
// SIMD pipelining feature. When enabled, the CU tracks per-wavefront
// register availability to detect RAW hazards and allows multiple
// wavefronts to be in-flight in SIMD units simultaneously.
func (b Builder) WithRegisterScoreboard(enabled bool) Builder {
	b.registerScoreboard = enabled
	return b
}

// WithIsCDNA3 sets whether the compute unit should use CDNA3 ISA decoding.
// When enabled, the disassembler uses CDNA3-specific instruction formats.
func (b Builder) WithIsCDNA3(cdna3 bool) Builder {
	b.isCDNA3 = cdna3
	return b
}

// Build returns a newly constructed compute unit according to the
// configuration.
func (b Builder) Build(name string) *ComputeUnit {
	b.name = name
	cu := NewComputeUnit(name, b.engine)
	cu.Freq = b.freq
	decoder := insts.NewDisassembler()
	decoder.IsCDNA3 = b.isCDNA3
	cu.Decoder = decoder
	wfDispatcher := NewWfDispatcher(cu)
	wfDispatcher.scoreboardEnabled = b.registerScoreboard
	cu.WfDispatcher = wfDispatcher
	if b.inFlightVectorMemAccessLimit > 0 {
		cu.InFlightVectorMemAccessLimit = b.inFlightVectorMemAccessLimit
	} else {
		cu.InFlightVectorMemAccessLimit = 512
	}

	if b.aluFactory != nil {
		b.alu = b.aluFactory(nil)
	} else {
		b.alu = emu.NewALU(nil)
	}
	for i := 0; i < 4; i++ {
		cu.WfPools = append(cu.WfPools, NewWavefrontPool(b.wfPoolSize))
	}

	cu.scratchALU = b.alu

	b.equipScheduler(cu)
	b.equipScalarUnits(cu)
	b.equipSIMDUnits(cu)
	b.equipLDSUnit(cu)
	b.equipVectorMemoryUnit(cu)
	b.equipRegisterFiles(cu)

	if b.instMem != nil {
		cu.InstMem = b.instMem
	}

	if b.scalarMem != nil {
		cu.ScalarMem = b.scalarMem
	}

	if b.vectorMemModules != nil {
		cu.VectorMemModules = b.vectorMemModules
	}

	return cu
}

func (b *Builder) equipScheduler(cu *ComputeUnit) {
	fetchArbitor := new(FetchArbiter)
	fetchArbitor.InstBufByteSize = 256
	issueArbitor := new(IssueArbiter)
	issueArbitor.scoreboardEnabled = b.registerScoreboard
	scheduler := NewScheduler(cu, fetchArbitor, issueArbitor)
	scheduler.scoreboardEnabled = b.registerScoreboard
	scheduler.isCDNA3 = b.isCDNA3
	cu.Scheduler = scheduler
}

func (b *Builder) equipScalarUnits(cu *ComputeUnit) {
	cu.BranchUnit = NewBranchUnit(cu, b.alu)

	scalarDecoder := NewDecodeUnit(cu)
	cu.ScalarDecoder = scalarDecoder
	scalarUnit := NewScalarUnit(cu, b.alu)
	scalarUnit.log2CachelineSize = b.log2CachelineSize
	cu.ScalarUnit = scalarUnit
	for i := 0; i < b.simdCount; i++ {
		scalarDecoder.AddExecutionUnit(scalarUnit)
	}
}

func (b *Builder) equipSIMDUnits(cu *ComputeUnit) {
	vectorDecoder := NewDecodeUnit(cu)
	cu.VectorDecoder = vectorDecoder
	for i := 0; i < b.simdCount; i++ {
		name := fmt.Sprintf(b.name+".SIMD%d", i)
		simdUnit := NewSIMDUnit(cu, name, b.alu)
		simdUnit.NumSinglePrecisionUnit = b.numSinglePrecisionUnits
		simdUnit.scoreboardEnabled = b.registerScoreboard
		simdUnit.isCDNA3 = b.isCDNA3
		if b.registerScoreboard {
			simdUnit.pipelineCapacity = 4
			simdUnit.pipelineSlots = make([]*simdPipelineSlot, 0, 4)
		}
		if b.enableVisTracing {
			tracing.CollectTrace(simdUnit, b.visTracer)
		}
		vectorDecoder.AddExecutionUnit(simdUnit)
		cu.SIMDUnit = append(cu.SIMDUnit, simdUnit)
	}
}

func (b *Builder) equipLDSUnit(cu *ComputeUnit) {
	ldsDecoder := NewDecodeUnit(cu)
	cu.LDSDecoder = ldsDecoder

	numBanks := b.ldsNumBanks
	if numBanks == 0 {
		numBanks = 32
	}
	ldsUnit := NewLDSUnit(cu, b.alu, numBanks)
	cu.LDSUnit = ldsUnit

	for i := 0; i < b.simdCount; i++ {
		ldsDecoder.AddExecutionUnit(ldsUnit)
	}
}

func (b *Builder) equipVectorMemoryUnit(cu *ComputeUnit) {
	vectorMemDecoder := NewDecodeUnit(cu)
	cu.VectorMemDecoder = vectorMemDecoder

	coalescer := &defaultCoalescer{
		log2CacheLineSize: b.log2CachelineSize,
	}
	vectorMemoryUnit := NewVectorMemoryUnit(cu, coalescer)
	vectorMemoryUnit.maxCoalescingPenalty = b.maxCoalescingPenalty
	vectorMemoryUnit.scratchLatency = b.scratchLatency
	cu.VectorMemUnit = vectorMemoryUnit

	vectorMemoryUnit.postInstructionPipelineBuffer = queueing.Buffer[vectorMemInst]{
		BufferName: cu.Name() + ".VectorMemoryUnit.PostInstPipelineBuffer",
		Cap:        8 * b.simdCount,
	}
	vectorMemoryUnit.instructionPipeline = queueing.Pipeline[vectorMemInst]{
		Width:     b.simdCount * 2,
		NumStages: b.vecMemInstPipelineStages,
	}

	pipelineWidth := b.vecMemTransPipelineWidth
	if pipelineWidth < 1 {
		pipelineWidth = 1
	}
	bufSize := b.memPipelineBufferSize
	if bufSize < 8 {
		bufSize = 8
	}
	vectorMemoryUnit.postTransactionPipelineBuffer = queueing.Buffer[VectorMemAccessInfo]{
		BufferName: cu.Name() + ".VectorMemoryUnit.PostTransPipelineBuffer",
		Cap:        bufSize,
	}
	vectorMemoryUnit.transactionPipeline = queueing.Pipeline[VectorMemAccessInfo]{
		Width:     pipelineWidth,
		NumStages: b.vecMemTransPipelineStages,
	}

	for i := 0; i < b.simdCount; i++ {
		vectorMemDecoder.AddExecutionUnit(vectorMemoryUnit)
	}
}

func (b *Builder) equipRegisterFiles(cu *ComputeUnit) {
	sRegFile := NewSimpleRegisterFile(uint64(b.sgprCount*4), 0)
	cu.SRegFile = sRegFile

	for i := 0; i < b.simdCount; i++ {
		vRegFile := NewSimpleRegisterFile(uint64(b.vgprCount[i]*4), 1024)
		cu.VRegFile = append(cu.VRegFile, vRegFile)
	}
}
