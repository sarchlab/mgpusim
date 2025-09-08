package cu

import (
	"fmt"

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
	vgprCount         []int
	sgprCount         int
	log2CachelineSize uint64

	decoder            emu.Decoder
	scratchpadPreparer ScratchpadPreparer
	alu                emu.ALU
	storageAccessor    *emu.StorageAccessor

	visTracer           tracing.Tracer
	enableVisTracing    bool
	enableCPIntegration bool
	instMem             sim.Port
	hasInstMem          bool
}

// MakeBuilder returns a default builder object
func MakeBuilder() Builder {
	var b Builder
	b.freq = 1000 * sim.MHz
	b.simdCount = 4
	b.sgprCount = 3200
	b.vgprCount = []int{16384, 16384, 16384, 16384}
	b.log2CachelineSize = 6

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

// WithInstMem sets the instruction memory port for the compute unit.
func (b Builder) WithInstMem(instMem sim.Port) Builder {
	b.instMem = instMem
	b.hasInstMem = true
	return b
}

// WithStorageAccessor sets a storage accessor for the ALU to enable memory access.
// This is required for simple GPU simulations where the ALU needs to read/write memory.
func (b Builder) WithStorageAccessor(storageAccessor *emu.StorageAccessor) Builder {
	b.storageAccessor = storageAccessor
	return b
}

// WithALU sets a custom ALU implementation.
// This allows using a completely custom ALU instead of the default one.
func (b Builder) WithALU(alu emu.ALU) Builder {
	b.alu = alu
	return b
}

// WithCPIntegration enables CP integration by creating the necessary ports
// and internal components for command processor communication.
func (b Builder) WithCPIntegration() Builder {
	b.enableCPIntegration = true
	return b
}

// Build returns a newly constructed compute unit according to the
// configuration.
func (b *Builder) Build(name string) *ComputeUnit {
	b.name = name
	cu := NewComputeUnit(name, b.engine)
	cu.Freq = b.freq
	cu.Decoder = insts.NewDisassembler()
	cu.WfDispatcher = NewWfDispatcher(cu)
	cu.InFlightVectorMemAccessLimit = 512

	// Configure ALU with storage accessor if provided
	if b.alu == nil {
		if b.storageAccessor != nil {
			b.alu = emu.NewALU(b.storageAccessor)
		} else {
			b.alu = emu.NewALU(nil) // Default behavior for complex simulations
		}
	}
	b.scratchpadPreparer = NewScratchpadPreparerImpl(cu)

	for i := 0; i < 4; i++ {
		cu.WfPools = append(cu.WfPools, NewWavefrontPool(10))
	}

	b.equipScheduler(cu)
	b.equipScalarUnits(cu)
	b.equipSIMDUnits(cu)
	b.equipLDSUnit(cu)
	b.equipVectorMemoryUnit(cu)
	b.equipRegisterFiles(cu)

	// Set the instruction memory port if provided
	if b.hasInstMem {
		cu.InstMem = b.instMem
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
	cu.BranchUnit = NewBranchUnit(cu, b.scratchpadPreparer, b.alu)

	scalarDecoder := NewDecodeUnit(cu)
	cu.ScalarDecoder = scalarDecoder
	scalarUnit := NewScalarUnit(cu, b.scratchpadPreparer, b.alu)
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
		simdUnit := NewSIMDUnit(cu, name, b.scratchpadPreparer, b.alu)
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

	ldsUnit := NewLDSUnit(cu, b.scratchpadPreparer, b.alu)
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
	vectorMemoryUnit := NewVectorMemoryUnit(cu, b.scratchpadPreparer, coalescer)
	cu.VectorMemUnit = vectorMemoryUnit

	vectorMemoryUnit.postInstructionPipelineBuffer = sim.NewBuffer(
		cu.Name()+".VectorMemoryUnit.PostInstPipelineBuffer", 8)
	vectorMemoryUnit.instructionPipeline = pipelining.NewPipeline(
		cu.Name()+".VectorMemoryUnit.InstPipeline",
		6, 1,
		vectorMemoryUnit.postInstructionPipelineBuffer)

	vectorMemoryUnit.postTransactionPipelineBuffer = sim.NewBuffer(
		cu.Name()+".VectorMemoryUnit.PostTransPipelineBuffer", 8)
	vectorMemoryUnit.transactionPipeline = pipelining.NewPipeline(
		cu.Name()+".VectorMemoryUnit.TransactionPipeline",
		60, 1,
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
