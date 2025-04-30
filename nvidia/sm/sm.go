package sm

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/mem/cache/writearound"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
	"github.com/sarchlab/mgpusim/v4/nvidia/smsp"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type SM struct {
	*sim.TickingComponent

	ID         string
	warpsCount uint64
	// instsCount uint64

	// meta
	toGPU       sim.Port
	toGPURemote sim.Port

	toSMSPs   sim.Port
	SMSPs     map[string]*smsp.SMSP
	freeSMSPs []*smsp.SMSP

	undispatchedWarps    []*trace.WarpTrace
	unfinishedWarpsCount uint64

	finishedThreadblocksCount uint64
	l1Caches                  []*writearound.Comp
}

func (s *SM) SetGPURemotePort(remote sim.Port) {
	s.toGPURemote = remote
}

func (s *SM) Tick() bool {
	madeProgress := false
	madeProgress = s.reportFinishedKernels() || madeProgress
	madeProgress = s.dispatchThreadblocksToSMSPs() || madeProgress
	madeProgress = s.processGPUInput() || madeProgress
	madeProgress = s.processSMSPsInput() || madeProgress

	return madeProgress
}

func (s *SM) processGPUInput() bool {
	msg := s.toGPU.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.DeviceToSMMsg:
		s.processSMMsg(msg)
	default:
		log.WithField("function", "processGPUInput").Panic("Unhandled message type")
	}

	return true
}

func (s *SM) processSMSPsInput() bool {
	msg := s.toSMSPs.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMSPToSMMsg:
		s.processSMSPSMSPsg(msg)
	default:
		log.WithField("function", "processSMSPsInput").Panic("Unhandled message type")
	}

	return true
}

func (s *SM) processSMMsg(msg *message.DeviceToSMMsg) {
	for i := range msg.Threadblock.Warps {
		s.undispatchedWarps = append(s.undispatchedWarps, msg.Threadblock.Warps[i])
		s.unfinishedWarpsCount++
		s.warpsCount++
	}
	s.toGPU.RetrieveIncoming()
}

func (s *SM) processSMSPSMSPsg(msg *message.SMSPToSMMsg) {
	if msg.WarpFinished {
		s.freeSMSPs = append(s.freeSMSPs, s.SMSPs[msg.SMSPID])
		s.unfinishedWarpsCount--
		if s.unfinishedWarpsCount == 0 {
			s.finishedThreadblocksCount++
		}
	}
	s.toSMSPs.RetrieveIncoming()
}

func (s *SM) reportFinishedKernels() bool {
	if s.finishedThreadblocksCount == 0 {
		return false
	}

	msg := &message.SMToDeviceMsg{
		ThreadblockFinished: true,
		SMID:                s.ID,
	}
	msg.Src = s.toGPU.AsRemote()
	msg.Dst = s.toGPURemote.AsRemote()

	err := s.toGPU.Send(msg)
	if err != nil {
		return false
	}

	s.finishedThreadblocksCount--

	return true
}

func (s *SM) dispatchThreadblocksToSMSPs() bool {
	if len(s.freeSMSPs) == 0 || len(s.undispatchedWarps) == 0 {
		return false
	}

	smsp := s.freeSMSPs[0]
	warp := s.undispatchedWarps[0]

	msg := &message.SMToSMSPMsg{
		Warp: *warp,
	}
	msg.Src = s.toSMSPs.AsRemote()
	msg.Dst = smsp.GetPortByName(fmt.Sprintf("%s.ToSM", smsp.Name())).AsRemote()

	err := s.toSMSPs.Send(msg)
	if err != nil {
		return false
	}

	s.freeSMSPs = s.freeSMSPs[1:]
	s.undispatchedWarps = s.undispatchedWarps[1:]

	return false
}

func (s *SM) GetTotalWarpsCount() uint64 {
	return s.warpsCount
}

// func (s *SM) GetTotalInstsCount() uint64 {
// 	return s.instsCount
// }

func (s *SM) LogStatus() {
	// log.WithFields(log.Fields{
	// 	"sm_id":             s.ID,
	// 	"total_warps_count": s.warpsCount,
	// }).Info("SM status")
}
