package cu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu/gcn3"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// DefaultSpec returns a copy of the default compute-unit configuration.
// Callers obtain it, tweak the fields they care about, and pass it to
// WithSpec.
func DefaultSpec() Spec {
	return Spec{
		InstMem: "", ScalarMem: "", IsCDNA3: false,
		Freq:                         1 * timing.GHz,
		SIMDCount:                    4,
		WfPoolSize:                   10,
		VGPRCounts:                   []int{16384, 16384, 16384, 16384},
		SGPRCount:                    3200,
		LDSBytes:                     64 * 1024,
		Log2CachelineSize:            6,
		NumSinglePrecisionUnits:      16,
		VecMemInstPipelineStages:     6,
		VecMemTransPipelineStages:    10,
		VecMemTransPipelineWidth:     1,
		MemPipelineBufferSize:        8,
		MaxCoalescingPenalty:         0,
		RegisterScoreboard:           false,
		InFlightVectorMemAccessLimit: 512,
		InstBufByteSize:              256,
	}
}

// A Builder can construct a fully functional Compute Unit. Configuration is
// supplied as a whole through WithSpec; shared references (decoder, ALU,
// vector memory address mapper) through WithResources; wiring through
// WithSimulation. The component declares its "Top", "InstMem",
// "ScalarMem", and "VectorMem" ports; the port instances are supplied
// externally after Build with BindPort. The destinations of instruction
// and scalar memory accesses are supplied in Spec before Build.
type Builder struct {
	spec      Spec
	resources Resources
	registrar modeling.Simulation
}

// MakeBuilder returns a builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: DefaultSpec()}
}

// WithSimulation supplies the simulation that owns the component and its clock.
func (b Builder) WithSimulation(reg modeling.Simulation) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources sets the shared references of the compute unit. Decoder and
// ALU default to insts.NewDisassembler() and gcn3.NewALU(nil) when left nil.
func (b Builder) WithResources(resources Resources) Builder {
	b.resources = resources
	return b
}

// Build returns a newly constructed compute unit according to the
// configuration.
func (b Builder) Build(name string) *Comp {
	if b.registrar == nil {
		panic("cu: WithSimulation is required")
	}

	b.mustHaveValidSpec()
	b.fillResourceDefaults()
	if d, ok := b.resources.Decoder.(*insts.Disassembler); ok {
		b.spec.IsCDNA3 = d.IsCDNA3
	}

	comp := modeling.MakeBuilder[*Comp](b.spec).WithSimulation(b.registrar).WithResources(b.resources).Build(name)

	return comp
}

func (b *Builder) mustHaveValidSpec() {
	if len(b.spec.VGPRCounts) != b.spec.SIMDCount {
		panic("cu: VGPRCounts must have a length that equals to the SIMDCount")
	}
}

func (b *Builder) fillResourceDefaults() {
	if b.resources.Decoder == nil {
		b.resources.Decoder = insts.NewDisassembler()
	}

	if b.resources.ALU == nil {
		b.resources.ALU = gcn3.NewALU(nil)
	}
}

func (b *Builder) equipScheduler(cu *ComputeUnit) {
	fetchArbitor := new(FetchArbiter)
	fetchArbitor.InstBufByteSize = b.spec.InstBufByteSize
	issueArbitor := NewIssueArbiter(cu)
	issueArbitor.scoreboardEnabled = b.spec.RegisterScoreboard
	scheduler := NewScheduler(cu, fetchArbitor, issueArbitor)
	scheduler.scoreboardEnabled = b.spec.RegisterScoreboard
	cu.Scheduler = scheduler
}

func (b *Builder) equipScalarUnits(cu *ComputeUnit) {
	cu.BranchUnit = NewBranchUnit(cu, b.resources.ALU)

	scalarDecoder := NewDecodeUnit(cu)
	scalarDecoder.stage = "decode_scalar"
	scalarDecoder.index = 0
	cu.ScalarDecoder = scalarDecoder
	scalarUnit := NewScalarUnit(cu, b.resources.ALU)
	scalarUnit.log2CachelineSize = b.spec.Log2CachelineSize
	cu.ScalarUnit = scalarUnit
	for i := 0; i < b.spec.SIMDCount; i++ {
		scalarDecoder.AddExecutionUnit(scalarUnit)
	}
}

