package cu

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu/gcn3"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

var defaultSpec = Spec{
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

// DefaultSpec returns a copy of the default compute-unit configuration.
// Callers obtain it, tweak the fields they care about, and pass it to
// WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// A Builder can construct a fully functional Compute Unit. Configuration is
// supplied as a whole through WithSpec; shared references (decoder, ALU,
// vector memory address mapper) through WithResources; wiring through
// WithRegistrar. The component declares its "Top", "Ctrl", "InstMem",
// "ScalarMem", and "VectorMem" ports; the port instances are supplied
// externally after Build with AssignPort. The destinations of instruction
// and scalar memory accesses are set after Build through comp.State.InstMem
// and comp.State.ScalarMem.
type Builder struct {
	spec      Spec
	resources Resources
	registrar modeling.Registrar
}

// MakeBuilder returns a builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation
// in platform assembly, or modeling.NewStandaloneRegistrar(engine) in
// isolated tests).
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
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
		panic("cu: WithRegistrar is required")
	}

	b.mustHaveValidSpec()
	b.fillResourceDefaults()

	comp := modeling.NewBuilder[Spec, State, Resources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		WithResources(b.resources).
		Build(name)
	comp.State = State{}

	cuMW := &ComputeUnit{
		comp:                  comp,
		engine:                b.registrar.GetEngine(),
		wfCompletionHandlerID: name + ".WfCompletion",
		wfDispatchHandlerID:   name + ".WfDispatch",
		Decoder:               b.resources.Decoder,
		wftime:                make(map[uint64]timing.VTimeInPicoSec),
	}
	cuMW.InFlightVectorMemAccessLimit = b.spec.InFlightVectorMemAccessLimit

	wfDispatcher := NewWfDispatcher(cuMW)
	wfDispatcher.scoreboardEnabled = b.spec.RegisterScoreboard
	cuMW.WfDispatcher = wfDispatcher

	for i := 0; i < numWfPools; i++ {
		cuMW.WfPools = append(cuMW.WfPools, NewWavefrontPool(b.spec.WfPoolSize))
	}

	b.equipScheduler(cuMW)
	b.equipScalarUnits(cuMW)
	b.equipSIMDUnits(cuMW, name)
	b.equipLDSUnit(cuMW)
	b.equipVectorMemoryUnit(cuMW, name)
	b.equipRegisterFiles(cuMW)

	comp.AddMiddleware(cuMW)

	comp.DeclarePort(DispatchPortName)
	comp.DeclarePort(CtrlPortName)
	comp.DeclarePort(InstMemPortName, memprotocol.Requester)
	comp.DeclarePort(ScalarMemPortName, memprotocol.Requester)
	comp.DeclarePort(VectorMemPortName, memprotocol.Requester)

	if hr, ok := b.registrar.GetEngine().(timing.HandlerRegistrar); ok {
		hr.RegisterHandler(cuMW.wfCompletionHandlerID, cuMW)
		hr.RegisterHandler(cuMW.wfDispatchHandlerID, cuMW)
	}

	b.registrar.RegisterComponent(comp)

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
	issueArbitor := new(IssueArbiter)
	issueArbitor.scoreboardEnabled = b.spec.RegisterScoreboard
	scheduler := NewScheduler(cu, fetchArbitor, issueArbitor)
	scheduler.scoreboardEnabled = b.spec.RegisterScoreboard
	cu.Scheduler = scheduler
}

func (b *Builder) equipScalarUnits(cu *ComputeUnit) {
	cu.BranchUnit = NewBranchUnit(cu, b.resources.ALU)

	scalarDecoder := NewDecodeUnit(cu)
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
	cu.VectorDecoder = vectorDecoder
	for i := 0; i < b.spec.SIMDCount; i++ {
		simdName := fmt.Sprintf(name+".SIMD%d", i)
		simdUnit := NewSIMDUnit(cu, simdName, b.resources.ALU)
		simdUnit.NumSinglePrecisionUnit = b.spec.NumSinglePrecisionUnits
		simdUnit.scoreboardEnabled = b.spec.RegisterScoreboard
		if b.spec.RegisterScoreboard {
			simdUnit.pipelineCapacity = 1
			simdUnit.pipelineSlots = make([]*simdPipelineSlot, 0, 1)
		}
		vectorDecoder.AddExecutionUnit(simdUnit)
		cu.SIMDUnit = append(cu.SIMDUnit, simdUnit)
	}
}

func (b *Builder) equipLDSUnit(cu *ComputeUnit) {
	ldsDecoder := NewDecodeUnit(cu)
	cu.LDSDecoder = ldsDecoder

	ldsUnit := NewLDSUnit(cu, b.resources.ALU)
	cu.LDSUnit = ldsUnit

	for i := 0; i < b.spec.SIMDCount; i++ {
		ldsDecoder.AddExecutionUnit(ldsUnit)
	}
}

func (b *Builder) equipVectorMemoryUnit(cu *ComputeUnit, name string) {
	vectorMemDecoder := NewDecodeUnit(cu)
	cu.VectorMemDecoder = vectorMemDecoder

	coalescer := &defaultCoalescer{
		log2CacheLineSize: b.spec.Log2CachelineSize,
	}
	vectorMemoryUnit := NewVectorMemoryUnit(cu, coalescer)
	vectorMemoryUnit.maxCoalescingPenalty = b.spec.MaxCoalescingPenalty
	cu.VectorMemUnit = vectorMemoryUnit

	vectorMemoryUnit.postInstructionPipelineBuffer =
		queueing.NewBuffer[vectorMemInst](
			name+".VectorMemoryUnit.PostInstPipelineBuffer",
			4*b.spec.SIMDCount)
	// v4 used CyclePerStage=1, so the v5 stage count equals the v4 stage
	// count and the total latency is preserved.
	vectorMemoryUnit.instructionPipeline = queueing.NewPipeline[vectorMemInst](
		b.spec.SIMDCount, b.spec.VecMemInstPipelineStages)

	pipelineWidth := b.spec.VecMemTransPipelineWidth
	if pipelineWidth < 1 {
		pipelineWidth = 1
	}
	bufSize := b.spec.MemPipelineBufferSize
	if bufSize < 8 {
		bufSize = 8
	}
	vectorMemoryUnit.postTransactionPipelineBuffer =
		queueing.NewBuffer[VectorMemAccessInfo](
			name+".VectorMemoryUnit.PostTransPipelineBuffer", bufSize)
	vectorMemoryUnit.transactionPipeline =
		queueing.NewPipeline[VectorMemAccessInfo](
			pipelineWidth, b.spec.VecMemTransPipelineStages)

	for i := 0; i < b.spec.SIMDCount; i++ {
		vectorMemDecoder.AddExecutionUnit(vectorMemoryUnit)
	}
}

func (b *Builder) equipRegisterFiles(cu *ComputeUnit) {
	sRegFile := NewSimpleRegisterFile(uint64(b.spec.SGPRCount*4), 0)
	cu.SRegFile = sRegFile

	for i := 0; i < b.spec.SIMDCount; i++ {
		vRegFile := NewSimpleRegisterFile(uint64(b.spec.VGPRCounts[i]*4), 1024)
		cu.VRegFile = append(cu.VRegFile, vRegFile)
	}
}
