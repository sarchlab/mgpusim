package sm

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/cache/writethroughcache"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/mgpusim/v5/nvidia/message"
	"github.com/sarchlab/mgpusim/v5/nvidia/smsp"
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

const warpSize = 32

// SMController models a streaming multiprocessor. It receives thread blocks
// from the GPU, spreads their warps over its SMSPs, and reports finished
// thread blocks back to the GPU.
type SMController struct {
	*modeling.TickingComponent

	ID string

	toGPU       messaging.Port
	toGPURemote messaging.RemotePort
	toSMSPs     messaging.Port

	SMSPs     []*smsp.SMSPController
	L1VCaches []*writethroughcache.Comp

	undispatchedWarps []*trace.WarpTrace
	warpsCount        uint64
	smspIssueIndex    int

	// Unfinished and total warp counts of the thread blocks on this SM.
	threadblockWarpCount       map[trace.Dim3]uint64
	threadblockWarpCountOrigin map[trace.Dim3]uint64
	finishedThreadblocks       []trace.Dim3

	SM2SMSPWarpIssueLatency          uint64
	SM2SMSPWarpIssueLatencyRemaining uint64

	SMReceiveGPULatency          uint64
	SMReceiveGPULatencyRemaining uint64

	SMSPResponseHandleWidth              uint64
	CWDAdmissionPathCostLatencyRemaining uint64
}

// SetGPURemotePort sets the GPU port that finished thread blocks are
// reported to.
func (s *SMController) SetGPURemotePort(remote messaging.RemotePort) {
	s.toGPURemote = remote
}

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
	msg := s.toGPU.PeekIncoming()
	if msg == nil {
		return false
	}

	if s.SMReceiveGPULatencyRemaining > 0 {
		s.SMReceiveGPULatencyRemaining--
		return true
	}

	switch msg := msg.(type) {
	case message.DeviceToSMMsg:
		s.processThreadblock(msg.Threadblock)
	default:
		panic(fmt.Sprintf("%s: unexpected message %T from GPU", s.Name(), msg))
	}

	s.SMReceiveGPULatencyRemaining = s.SMReceiveGPULatency

	return true
}

func (s *SMController) processSMSPsInput() bool {
	madeProgress := false

	for range max(s.SMSPResponseHandleWidth, 1) {
		msg := s.toSMSPs.PeekIncoming()
		if msg == nil {
			break
		}

		switch msg := msg.(type) {
		case message.SMSPToSMMsg:
			s.processWarpFinished(msg)
		default:
			panic(fmt.Sprintf("%s: unexpected message %T from SMSP",
				s.Name(), msg))
		}

		s.toSMSPs.RetrieveIncoming()

		madeProgress = true
	}

	return madeProgress
}

// processThreadblock admits a thread block. Admission takes a few cycles
// that grow with the number of SMSPs the thread block occupies.
func (s *SMController) processThreadblock(tb *trace.ThreadblockTrace) {
	if s.CWDAdmissionPathCostLatencyRemaining == 0 {
		nSMSPToUse := (tb.WarpsCount() + 15) / 16
		s.CWDAdmissionPathCostLatencyRemaining = 2*nSMSPToUse - 1

		if s.CWDAdmissionPathCostLatencyRemaining > 0 {
			return
		}
	}

	if s.CWDAdmissionPathCostLatencyRemaining > 1 {
		s.CWDAdmissionPathCostLatencyRemaining--
		return
	}

	s.CWDAdmissionPathCostLatencyRemaining = 0

	s.threadblockWarpCount[tb.ID] = tb.WarpsCount()
	s.threadblockWarpCountOrigin[tb.ID] = tb.WarpsCount()
	s.undispatchedWarps = append(s.undispatchedWarps, tb.Warps...)
	s.warpsCount += tb.WarpsCount()

	s.toGPU.RetrieveIncoming()
}

func (s *SMController) processWarpFinished(msg message.SMSPToSMMsg) {
	if !msg.WarpFinished {
		return
	}

	tbID := msg.Warp.FatherThreadblockID
	if s.threadblockWarpCount[tbID] == 0 {
		panic(fmt.Sprintf("%s: unexpected finished warp of thread block %v",
			s.Name(), tbID))
	}

	s.threadblockWarpCount[tbID]--
	if s.threadblockWarpCount[tbID] == 0 {
		delete(s.threadblockWarpCount, tbID)
		s.finishedThreadblocks = append(s.finishedThreadblocks, tbID)
	}
}

func (s *SMController) reportFinishedThreadblocks() bool {
	if len(s.finishedThreadblocks) == 0 || !s.toGPU.CanSend() {
		return false
	}

	tbID := s.finishedThreadblocks[0]
	msg := message.SMToDeviceMsg{
		MsgMeta:           message.NewMeta(s.toGPU.AsRemote(), s.toGPURemote),
		NumThreadFinished: s.threadblockWarpCountOrigin[tbID] * warpSize,
		SMID:              s.ID,
	}
	s.toGPU.Send(msg)

	delete(s.threadblockWarpCountOrigin, tbID)
	s.finishedThreadblocks = s.finishedThreadblocks[1:]

	return true
}

// dispatchWarpsToSMSPs sends all waiting warps to the SMSPs round-robin, one
// message per SMSP.
func (s *SMController) dispatchWarpsToSMSPs() bool {
	if len(s.SMSPs) == 0 || len(s.undispatchedWarps) == 0 {
		return false
	}

	if s.SM2SMSPWarpIssueLatencyRemaining > 0 {
		s.SM2SMSPWarpIssueLatencyRemaining--
		return true
	}

	if s.toSMSPs.NumOutgoing()+len(s.SMSPs) > portBufSize {
		return false
	}

	warpLists := make([][]*trace.WarpTrace, len(s.SMSPs))
	for _, warp := range s.undispatchedWarps {
		warpLists[s.smspIssueIndex] = append(warpLists[s.smspIssueIndex], warp)
		s.smspIssueIndex = (s.smspIssueIndex + 1) % len(s.SMSPs)
	}

	for i, warps := range warpLists {
		if len(warps) == 0 {
			continue
		}

		dst := s.SMSPs[i].GetPortByName("ToSM").AsRemote()
		s.toSMSPs.Send(message.SMToSMSPMsg{
			MsgMeta:  message.NewMeta(s.toSMSPs.AsRemote(), dst),
			WarpList: warps,
		})
	}

	s.undispatchedWarps = nil
	s.SM2SMSPWarpIssueLatencyRemaining = s.SM2SMSPWarpIssueLatency

	return true
}

// GetTotalWarpsCount returns the number of warps this SM has received.
func (s *SMController) GetTotalWarpsCount() uint64 {
	return s.warpsCount
}
