package cu

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mgpusim/emu"
	"gitlab.com/akita/util/tracing"
)

// A Builder can construct a fully functional ComputeUnit to the outside world.
// It simplify the compute unit building process.
type Builder struct {
	Engine    akita.Engine
	Freq      akita.Freq
	CUName    string
	SIMDCount int
	VGPRCount []int
	SGPRCount int

	Decoder            emu.Decoder
	ScratchpadPreparer ScratchpadPreparer
	ALU                emu.ALU

	InstMem          akita.Port
	ScalarMem        akita.Port
	VectorMemModules cache.LowModuleFinder

	ConnToInstMem   akita.Connection
	ConnToScalarMem akita.Connection
	ConnToVectorMem akita.Connection

	visTracer         tracing.Tracer
	enableVisTracing  bool
	log2CachelineSize uint64
}

// MakeBuilder returns a default builder object
func MakeBuilder() Builder {
	var b Builder
	b.Freq = 1000 * akita.MHz
	b.SIMDCount = 4
	b.SGPRCount = 3200
	b.VGPRCount = []int{16384, 16384, 16384, 16384}
	b.log2CachelineSize = 6

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

// Build returns a newly constructed compute unit according to the
// configuration
func (b *Builder) Build() *ComputeUnit {
	cu := NewComputeUnit(b.CUName, b.Engine)
	cu.Freq = b.Freq
	cu.Decoder = b.Decoder
	cu.WfDispatcher = NewWfDispatcher(cu)

	b.ALU = emu.NewALU(nil)
	b.ScratchpadPreparer = NewScratchpadPreparerImpl(cu)

	for i := 0; i < 4; i++ {
		cu.WfPools = append(cu.WfPools, NewWavefrontPool(10))
	}

	b.equipScheduler(cu)
	b.equipScalarUnits(cu)
	b.equipSIMDUnits(cu)
	b.equipLDSUnit(cu)
	b.equipVectorMemoryUnit(cu)
	b.equipRegisterFiles(cu)

	b.connectToMem(cu)

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
	cu.BranchUnit = NewBranchUnit(cu, b.ScratchpadPreparer, b.ALU)

	scalarDecoder := NewDecodeUnit(cu)
	cu.ScalarDecoder = scalarDecoder
	scalarUnit := NewScalarUnit(cu, b.ScratchpadPreparer, b.ALU)
	scalarUnit.log2CachelineSize = b.log2CachelineSize
	cu.ScalarUnit = scalarUnit
	for i := 0; i < b.SIMDCount; i++ {
		scalarDecoder.AddExecutionUnit(scalarUnit)
	}
}

func (b *Builder) equipSIMDUnits(cu *ComputeUnit) {
	vectorDecoder := NewDecodeUnit(cu)
	cu.VectorDecoder = vectorDecoder
	for i := 0; i < b.SIMDCount; i++ {
		name := fmt.Sprintf(b.CUName+".SIMD%d", i)
		simdUnit := NewSIMDUnit(cu, name, b.ScratchpadPreparer, b.ALU)
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

	ldsUnit := NewLDSUnit(cu, b.ScratchpadPreparer, b.ALU)
	cu.LDSUnit = ldsUnit

	for i := 0; i < b.SIMDCount; i++ {
		ldsDecoder.AddExecutionUnit(ldsUnit)
	}
}

func (b *Builder) equipVectorMemoryUnit(cu *ComputeUnit) {
	vectorMemDecoder := NewDecodeUnit(cu)
	cu.VectorMemDecoder = vectorMemDecoder

	coalescer := &defaultCoalescer{
		log2CacheLineSize: 6,
	}
	vectorMemoryUnit := NewVectorMemoryUnit(cu, b.ScratchpadPreparer, coalescer)
	cu.VectorMemUnit = vectorMemoryUnit

	for i := 0; i < b.SIMDCount; i++ {
		vectorMemDecoder.AddExecutionUnit(vectorMemoryUnit)
	}
}

func (b *Builder) equipRegisterFiles(cu *ComputeUnit) {
	sRegFile := NewSimpleRegisterFile(uint64(b.SGPRCount*4), 0)
	cu.SRegFile = sRegFile

	for i := 0; i < b.SIMDCount; i++ {
		vRegFile := NewSimpleRegisterFile(uint64(b.VGPRCount[i]*4), 1024)
		cu.VRegFile = append(cu.VRegFile, vRegFile)
	}
}

func (b *Builder) connectToMem(cu *ComputeUnit) {
	cu.InstMem = b.InstMem
	cu.ScalarMem = b.ScalarMem
	cu.VectorMemModules = b.VectorMemModules
	b.ConnToInstMem.PlugIn(cu.ToInstMem, 4)
	b.ConnToScalarMem.PlugIn(cu.ToScalarMem, 4)
	b.ConnToVectorMem.PlugIn(cu.ToVectorMem, 4)
}
