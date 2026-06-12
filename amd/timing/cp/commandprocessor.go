package cp

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// Spec contains the immutable configuration of the Command Processor.
type Spec struct {
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
}

// Control sequences that the Command Processor can be running. The Command
// Processor runs at most one control sequence at a time.
const (
	ctrlSeqNone      = ""
	ctrlSeqFlush     = "flush"
	ctrlSeqShootdown = "shootdown"
	ctrlSeqRestart   = "restart"
)

// Driver-response kinds queued in State.PendingDriverRsps.
const (
	driverRspFlushDone       = "flushDone"
	driverRspShootdownDone   = "shootdownDone"
	driverRspRestartDone     = "restartDone"
	driverRspRDMADrainDone   = "rdmaDrainDone"
	driverRspRDMARestartDone = "rdmaRestartDone"
)

// State contains the mutable runtime data of the Command Processor.
type State struct {
	// Destinations of the messages that the Command Processor sends. They are
	// set by the configuration code after Build. The TLB, address translator,
	// ROB, cache, and DRAM destinations are the "Control" ports of the
	// respective components (they speak memcontrolprotocol).
	Driver             messaging.RemotePort   `json:"driver"`
	DMAEngine          messaging.RemotePort   `json:"dma_engine"`
	RDMA               messaging.RemotePort   `json:"rdma"`
	CUs                []messaging.RemotePort `json:"cus"`
	TLBs               []messaging.RemotePort `json:"tlbs"`
	AddressTranslators []messaging.RemotePort `json:"address_translators"`
	ROBs               []messaging.RemotePort `json:"robs"`
	L1VCaches          []messaging.RemotePort `json:"l1v_caches"`
	L1SCaches          []messaging.RemotePort `json:"l1s_caches"`
	L1ICaches          []messaging.RemotePort `json:"l1i_caches"`
	L2Caches           []messaging.RemotePort `json:"l2_caches"`

	// DRAMControllers list the Control ports of the DRAM controllers. The
	// Command Processor currently sends no commands to them (the page
	// migration flow that used them has been dropped); the list is kept so
	// that configuration code can record the topology.
	DRAMControllers []messaging.RemotePort `json:"dram_controllers"`

	// In-flight memory-copy bookkeeping: cloned request ID -> original
	// request from the driver.
	BottomMemCopyH2DToTop map[uint64]protocol.MemCopyH2DReq `json:"bottom_mem_copy_h2d_to_top"`
	BottomMemCopyD2HToTop map[uint64]protocol.MemCopyD2HReq `json:"bottom_mem_copy_d2h_to_top"`

	// Control-sequence bookkeeping. CtrlSeq names the sequence in progress
	// (flush, shootdown, restart), CtrlStep is the index of the current step
	// within the sequence, and PendingAcks counts the responses that must
	// arrive before the sequence advances to the next step.
	CtrlSeq     string `json:"ctrl_seq"`
	CtrlStep    int    `json:"ctrl_step"`
	PendingAcks uint64 `json:"pending_acks"`

	ShootDownInProcess bool `json:"shoot_down_in_process"`

	// Requests from the driver that are being served by a control sequence.
	CurrFlushReq  protocol.FlushReq         `json:"curr_flush_req"`
	CurrShootdown protocol.ShootDownCommand `json:"curr_shootdown"`

	// Outbound message queues, drained by the control middleware as the
	// corresponding ports become available.
	PendingCacheReqs     []memcontrolprotocol.Req        `json:"pending_cache_reqs"`
	PendingTLBReqs       []memcontrolprotocol.Req        `json:"pending_tlb_reqs"`
	PendingATReqs        []memcontrolprotocol.Req        `json:"pending_at_reqs"`
	PendingCUFlushReqs   []protocol.CUPipelineFlushReq   `json:"pending_cu_flush_reqs"`
	PendingCURestartReqs []protocol.CUPipelineRestartReq `json:"pending_cu_restart_reqs"`
	PendingDriverRsps    []string                        `json:"pending_driver_rsps"`
}

// Comp is the Command Processor, an Akita component that is responsible for
// receiving requests from the driver and dispatching them to other parts of
// the GPU.
//
// Ports (declared in Build, assigned externally):
//   - "ToDriver": connects to the driver.
//   - "ToDMA": connects to the DMA engine.
//   - "ToCUs": connects to the Compute Units (dispatching + pipeline control).
//   - "ToTLBs": connects to the Control ports of the TLBs.
//   - "ToAddressTranslators": connects to the Control ports of the address
//     translators and the reorder buffers.
//   - "ToCaches": connects to the Control ports of the L1/L2 caches.
//   - "ToRDMA": connects to the Ctrl port of the RDMA engine.
type Comp = modeling.Component[Spec, State, modeling.None]

// CommandProcessor is an alias of Comp, kept for readability at use sites.
type CommandProcessor = Comp

// CUInterfaceForCP defines the interface that a CP requires from CU.
type CUInterfaceForCP interface {
	resource.DispatchableCU

	// ControlPort returns a port on the CU that the CP can send controlling
	// messages to.
	ControlPort() messaging.RemotePort
}

// RegisterCU allows the Command Processor to control and dispatch to the CU.
func RegisterCU(cp *Comp, cu CUInterfaceForCP) {
	cp.State.CUs = append(cp.State.CUs, cu.ControlPort())

	for _, mw := range cp.Middlewares() {
		if cpMW, ok := mw.(*cpMiddleware); ok {
			for _, d := range cpMW.dispatchers {
				d.RegisterCU(cu)
			}
		}
	}
}
