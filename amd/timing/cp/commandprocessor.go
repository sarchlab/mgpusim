package cp

import (
	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/dispatching"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// Spec contains the immutable configuration of the Command Processor.
type Spec struct {
	Driver    messaging.RemotePort   `json:"driver"`
	DMAEngine messaging.RemotePort   `json:"dma_engine"`
	RDMA      messaging.RemotePort   `json:"rdma"`
	L1VCaches []messaging.RemotePort `json:"l1v_caches"`
	L1SCaches []messaging.RemotePort `json:"l1s_caches"`
	L1ICaches []messaging.RemotePort `json:"l1i_caches"`
	L2Caches  []messaging.RemotePort `json:"l2_caches"`

	CUs []resource.CUConfig `json:"cus"`

	Freq           timing.Freq `json:"freq"`
	NumDispatchers int         `json:"num_dispatchers"`

	// ConstantKernelLaunchOverhead is the fixed per-kernel launch latency, in
	// cycles, applied to the first kernel launched by a dispatcher.
	ConstantKernelLaunchOverhead int `json:"constant_kernel_launch_overhead"`

	// ConstantKernelOverhead is the fixed overhead, in cycles, after all the
	// work-groups of a kernel complete. When 0, the dispatcher default is
	// used.
	ConstantKernelOverhead int `json:"constant_kernel_overhead"`

	// SubsequentKernelLaunchOverhead is the launch latency, in cycles, for
	// kernels launched after the first one.
	SubsequentKernelLaunchOverhead int `json:"subsequent_kernel_launch_overhead"`

	// WGScalingThreshold is the threshold for WG-count-based scaling of the
	// subsequent kernel launch overhead.
	WGScalingThreshold int `json:"wg_scaling_threshold"`

	// Alg selects the work-group dispatching algorithm: "round-robin",
	// "greedy", "partition", or "per-die". Empty defaults to "round-robin".
	Alg string `json:"alg"`

	// NumDies is the number of dies (XCDs) the "per-die" algorithm dispatches
	// across in parallel.
	NumDies int `json:"num_dies"`

	// WavefrontDispatchCycles is the per-die dispatch cost charged per wavefront,
	// in cycles, for the "per-die" algorithm. A W-wavefront work-group occupies
	// its die's dispatch pipe for W*cycles before the die dispatches the next.
	WavefrontDispatchCycles int `json:"wavefront_dispatch_cycles"`
}

// Control sequences that the Command Processor can be running. The Command
// Processor runs at most one control sequence at a time.
const (
	ctrlSeqNone        = ""
	ctrlSeqFlush       = "flush"
	ctrlSeqKernelStart = "kernelStart"
)

// Driver-response kinds queued in State.PendingDriverRsps.
const (
	driverRspFlushDone       = "flushDone"
	driverRspRDMADrainDone   = "rdmaDrainDone"
	driverRspRDMARestartDone = "rdmaRestartDone"
)

// State contains the mutable runtime data of the Command Processor.
type State struct {
	CopyRecvTasks map[uint64]uint64
	CopySendTasks map[uint64]uint64
	FlushTaskID   uint64
	Dispatchers   []dispatching.State
	CUResources   []resource.CUState

	// In-flight memory-copy bookkeeping: cloned request ID -> original
	// request from the driver.
	BottomMemCopyH2DToTop map[uint64]messaging.Msg `json:"bottom_mem_copy_h2d_to_top"`
	BottomMemCopyD2HToTop map[uint64]messaging.Msg `json:"bottom_mem_copy_d2h_to_top"`

	// Control-sequence bookkeeping. CtrlSeq names the sequence in progress
	// (flush or kernelStart), CtrlStep is the index of the
	// current step within the sequence, and PendingAcks counts the responses
	// that must arrive before the sequence advances to the next step.
	CtrlSeq     string `json:"ctrl_seq"`
	CtrlStep    int    `json:"ctrl_step"`
	PendingAcks uint64 `json:"pending_acks"`

	// Requests from the driver that are being served by a control sequence.
	CurrFlushReq messaging.Msg `json:"curr_flush_req"`

	// KernelStartReqID is the ID of the LaunchKernelReq whose kernel-start
	// L1 invalidation has been started. The request stays in the incoming
	// buffer until the sequence finishes; it is then dispatched and the ID
	// is cleared.
	KernelStartReqID uint64 `json:"kernel_start_req_id"`

	// Outbound message queues, drained by the control middleware as the
	// corresponding ports become available.
	PendingCacheReqs  []messaging.Msg `json:"pending_cache_reqs"`
	PendingDriverRsps []string        `json:"pending_driver_rsps"`
}

// Comp is the Command Processor, an Akita component that is responsible for
// receiving requests from the driver and dispatching them to other parts of
// the GPU.
//
// Ports (declared in Build, assigned externally):
//   - "ToDriver": connects to the driver.
//   - "ToDMA": connects to the DMA engine.
//   - "ToCUs": connects to the Compute Units (dispatching).
//   - "ToCaches": connects to the Control ports of the L1/L2 caches.
//   - "ToRDMA": connects to the Ctrl port of the RDMA engine.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]

type Resources struct {
	Monitor *monitoring.Monitor
	Sampler *sampling.SampledEngine
}
type Ports struct {
	ToDriver messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.cp"`
	ToDMA    messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.cp"`
	ToCUs    messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.cp"`
	ToCaches messaging.Port `akita:"role=github.com/sarchlab/akita/v5/mem/memcontrolprotocol.requester"`
	ToRDMA   messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/timing/rdma.requester"`
}
type Middlewares struct {
	Data    *cpMiddleware
	Control *ctrlMiddleware
}

func (m *Middlewares) Initialize(c *Comp) {
	m.Data = &cpMiddleware{comp: c, dispatchers: buildDispatchers(c)}
	m.Control = &ctrlMiddleware{comp: c}
}

// dispatcherPorts binds the dispatcher's two connections after native port setup.
type dispatcherPorts struct{ cp *Comp }

func (p dispatcherPorts) GetPortByName(name string) messaging.Port {
	switch name {
	case "ToDriver":
		return p.cp.Ports.ToDriver
	case "ToCUs":
		return p.cp.Ports.ToCUs
	default:
		panic("unknown dispatcher port " + name)
	}
}

// CUInterfaceForCP defines the interface that a CP requires from CU.
type CUInterfaceForCP interface {
	resource.DispatchableCU
}

// CUConfig captures immutable dispatch wiring and capacities for Spec.
type CUConfig = resource.CUConfig

// DescribeCU copies a CU's dispatch wiring and capacities before CP Build.
func DescribeCU(c CUInterfaceForCP) CUConfig { return resource.DescribeCU(c) }

func (s *State) Initialize(c *Comp) {
	if c.Spec.NumDispatchers < 1 {
		panic("cp: at least one dispatcher is required")
	}
	if c.Spec.Alg == "per-die" && (c.Spec.NumDies < 1 ||
		len(c.Spec.CUs) < c.Spec.NumDies || len(c.Spec.CUs)%c.Spec.NumDies != 0) {
		panic("cp: per-die dispatch requires equal nonempty CU partitions")
	}

	s.BottomMemCopyH2DToTop = make(map[uint64]messaging.Msg)
	s.BottomMemCopyD2HToTop = make(map[uint64]messaging.Msg)
	s.CopyRecvTasks = make(map[uint64]uint64)
	s.CopySendTasks = make(map[uint64]uint64)
	s.CUResources = resource.InitialState(c.Spec.CUs)
	for i := 0; i < c.Spec.NumDispatchers; i++ {
		s.Dispatchers = append(s.Dispatchers, dispatching.NewState(c.Spec.Alg, c.Spec.NumDies))
	}
}
