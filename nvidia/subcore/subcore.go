package subcore

import (
	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
)

type Subcore struct {
	*sim.TickingComponent

	ID         string
	instsCount int64

	// meta
	toSM       sim.Port
	toSMRemote sim.Port

	unfinishedInstsCount int64

	finishedWarpsCount int64
}

func (s *Subcore) SetSMRemotePort(remote sim.Port) {
	s.toSMRemote = remote
}

func (s *Subcore) Tick() bool {
	madeProgress := false
	madeProgress = s.reportFinishedWarps() || madeProgress
	madeProgress = s.run() || madeProgress
	madeProgress = s.processSMInput() || madeProgress
	// warps can be switched, but ignore now

	return madeProgress
}

func (s *Subcore) processSMInput() bool {
	msg := s.toSM.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMToSubcoreMsg:
		s.processSMMsg(msg)
	default:
		log.WithField("function", "processSMInput").Panic("Unhandled message type")
	}

	return true
}

func (s *Subcore) processSMMsg(msg *message.SMToSubcoreMsg) {
	s.unfinishedInstsCount = msg.Warp.InstructionsCount
	s.instsCount += msg.Warp.InstructionsCount
	s.toSM.RetrieveIncoming()
}

func (s *Subcore) run() bool {
	if s.unfinishedInstsCount == 0 {
		return false
	}

	s.unfinishedInstsCount--
	if s.unfinishedInstsCount == 0 {
		s.finishedWarpsCount++
	}

	return true
}

func (s *Subcore) reportFinishedWarps() bool {
	if s.finishedWarpsCount == 0 {
		return false
	}

	msg := &message.SubcoreToSMMsg{
		WarpFinished: true,
		SubcoreID:    s.ID,
	}
	msg.Src = s.toSM.AsRemote()
	msg.Dst = s.toSMRemote.AsRemote()

	err := s.toSM.Send(msg)
	if err != nil {
		return false
	}

	s.finishedWarpsCount--

	return true
}

func (s *Subcore) GetTotalInstsCount() int64 {
	return s.instsCount
}

func (s *Subcore) LogStatus() {
	log.WithFields(log.Fields{
		"subcore_id":        s.ID,
		"total_insts_count": s.instsCount,
	}).Info("Subcore status")
}
