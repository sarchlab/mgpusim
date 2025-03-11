package sm

import (
	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/message"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/nvidia"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/subcore"
)

type SM struct {
	*sim.TickingComponent

	ID         string
	warpsCount int64
	instsCount int64

	// meta
	toGPU       sim.Port
	toGPURemote sim.Port

	toSubcores   sim.Port
	Subcores     map[string]*subcore.Subcore
	freeSubcores []*subcore.Subcore

	undispatchedWarps    []*nvidia.Warp
	unfinishedWarpsCount int64

	finishedThreadblocksCount int64
}

func (s *SM) SetGPURemotePort(remote sim.Port) {
	s.toGPURemote = remote
}

// v3
// func (s *SM) Tick(now sim.VTimeInSec) bool {
func (s *SM) Tick() bool {
	madeProgress := false
    // v3
    // madeProgress = s.reportFinishedKernels(now) || madeProgress
    // 	madeProgress = s.dispatchThreadblocksToSubcores(now) || madeProgress
    // 	madeProgress = s.processGPUInput(now) || madeProgress
    // 	madeProgress = s.processSubcoresInput(now) || madeProgress
	madeProgress = s.reportFinishedKernels() || madeProgress
	madeProgress = s.dispatchThreadblocksToSubcores() || madeProgress
	madeProgress = s.processGPUInput() || madeProgress
	madeProgress = s.processSubcoresInput() || madeProgress

	return madeProgress
}

// v3
// func (s *SM) processGPUInput(now sim.VTimeInSec) bool {
func (s *SM) processGPUInput() bool {
    // v3
    // msg := s.toGPU.Peek()
	msg := s.toGPU.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DeviceToSMMsg:
		s.processSMMsg(msg)
		// v3
		// s.processSMMsg(msg, now)
	default:
		log.WithField("function", "processGPUInput").Panic("Unhandled message type")
	}

	return true
}

// v3
// func (s *SM) processSubcoresInput(now sim.VTimeInSec) bool {
func (s *SM) processSubcoresInput() bool {
    // v3
    // msg := s.toSubcores.Peek()
	msg := s.toSubcores.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SubcoreToSMMsg:
		s.processSubcoreSubcoresg(msg)
		// v3
		// s.processSubcoreSubcoresg(msg, now)
	default:
		log.WithField("function", "processSubcoresInput").Panic("Unhandled message type")
	}

	return true
}

// v3
// func (s *SM) processSMMsg(msg *message.DeviceToSMMsg, now sim.VTimeInSec) {
func (s *SM) processSMMsg(msg *message.DeviceToSMMsg) {
	for i := range msg.Threadblock.Warps {
		s.undispatchedWarps = append(s.undispatchedWarps, &msg.Threadblock.Warps[i])
		s.unfinishedWarpsCount++
		s.warpsCount++
	}
    // v3
    // s.toGPU.Retrieve(now)
	s.toGPU.RetrieveIncoming()
}

// v3
// func (s *SM) processSubcoreSubcoresg(msg *message.SubcoreToSMMsg, now sim.VTimeInSec) {
func (s *SM) processSubcoreSubcoresg(msg *message.SubcoreToSMMsg) {
	if msg.WarpFinished {
		s.freeSubcores = append(s.freeSubcores, s.Subcores[msg.SubcoreID])
		s.unfinishedWarpsCount--
		if s.unfinishedWarpsCount == 0 {
			s.finishedThreadblocksCount++
		}
	}
    // v3
    // s.toSubcores.Retrieve(now)
	s.toSubcores.RetrieveIncoming()
}

// v3
// func (s *SM) reportFinishedKernels(now sim.VTimeInSec) bool {
func (s *SM) reportFinishedKernels() bool {
	if s.finishedThreadblocksCount == 0 {
		return false
	}

	msg := &message.SMToDeviceMsg{
		ThreadblockFinished: true,
		SMID:                s.ID,
	}
	// v3
    // msg.Src = s.toGPU
    // msg.Dst = s.toGPURemote
	msg.Src = s.toGPU.AsRemote()
	msg.Dst = s.toGPURemote.AsRemote()
	// v3
    // 	msg.SendTime = now

	err := s.toGPU.Send(msg)
	if err != nil {
		return false
	}

	s.finishedThreadblocksCount--

	return true
}

// v3
// func (s *SM) dispatchThreadblocksToSubcores(now sim.VTimeInSec) bool {
func (s *SM) dispatchThreadblocksToSubcores() bool {
	if len(s.freeSubcores) == 0 || len(s.undispatchedWarps) == 0 {
		return false
	}

	subcore := s.freeSubcores[0]
	warp := s.undispatchedWarps[0]

	msg := &message.SMToSubcoreMsg{
		Warp: *warp,
	}
	// v3
	// msg.Src = s.toSubcores
	// msg.Dst = subcore.GetPortByName("ToSM")
	msg.Src = s.toSubcores.AsRemote()
	msg.Dst = subcore.GetPortByName("ToSM").AsRemote()
	// v3
    // msg.SendTime = now

	err := s.toSubcores.Send(msg)
	if err != nil {
		return false
	}

	s.freeSubcores = s.freeSubcores[1:]
	s.undispatchedWarps = s.undispatchedWarps[1:]

	return false
}

func (s *SM) GetTotalWarpsCount() int64 {
	return s.warpsCount
}

func (s *SM) GetTotalInstsCount() int64 {
	return s.instsCount
}

func (s *SM) LogStatus() {
	log.WithFields(log.Fields{
		"sm_id":             s.ID,
		"total_warps_count": s.warpsCount,
	}).Info("SM status")
}
