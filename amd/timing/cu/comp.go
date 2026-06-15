package cu

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
)

// Port names of the ComputeUnit. The port instances are created externally
// (by the platform configuration or the test setup) and supplied with
// AssignPort after Build.
const (
	// DispatchPortName receives MapWGReqs from the dispatcher and sends
	// WGCompletionMsgs back (v4: ToACE / "Top").
	DispatchPortName = "Top"

	// CtrlPortName receives pipeline flush/restart requests from the command
	// processor (v4: ToCP / "Ctrl").
	CtrlPortName = "Ctrl"

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
	Freq timing.Freq `json:"freq"`

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

// State is the pure, serializable runtime state of a timing ComputeUnit.
//
// The complex runtime structures (wavefront pools, in-flight access records,
// sub-unit pipeline contents) hold pointers and live on the ComputeUnit
// middleware instead. // TODO(akita5): state purity
type State struct {
	// InstMem is the port instruction fetches are sent to.
	InstMem messaging.RemotePort `json:"inst_mem"`

	// ScalarMem is the port scalar memory accesses are sent to.
	ScalarMem messaging.RemotePort `json:"scalar_mem"`

	// Running indicates that at least one work-group has been mapped.
	Running bool `json:"running"`

	IsFlushing                   bool `json:"is_flushing"`
	IsPaused                     bool `json:"is_paused"`
	IsSendingOutShadowBufferReqs bool `json:"is_sending_out_shadow_buffer_reqs"`
	IsHandlingWfCompletionEvent  bool `json:"is_handling_wf_completion_event"`

	// HasFlushReq, FlushReqID, and FlushReqSrc record the pipeline flush
	// request currently being served (v4: currentFlushReq).
	HasFlushReq bool                 `json:"has_flush_req"`
	FlushReqID  uint64               `json:"flush_req_id"`
	FlushReqSrc messaging.RemotePort `json:"flush_req_src"`

	// HasPendingCPRsp, PendingCPRspTo, and PendingCPRspDst record the flush
	// response waiting to be sent to the command processor (v4: toSendToCP).
	HasPendingCPRsp bool                 `json:"has_pending_cp_rsp"`
	PendingCPRspTo  uint64               `json:"pending_cp_rsp_to"`
	PendingCPRspDst messaging.RemotePort `json:"pending_cp_rsp_dst"`
}

// Resources holds the shared references that a timing ComputeUnit needs.
type Resources struct {
	// Decoder decodes raw instruction bytes. Defaults to
	// insts.NewDisassembler() when nil at Build time.
	Decoder emu.Decoder

	// ALU executes the instructions. Defaults to gcn3.NewALU(nil) when nil at
	// Build time.
	ALU emu.ALU

	// VectorMemModules maps addresses to the ports that serve them.
	VectorMemModules mem.AddressToPortMapper
}

// Comp is the timing ComputeUnit component.
type Comp = modeling.Component[Spec, State, Resources]

// MiddlewareOf returns the ComputeUnit middleware attached to a timing
// compute-unit component. It is used by external code (e.g., the ISA
// debugger) that needs access to the register files and other complex
// runtime state.
func MiddlewareOf(comp *Comp) *ComputeUnit {
	for _, mw := range comp.Middlewares() {
		if cuMW, ok := mw.(*ComputeUnit); ok {
			return cuMW
		}
	}

	panic("cu: component does not carry a ComputeUnit middleware")
}

// DispatcherView adapts a timing ComputeUnit to the interface that the
// command processor's resource pool expects from a dispatchable CU.
type DispatcherView struct {
	CU *Comp
}

// DispatchingPort returns the port that the dispatcher can use to dispatch
// work-groups to the CU.
func (v DispatcherView) DispatchingPort() messaging.RemotePort {
	return v.CU.GetPortByName(DispatchPortName).AsRemote()
}

// ControlPort returns the port that can receive controlling messages from
// the Command Processor.
func (v DispatcherView) ControlPort() messaging.RemotePort {
	return v.CU.GetPortByName(CtrlPortName).AsRemote()
}

// WfPoolSizes returns an array of the numbers of wavefronts that each SIMD
// unit can execute.
func (v DispatcherView) WfPoolSizes() []int {
	sizes := make([]int, numWfPools)
	for i := range sizes {
		sizes[i] = v.CU.Spec().WfPoolSize
	}

	return sizes
}

// VRegCounts returns an array of the numbers of vector registers in each
// SIMD unit.
func (v DispatcherView) VRegCounts() []int {
	return v.CU.Spec().VGPRCounts
}

// SRegCount returns the number of scalar registers in the Compute Unit.
func (v DispatcherView) SRegCount() int {
	return v.CU.Spec().SGPRCount
}

// LDSBytes returns the number of bytes in the LDS of the CU.
func (v DispatcherView) LDSBytes() int {
	return v.CU.Spec().LDSBytes
}
