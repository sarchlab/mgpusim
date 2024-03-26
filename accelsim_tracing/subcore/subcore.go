package subcore

import (
	"log"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/message"
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

func (s *Subcore) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = s.reportFinishedWarps(now) || madeProgress
	madeProgress = s.run() || madeProgress
	madeProgress = s.processSMInput(now) || madeProgress
	// warps can be switched, but ignore now

	return madeProgress
}

func (s *Subcore) processSMInput(now sim.VTimeInSec) bool {
	msg := s.toSM.Peek()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMToSubcoreMsg:
		s.processSMMsg(msg, now)
	default:
		panic("Unrecognized message")
	}

	return true
}

func (s *Subcore) processSMMsg(msg *message.SMToSubcoreMsg, now sim.VTimeInSec) {
	s.unfinishedInstsCount = msg.Warp.InstructionsCount
	s.instsCount += msg.Warp.InstructionsCount

	s.toSM.Retrieve(now)
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

func (s *Subcore) reportFinishedWarps(now sim.VTimeInSec) bool {
	if s.finishedWarpsCount == 0 {
		return false
	}

	msg := &message.SubcoreToSMMsg{
		WarpFinished: true,
		SubcoreID:    s.ID,
	}
	msg.Src = s.toSM
	msg.Dst = s.toSMRemote
	msg.SendTime = now

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
	log.Printf("[subcore#%s] total_insts_count=%d\n", s.ID, s.instsCount)
}
