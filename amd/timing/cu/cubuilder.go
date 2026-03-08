package cu

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/pipelining"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
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
	memPipelineBufferSize      int

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
// This allows using different ALU implementations (e.g., GCN3 vs CDNA3).
func (b Builder) WithALUFactory(factory emu.ALUFactory) Builder {
	b.aluFactory = factory
	return b
}

// WithNumSinglePrecisionUnits sets the number of single-precision units per
// SIMD. Default is 16 (GCN3). CDNA3 uses 32.
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

// Build returns a newly constructed compute unit according to the
// configuration.
func (b Builder) Build(name string) *ComputeUnit {
	b.name = name
	cu := NewComputeUnit(name, b.engine)
	cu.Freq = b.freq
	cu.Decoder = insts.NewDisassembler()
	cu.WfDispatcher = NewWfDispatcher(cu)
	cu.InFlightVectorMemAccessLimit = 512

	if b.aluFactory != nil {
		b.alu = b.aluFactory(nil)
	} else {
		b.alu = emu.NewALU(nil)
	}
	for i := 0; i < 4; i++ {
		cu.WfPools = append(cu.WfPools, NewWavefrontPool(b.wfPoolSize))
	}

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
	scheduler := NewScheduler(cu, fetchArbitor, issueArbitor)
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

	ldsUnit := NewLDSUnit(cu, b.alu)
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
	cu.VectorMemUnit = vectorMemoryUnit

	vectorMemoryUnit.postInstructionPipelineBuffer = sim.NewBuffer(
		cu.Name()+".VectorMemoryUnit.PostInstPipelineBuffer", 8)
	vectorMemoryUnit.instructionPipeline = pipelining.NewPipeline(
		cu.Name()+".VectorMemoryUnit.InstPipeline",
		b.vecMemInstPipelineStages, 1,
		vectorMemoryUnit.postInstructionPipelineBuffer)

	pipelineWidth := b.vecMemTransPipelineWidth
	if pipelineWidth < 1 {
		pipelineWidth = 1
	}
	bufSize := b.memPipelineBufferSize
	if bufSize < 8 {
		bufSize = 8
	}
	vectorMemoryUnit.postTransactionPipelineBuffer = sim.NewBuffer(
		cu.Name()+".VectorMemoryUnit.PostTransPipelineBuffer", bufSize)
	vectorMemoryUnit.transactionPipeline = pipelining.NewPipeline(
		cu.Name()+".VectorMemoryUnit.TransactionPipeline",
		b.vecMemTransPipelineStages, pipelineWidth,
		vectorMemoryUnit.postTransactionPipelineBuffer)

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