func (b *Builder) equipSIMDUnits(cu *ComputeUnit, name string) {
	vectorDecoder := NewDecodeUnit(cu)
	vectorDecoder.stage = "decode_vector"
	vectorDecoder.index = 1
	cu.VectorDecoder = vectorDecoder
	for i := 0; i < b.spec.SIMDCount; i++ {
		simdName := fmt.Sprintf(name+".SIMD%d", i)
		simdUnit := NewSIMDUnit(cu, simdName, b.resources.ALU)
		simdUnit.index = i
		simdUnit.NumSinglePrecisionUnit = b.spec.NumSinglePrecisionUnits
		simdUnit.scoreboardEnabled = b.spec.RegisterScoreboard
		if b.spec.RegisterScoreboard {
			simdUnit.pipelineCapacity = 1
		}
		vectorDecoder.AddExecutionUnit(simdUnit)
		cu.SIMDUnit = append(cu.SIMDUnit, simdUnit)
	}
}

func (b *Builder) equipLDSUnit(cu *ComputeUnit) {
	ldsDecoder := NewDecodeUnit(cu)
	ldsDecoder.stage = "decode_lds"
	ldsDecoder.index = 2
	cu.LDSDecoder = ldsDecoder

	ldsUnit := NewLDSUnit(cu, b.resources.ALU)
	cu.LDSUnit = ldsUnit

	for i := 0; i < b.spec.SIMDCount; i++ {
		ldsDecoder.AddExecutionUnit(ldsUnit)
	}
}

func (b *Builder) equipVectorMemoryUnit(cu *ComputeUnit, name string) {
	vectorMemDecoder := NewDecodeUnit(cu)
	vectorMemDecoder.stage = "decode_vmem"
	vectorMemDecoder.index = 3
	cu.VectorMemDecoder = vectorMemDecoder

	coalescer := &defaultCoalescer{
		ids:               cu.comp,
		log2CacheLineSize: b.spec.Log2CachelineSize,
	}
	vectorMemoryUnit := NewVectorMemoryUnit(cu, coalescer)
	vectorMemoryUnit.maxCoalescingPenalty = b.spec.MaxCoalescingPenalty
	cu.VectorMemUnit = vectorMemoryUnit

	for i := 0; i < b.spec.SIMDCount; i++ {
		vectorMemDecoder.AddExecutionUnit(vectorMemoryUnit)
	}
}

func (b *Builder) equipRegisterFiles(cu *ComputeUnit) {
	sRegFile := &SimpleRegisterFile{owner: cu.comp, vectorIndex: -1}
	cu.SRegFile = sRegFile

	for i := 0; i < b.spec.SIMDCount; i++ {
		vRegFile := &SimpleRegisterFile{owner: cu.comp, vectorIndex: i, ByteSizePerLane: 1024}
		cu.VRegFile = append(cu.VRegFile, vRegFile)
	}
}

func (cu *ComputeUnit) initializeUnits() {
	b := Builder{spec: cu.comp.Spec, resources: cu.comp.Resources}
	cu.InFlightVectorMemAccessLimit = b.spec.InFlightVectorMemAccessLimit
	dispatcher := NewWfDispatcher(cu)
	dispatcher.scoreboardEnabled = b.spec.RegisterScoreboard
	cu.WfDispatcher = dispatcher
	for i := 0; i < numWfPools; i++ {
		cu.WfPools = append(cu.WfPools, NewWavefrontPool(cu, i, b.spec.WfPoolSize))
	}
	b.equipScheduler(cu)
	b.equipScalarUnits(cu)
	b.equipSIMDUnits(cu, cu.comp.Name())
	b.equipLDSUnit(cu)
	b.equipVectorMemoryUnit(cu, cu.comp.Name())
	b.equipRegisterFiles(cu)
}
