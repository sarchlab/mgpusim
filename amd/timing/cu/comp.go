package cu

import (
	"fmt"

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

	// InstPrefetchPortName is the one slot of the optional InstPrefetch port
	// group, bound only when Spec.InstPrefetchLines is positive. It sends the
	// reads of lines prefetched into the instruction cache. Their responses
	// are dropped, so they need not return in order with the instruction
	// fetches.
	InstPrefetchPortName = "InstPrefetch[0]"

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
	// InstPrefetchMem is the destination of instruction prefetches (see
	// InstPrefetchLines), usually the instruction cache itself.
	InstPrefetchMem messaging.RemotePort `json:"inst_prefetch_mem"`

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

	// LDSReturnCycles is the fixed gfx942 completion latency beyond SIMD
	// port transfer and serial bank replays. GCN3 uses its legacy pipeline.
	LDSReturnCycles int `json:"lds_return_cycles"`

	// Log2CachelineSize is the cacheline size as a power of 2. It is also the
	// line size of the instruction cache: with block fetch, a block lies in
	// one line, and a prefetch reads one whole line.
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

	// IssueRotation makes the scheduler serve one SIMD's wavefronts per
	// cycle, in rotation over the compute unit's four wavefront pools, so a
	// wavefront issues at most one instruction every four cycles. It requires
	// SIMDCount to be 4.
	IssueRotation bool `json:"issue_rotation"`

	// VALUIssueOverlap lets a wavefront issue its next instruction during the
	// last execute cycle of a VALU instruction. It applies only with
	// RegisterScoreboard.
	VALUIssueOverlap bool `json:"valu_issue_overlap"`

	// UniformVALUCycles gives every VALU instruction the same execution time.
	// Otherwise instructions whose name contains "f64" take twice as long.
	UniformVALUCycles bool `json:"uniform_valu_cycles"`

	// SCCToBranchCycles is the number of cycles from issuing a scalar ALU
	// instruction, which writes SCC, to the earliest issue of a conditional
	// branch on SCC (s_cbranch_scc0 and s_cbranch_scc1). Zero adds no wait.
	// The issue arbiter holds the branch back, with or without
	// RegisterScoreboard, and the compute unit wakes when the wait is over.
	SCCToBranchCycles int `json:"scc_to_branch_cycles"`

	// InFlightVectorMemAccessLimit caps the number of outstanding vector
	// memory transactions.
	InFlightVectorMemAccessLimit int `json:"in_flight_vector_mem_access_limit"`

	// InstBufByteSize is the per-wavefront instruction buffer size. Without
	// block fetch the fetch arbiter fills the buffer up to it, one
	// InstFetchByteSize line at a time. With block fetch it must be a whole
	// number of blocks, at least two, because an instruction can straddle two
	// of them.
	InstBufByteSize int `json:"inst_buf_byte_size"`

	// InstBlockByteSize, when positive, selects block fetch: a wavefront's
	// instruction buffer holds InstBufByteSize/InstBlockByteSize blocks of
	// this size, and a wavefront requests the next block from the instruction
	// memory as soon as one is free. A block is free once the wavefront has
	// issued the instruction that holds its last byte. A taken branch empties
	// the buffer and starts a new fetch stream at its target; a branch that
	// is not taken keeps the buffer. It must be a multiple of 4 that divides
	// the cache line size (Log2CachelineSize). With block fetch, the demand
	// replies from InstMem must arrive in the order of their requests: the
	// compute unit panics on a block that arrives out of order within a fetch
	// stream. The stock assembly sends demand fetches through the L1I reorder
	// buffer; a custom assembly needs a reorder buffer or another responder
	// that answers in order. Replies from InstPrefetchMem may arrive in any
	// order. Zero keeps fetching whole InstFetchByteSize lines, one at a time,
	// and every branch empties the buffer.
	InstBlockByteSize int `json:"inst_block_byte_size"`

	// InstBlockFetchCycles is, with block fetch, the least number of cycles
	// between two block requests of one fetch stream of a wavefront. A taken
	// branch starts a new stream, so the target block is requested at once,
	// however recently the fall-through block was.
	InstBlockFetchCycles int `json:"inst_block_fetch_cycles"`

	// InstPrefetchLines is, with block fetch, the number of cache lines
	// (Log2CachelineSize) past the next block a wavefront requests that the
	// compute unit reads ahead into the instruction cache for it. The
	// responses only warm the cache; they never enter the instruction buffer,
	// and no wavefront waits for them. The compute unit skips a prefetch when
	// a demand fetch or a prefetch of the same process and cache line is
	// already in flight; demand block fetches are always sent. Only wavefronts
	// whose code object records exactly where their code ends (a V5 code
	// object with a kernel symbol) prefetch, and never past that end.
	// Prefetches go to InstPrefetchMem through the InstPrefetch port, which is
	// bound (InstPrefetchPortName) exactly when this is positive. Zero
	// prefetches nothing.
	InstPrefetchLines int `json:"inst_prefetch_lines"`
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
	// BranchTaken records whether the branch in the write stage changed the PC.
	BranchTaken bool
	BankedLDS   BankedLDSState
	ScalarUnit  ScalarUnitState
	VectorUnit  VectorUnitState
	SIMDs       []SIMDState

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
	Top     messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.cu"`
	InstMem messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
	// InstPrefetch holds the instruction-prefetch requester port when
	// Spec.InstPrefetchLines is positive, and nothing otherwise.
	InstPrefetch []messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
	ScalarMem    messaging.Port   `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
	VectorMem    messaging.Port   `akita:"role=github.com/sarchlab/akita/v5/mem/memprotocol.requester"`
}
type Middlewares struct{ CU *cuMiddleware }
type cuMiddleware struct{ cu *ComputeUnit }

func (m *cuMiddleware) Handle(timing.Event) bool { return m.cu.Tick() }
func (m *Middlewares) Initialize(c *Comp) {
	if err := c.Spec.VectorMemModules.Validate(); err != nil {
		panic("cu: VectorMemModules: " + err.Error())
	}
	wantPrefetchPorts := 0
	if c.Spec.InstPrefetchLines > 0 {
		wantPrefetchPorts = 1
	}
	if len(c.Ports.InstPrefetch) != wantPrefetchPorts {
		panic(fmt.Sprintf("cu: %s has %d InstPrefetch ports; it needs one exactly when InstPrefetchLines is positive",
			c.Name(), len(c.Ports.InstPrefetch)))
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
