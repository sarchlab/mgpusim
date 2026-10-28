package cu

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// Port names of the ComputeUnit. The port instances are created externally
// (by the platform configuration or the test setup) and supplied with
// BindPort after Build.
const (
	// DispatchPortName receives MapWGReqs from the dispatcher and sends
	// WGCompletionMsgs back (v4: ToACE / "Top").
	DispatchPortName = "Top"

	// InstMemPortName sends instruction-fetch reads (v4: ToInstMem).
	InstMemPortName = "InstMem"

	// ScalarMemPortName sends scalar memory reads (v4: ToScalarMem).
	ScalarMemPortName = "ScalarMem"

	// VectorMemPortName sends vector memory reads/writes (v4: ToVectorMem).
	VectorMemPortName = "VectorMem"
)

// numWfPools is the number of wavefront pools in a compute unit. The v4
// implementation hard-coded 4 pools; this is preserved.
const numWfPools = 4

// Spec is the immutable configuration of a timing ComputeUnit.
type Spec struct {
	// VectorMemModules selects the destination for vector memory requests.
	VectorMemModules mem.AddressMapper `json:"vector_mem_modules"`
	// InstMem and ScalarMem are fixed request destinations, supplied before Build.
	InstMem   messaging.RemotePort `json:"inst_mem"`
	ScalarMem messaging.RemotePort `json:"scalar_mem"`

	// IsCDNA3 is captured from the configured decoder by the builder.
	IsCDNA3 bool        `json:"is_cdna3"`
	Freq    timing.Freq `json:"freq"`

	// SIMDCount is the number of SIMD units in the compute unit.
	SIMDCount int `json:"simd_count"`

	// WfPoolSize is the number of wavefronts each wavefront pool can hold.
	WfPoolSize int `json:"wf_pool_size"`

	// VGPRCounts is the number of vector registers per SIMD unit. Its length
	// must equal SIMDCount.
	VGPRCounts []int `json:"vgpr_counts"`

	// SGPRCount is the number of scalar registers in the compute unit.
	SGPRCount int `json:"sgpr_count"`

	// LDSBytes is the size of the local data share in bytes.
	LDSBytes int `json:"lds_bytes"`

	// Log2CachelineSize is the cacheline size as a power of 2.
	Log2CachelineSize uint64 `json:"log2_cacheline_size"`

	// NumSinglePrecisionUnits is the number of single-precision units per
	// SIMD. GCN3 uses 16, CDNA3 uses 32.
	NumSinglePrecisionUnits int `json:"num_single_precision_units"`

	// VecMemInstPipelineStages is the number of stages in the vector memory
	// instruction pipeline (v4 used CyclePerStage=1, so the total latency in
	// cycles equals the stage count).
	VecMemInstPipelineStages int `json:"vec_mem_inst_pipeline_stages"`

	// VecMemTransPipelineStages is the number of stages in the vector memory
	// transaction pipeline.
	VecMemTransPipelineStages int `json:"vec_mem_trans_pipeline_stages"`

	// VecMemTransPipelineWidth is the number of transactions that can enter
	// the transaction pipeline per cycle.
	VecMemTransPipelineWidth int `json:"vec_mem_trans_pipeline_width"`

	// MemPipelineBufferSize is the capacity of the post-pipeline buffer for
	// vector memory transactions.
	MemPipelineBufferSize int `json:"mem_pipeline_buffer_size"`

	// MaxCoalescingPenalty is the maximum coalescing penalty in cycles for
	// poorly-coalesced read transactions. 0 disables the penalty.
	MaxCoalescingPenalty int `json:"max_coalescing_penalty"`

	// RegisterScoreboard enables the register scoreboard and SIMD pipelining
	// feature.
	RegisterScoreboard bool `json:"register_scoreboard"`

	// InFlightVectorMemAccessLimit caps the number of outstanding vector
	// memory transactions.
	InFlightVectorMemAccessLimit int `json:"in_flight_vector_mem_access_limit"`

	// InstBufByteSize is the per-wavefront instruction buffer size that the
	// fetch arbiter fills up to.
	InstBufByteSize int `json:"inst_buf_byte_size"`
}

