package emu

import (
	"math"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

// DispatchPortName is the name of the port that connects the emulation
// ComputeUnit with the dispatcher and the command processor.
const DispatchPortName = "ToDispatcher"

// Spec is the immutable configuration of an emulation ComputeUnit.
type Spec struct {
	// IsCDNA3 is captured from the configured decoder by the builder.
	IsCDNA3 bool        `json:"is_cdna3"`
	Freq    timing.Freq `json:"freq"`
}

// State retains queued work, completion deadlines and completion state.
// Each work-group is emulated atomically within one event. Register and LDS
// scratch values therefore never survive an engine stop between events.
type State struct {
	QueuedWGs          []messaging.Msg
	PendingCompletions []pendingWGCompletion
	// NextEmulationAt is the time at which the queued work-groups will be
	// emulated. Work-groups that arrive before this time are batched into
	// the same emulation pass.
	NextEmulationAt timing.VTimeInPicoSec

	// FinishedMapWGReqIDs accumulates the IDs of the MapWGReqs whose
	// work-groups have completed but have not been acknowledged with a
	// WGCompletionMsg yet.
	FinishedMapWGReqIDs []uint64

	// CompletionDst is the port to send the WGCompletionMsg to.
	CompletionDst messaging.RemotePort
}

// Resources holds the shared references that an emulation ComputeUnit needs.
type Resources struct {
	Decoder         Decoder
	ALU             ALU
	StorageAccessor StorageAccessor
}

// Comp is the emulation ComputeUnit. It omits the pipeline design but can
// still functionally execute GCN3/CDNA3 instructions. It is reactive: it
// wakes up when a protocol.MapWGReq arrives on the "ToDispatcher" port,
// emulates the work-group, and responds with a protocol.WGCompletionMsg.
type Comp = wakeup.Component[Spec, State, Resources, Ports, Middlewares]
type Ports struct {
	ToDispatcher messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.cu"`
}
type Middlewares struct{ Processor *cuProcessor }

func (m *Middlewares) Initialize(c *Comp) {
	m.Processor = &cuProcessor{
		comp:      c,
		wfs:       make(map[*kernels.WorkGroup][]*Wavefront),
		instCache: make(map[uint64]*insts.Inst),
	}
}

// DispatcherView adapts an emulation ComputeUnit to the interface that the
// command processor's resource pool expects from a dispatchable CU. The
// emulation CU has no resource limits.
type DispatcherView struct {
	CU *Comp
}

// DispatchingPort returns the port that the dispatcher can use to dispatch
// work-groups to the CU.
func (v DispatcherView) DispatchingPort() messaging.RemotePort {
	return v.CU.Ports.ToDispatcher.AsRemote()
}

// ControlPort returns the port that can receive controlling messages from
// the Command Processor.
func (v DispatcherView) ControlPort() messaging.RemotePort {
	return v.CU.Ports.ToDispatcher.AsRemote()
}

// WfPoolSizes returns an array of the numbers of wavefronts that each SIMD
// unit can execute.
func (v DispatcherView) WfPoolSizes() []int {
	return []int{math.MaxInt32}
}

// VRegCounts returns an array of the numbers of vector registers in each
// SIMD unit.
func (v DispatcherView) VRegCounts() []int {
	return []int{-1}
}

// SRegCount returns the number of scalar registers in the Compute Unit.
func (v DispatcherView) SRegCount() int {
	return -1
}

// LDSBytes returns the number of bytes in the LDS of the CU.
func (v DispatcherView) LDSBytes() int {
	return -1
}
