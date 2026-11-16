package sm

import (
	"fmt"

	writethroughcache "github.com/sarchlab/akita/v5/mem/cache/writethrough"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/nvidia/message"
	"github.com/sarchlab/mgpusim/v5/nvidia/smsp"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

const warpSize = 32

// SMController models a streaming multiprocessor. It receives thread blocks
// from the GPU, spreads their warps over its SMSPs, and reports finished
// thread blocks back to the GPU.
type SMController struct {
	*Comp
	SMSPs     []*smsp.SMSPController
	L1VCaches []*writethroughcache.Comp
}

type Spec struct {
	ToGPU                   messaging.RemotePort   `json:"to_gpu"`
	SMSPs                   []messaging.RemotePort `json:"smsps"`
	Freq                    timing.Freq            `json:"freq"`
	SM2SMSPWarpIssueLatency uint64                 `json:"sm2smsp_warp_issue_latency"`
	SMReceiveGPULatency     uint64                 `json:"sm_receive_gpu_latency"`
	SMSPResponseHandleWidth uint64                 `json:"smsp_response_handle_width"`
}

type State struct {
	UndispatchedWarps                    []trace.WarpRef
	WarpsCount                           uint64
	SmspIssueIndex                       int
	ThreadblockWarpCount                 map[string]uint64
	ThreadblockWarpCountOrigin           map[string]uint64
	FinishedThreadblocks                 []trace.Dim3
	SM2SMSPWarpIssueLatencyRemaining     uint64
	SMReceiveGPULatencyRemaining         uint64
	CWDAdmissionPathCostLatencyRemaining uint64
}

type Resources struct{ Traces *trace.Store }
type Ports struct {
	ToGPU   messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/nvidia/message.sm"`
	ToSMSPs messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/nvidia/message.sm"`
}
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Middlewares struct{ Controller *SMController }

func (m *Middlewares) Initialize(c *Comp) {
	m.Controller = &SMController{Comp: c}
}
func (c *SMController) Handle(timing.Event) bool { return c.Tick() }

// Tick advances the SM by one cycle.
func (s *SMController) Tick() bool {
	madeProgress := false
	madeProgress = s.reportFinishedThreadblocks() || madeProgress
	madeProgress = s.dispatchWarpsToSMSPs() || madeProgress
	madeProgress = s.processGPUInput() || madeProgress
	madeProgress = s.processSMSPsInput() || madeProgress

	return madeProgress
}

func (s *SMController) processGPUInput() bool {
	msg, ok := s.Ports.ToGPU.PeekIncoming()
	if !ok {
		return false
	}

	if s.State.SMReceiveGPULatencyRemaining > 0 {
		s.State.SMReceiveGPULatencyRemaining--
		return true
	}

	switch msg := msg.Payload.(type) {
	case message.DeviceToSMMsg:
		s.processThreadblock(msg.Threadblock)
	default:
		panic(fmt.Sprintf("%s: unexpected message %T from GPU", s.Name(), msg))
	}

	s.State.SMReceiveGPULatencyRemaining = s.Spec.SMReceiveGPULatency

	return true
}

func (s *SMController) processSMSPsInput() bool {
	madeProgress := false

	for range max(s.Spec.SMSPResponseHandleWidth, 1) {
		msg, ok := s.Ports.ToSMSPs.PeekIncoming()
		if !ok {
			break
		}

		switch msg := msg.Payload.(type) {
		case message.SMSPToSMMsg:
			s.processWarpFinished(msg)
		default:
			panic(fmt.Sprintf("%s: unexpected message %T from SMSP",
				s.Name(), msg))
		}

		s.Ports.ToSMSPs.RetrieveIncoming()

		madeProgress = true
	}

	return madeProgress
}

// processThreadblock admits a thread block. Admission takes a few cycles
// that grow with the number of SMSPs the thread block occupies.
func (s *SMController) processThreadblock(ref trace.BlockRef) {
	tb := s.Resources.Traces.Block(ref)
	if s.State.CWDAdmissionPathCostLatencyRemaining == 0 {
		nSMSPToUse := max((tb.WarpsCount()+15)/16, 1)
		s.State.CWDAdmissionPathCostLatencyRemaining = 2*nSMSPToUse - 1

		if s.State.CWDAdmissionPathCostLatencyRemaining > 0 {
			return
		}
	}

	if s.State.CWDAdmissionPathCostLatencyRemaining > 1 {
		s.State.CWDAdmissionPathCostLatencyRemaining--
		return
	}

	s.State.CWDAdmissionPathCostLatencyRemaining = 0

	s.State.ThreadblockWarpCount[fmt.Sprint(tb.ID)] = tb.WarpsCount()
	s.State.ThreadblockWarpCountOrigin[fmt.Sprint(tb.ID)] = tb.WarpsCount()
	for i := range tb.Warps {
		s.State.UndispatchedWarps = append(s.State.UndispatchedWarps,
			trace.WarpRef{Kernel: ref.Kernel, Block: ref.Block, Warp: i})
	}
	s.State.WarpsCount += tb.WarpsCount()

	s.Ports.ToGPU.RetrieveIncoming()
}

func (s *SMController) processWarpFinished(msg message.SMSPToSMMsg) {
	if !msg.WarpFinished {
		return
	}

	tbID := s.Resources.Traces.Warp(msg.Warp).FatherThreadblockID
	if s.State.ThreadblockWarpCount[fmt.Sprint(tbID)] == 0 {
		panic(fmt.Sprintf("%s: unexpected finished warp of thread block %v",
			s.Name(), tbID))
	}

	s.State.ThreadblockWarpCount[fmt.Sprint(tbID)]--
	if s.State.ThreadblockWarpCount[fmt.Sprint(tbID)] == 0 {
		delete(s.State.ThreadblockWarpCount, fmt.Sprint(tbID))
		s.State.FinishedThreadblocks = append(s.State.FinishedThreadblocks, tbID)
	}
}

func (s *SMController) reportFinishedThreadblocks() bool {
	if len(s.State.FinishedThreadblocks) == 0 || !s.Ports.ToGPU.CanSend() {
		return false
	}

	tbID := s.State.FinishedThreadblocks[0]
	msg := messaging.Msg{
		ID:  s.NewID(),
		Src: s.Ports.ToGPU.AsRemote(),
		Dst: s.Spec.ToGPU,
		Payload: message.SMToDeviceMsg{
			NumThreadFinished: s.State.ThreadblockWarpCountOrigin[fmt.Sprint(tbID)] * warpSize,
			SMID:              s.Name(),
		},
	}
	s.Ports.ToGPU.Send(msg)

	delete(s.State.ThreadblockWarpCountOrigin, fmt.Sprint(tbID))
	s.State.FinishedThreadblocks = s.State.FinishedThreadblocks[1:]

	return true
}

// dispatchWarpsToSMSPs sends all waiting warps to the SMSPs round-robin, one
// message per SMSP.
func (s *SMController) dispatchWarpsToSMSPs() bool {
	if len(s.Spec.SMSPs) == 0 || len(s.State.UndispatchedWarps) == 0 {
		return false
	}

	if s.State.SM2SMSPWarpIssueLatencyRemaining > 0 {
		s.State.SM2SMSPWarpIssueLatencyRemaining--
		return true
	}

	if s.Ports.ToSMSPs.OutgoingBuf().Size()+len(s.Spec.SMSPs) > portBufSize {
		return false
	}

	warpLists := make([][]trace.WarpRef, len(s.Spec.SMSPs))
	for _, warp := range s.State.UndispatchedWarps {
		warpLists[s.State.SmspIssueIndex] = append(warpLists[s.State.SmspIssueIndex], warp)
		s.State.SmspIssueIndex = (s.State.SmspIssueIndex + 1) % len(s.Spec.SMSPs)
	}

	for i, warps := range warpLists {
		if len(warps) == 0 {
			continue
		}

		dst := s.Spec.SMSPs[i]
		s.Ports.ToSMSPs.Send(
			messaging.Msg{ID: s.NewID(), Src: s.Ports.ToSMSPs.AsRemote(), Dst: dst, Payload: message.SMToSMSPMsg{
				WarpList: warps,
			}},
		)
	}

	s.State.UndispatchedWarps = nil
	s.State.SM2SMSPWarpIssueLatencyRemaining = s.Spec.SM2SMSPWarpIssueLatency

	return true
}

// GetTotalWarpsCount returns the number of warps this SM has received.
func (s *SMController) GetTotalWarpsCount() uint64 {
	return s.State.WarpsCount
}

func (s *State) Initialize(c *Comp) {
	if c.Resources.Traces == nil {
		panic("NVIDIA component requires a shared trace store")
	}
	if c.Spec.ToGPU == "" {
		panic(fmt.Sprintf("SM requires a GPU destination: to_gpu=%q", c.Spec.ToGPU))
	}
	ports := make(map[messaging.RemotePort]bool)
	for _, peer := range c.Spec.SMSPs {
		if peer == "" || ports[peer] {
			panic(fmt.Sprintf("invalid or duplicate SMSP destination: port=%q", peer))
		}
		ports[peer] = true
	}

	s.ThreadblockWarpCount = make(map[string]uint64)
	s.ThreadblockWarpCountOrigin = make(map[string]uint64)
	s.SM2SMSPWarpIssueLatencyRemaining = c.Spec.SM2SMSPWarpIssueLatency
	s.SMReceiveGPULatencyRemaining = c.Spec.SMReceiveGPULatency
}