// SampledCompletion is a pending wave completion, ordered by At in State.
type SampledCompletion struct {
	At   timing.VTimeInPicoSec
	Wave wavefront.Handle
}

// State is the sole owner of mutable compute-unit execution data.
type State struct {
	SampledCompletions                        []SampledCompletion
	InstTasks, WFTasks, WGTasks, RequestTasks map[uint64]uint64
	Groups                                    []WorkGroupState
	Waves                                     []WaveState
	Instructions                              map[uint64]InstructionState
	Pools                                     [][]waveHandle
	InstFetch                                 []FetchState
	Scalar                                    []ScalarAccessState
	Vector                                    []VectorAccessState
	WFTimes                                   map[uint64]timing.VTimeInPicoSec
	SRegisters                                []byte
	VRegisters                                [][]byte
	Scheduler                                 SchedulerState
	Decoders                                  []DecoderState
	Branch, LDS                               StageState
	ScalarUnit                                ScalarUnitState
	VectorUnit                                VectorUnitState
	SIMDs                                     []SIMDState

	// Running indicates that at least one work-group has been mapped.
	Running bool `json:"running"`
}

// Resources holds the shared references that a timing ComputeUnit needs.
type Resources struct {
	Sampler *sampling.SampledEngine
	// Decoder decodes raw instruction bytes. Defaults to
	// insts.NewDisassembler() when nil at Build time.
	Decoder emu.Decoder

	// ALU executes the instructions. Defaults to gcn3.NewALU(nil) when nil at
	// Build time.
	ALU emu.ALU
}

// Comp is the timing ComputeUnit component.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Ports struct {
	Top       messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.cu"`
	InstMem   messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
	ScalarMem messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
	VectorMem messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
}
type Middlewares struct{ CU *cuMiddleware }
type cuMiddleware struct{ cu *ComputeUnit }

func (m *cuMiddleware) Handle(timing.Event) bool { return m.cu.Tick() }
func (m *Middlewares) Initialize(c *Comp) {
	if err := c.Spec.VectorMemModules.Validate(); err != nil {
		panic("cu: VectorMemModules: " + err.Error())
	}

	cu := &ComputeUnit{comp: c, Decoder: c.Resources.Decoder}
	m.CU = &cuMiddleware{cu: cu}
	cu.initializeUnits()
}

// MiddlewareOf returns the ComputeUnit middleware attached to a timing
// compute-unit component. It is used by external code (e.g., the ISA
// debugger) that needs access to the register files and other complex
// runtime state.
func MiddlewareOf(comp *Comp) *ComputeUnit {
	if comp.Middlewares.CU == nil {
		panic("cu: middleware is not constructed")
	}
	cu := comp.Middlewares.CU.cu
	return cu
}

// DispatcherView adapts a timing ComputeUnit to the interface that the
// command processor's resource pool expects from a dispatchable CU.
type DispatcherView struct {
	CU *Comp
}

// DispatchingPort returns the port that the dispatcher can use to dispatch
// work-groups to the CU.
func (v DispatcherView) DispatchingPort() messaging.RemotePort {
	return v.CU.Ports.Top.AsRemote()
}

// WfPoolSizes returns an array of the numbers of wavefronts that each SIMD
// unit can execute.
func (v DispatcherView) WfPoolSizes() []int {
	sizes := make([]int, numWfPools)
	for i := range sizes {
		sizes[i] = v.CU.Spec.WfPoolSize
	}

	return sizes
}

// VRegCounts returns an array of the numbers of vector registers in each
// SIMD unit.
func (v DispatcherView) VRegCounts() []int {
	return v.CU.Spec.VGPRCounts
}

// SRegCount returns the number of scalar registers in the Compute Unit.
func (v DispatcherView) SRegCount() int {
	return v.CU.Spec.SGPRCount
}

// LDSBytes returns the number of bytes in the LDS of the CU.
func (v DispatcherView) LDSBytes() int {
	return v.CU.Spec.LDSBytes
}
