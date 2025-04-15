package smsp

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type SMSP struct {
	*sim.TickingComponent

	ID         string
	instsCount uint64

	// meta
	toSM       sim.Port
	toSMRemote sim.Port

	unfinishedInstsCount uint64

	finishedWarpsCount uint64
	currentWarp        trace.WarpTrace
}

func (s *SMSP) SetSMRemotePort(remote sim.Port) {
	s.toSMRemote = remote
}

func (s *SMSP) Tick() bool {
	madeProgress := false
	madeProgress = s.reportFinishedWarps() || madeProgress
	madeProgress = s.run() || madeProgress
	madeProgress = s.processSMInput() || madeProgress
	// warps can be switched, but ignore now

	return madeProgress
}

func (s *SMSP) processSMInput() bool {
	// fmt.Println("Called processSMInput")
	msg := s.toSM.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMToSMSPMsg:
		s.processSMMsg(msg)
	default:
		log.WithField("function", "processSMInput").Panic("Unhandled message type")
	}

	return true
}

func (s *SMSP) processSMMsg(msg *message.SMToSMSPMsg) {
	// fmt.Println("Called processSMMsg")
	s.unfinishedInstsCount = msg.Warp.InstructionsCount()
	s.currentWarp = msg.Warp
	s.instsCount += msg.Warp.InstructionsCount()
	// log.WithFields(log.Fields{
	// 	"msg.Warp id":     msg.Warp.ID,
	// 	"unit instsCount": msg.Warp.InstructionsCount()}).Info("SMSP received warp")
	s.toSM.RetrieveIncoming()
}

func (s *SMSP) run() bool {
	if s.unfinishedInstsCount == 0 {
		return false
	}

	s.unfinishedInstsCount--
	if s.unfinishedInstsCount == 0 {
		s.finishedWarpsCount++
	}
	currentInstruction := s.currentWarp.Instructions[s.currentWarp.InstructionsCount()-s.unfinishedInstsCount-1]
	if currentInstruction.OpCode.OpcodeType() == trace.OpCodeMemory {
		fmt.Printf("%.10f, %s, SMSP %s is dealing with the OpCodeMemory type operation, insts id = %d, %s, %v\n", // %v
			s.Engine.CurrentTime(), s.Name(), s.Name(),
			s.currentWarp.InstructionsCount()-s.unfinishedInstsCount-1,
			currentInstruction.OpCode,
			currentInstruction)
		s.sendMemMsg(s.toSM, *currentInstruction)
	}
	return true
}

func (s *SMSP) sendMemMsg(port sim.Port, inst trace.InstructionTrace) bool {
	msg := message.MemMsgBuilder{}.
		WithSrc(s.toSM.AsRemote()).
		WithDst(s.toSMRemote.AsRemote()).
		WithMemAddress(message.MemAddress{
			MemBaseAddr: inst.MemAddress,
			MemOffset:   uint64(inst.MemWidth),
		}).
		Build()
	err := port.Send(msg)
	fmt.Printf("%.10f, %s, SMSP %s sent mem msg: %s -> %s [%d %d]\n",
		s.Engine.CurrentTime(), s.Name(),
		s.ID, msg.Src, msg.Dst,
		msg.MemAddress.MemBaseAddr,
		msg.MemAddress.MemOffset)
	if err != nil {
		log.WithField("function", "sendMemMsg").Panic("Failed to send mem message")
		return false
	}
	return true
}

func (s *SMSP) reportFinishedWarps() bool {
	if s.finishedWarpsCount == 0 {
		return false
	}

	msg := &message.SMSPToSMMsg{
		WarpFinished: true,
		SMSPID:       s.ID,
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

func (s *SMSP) GetTotalInstsCount() uint64 {
	return s.instsCount
}

func (s *SMSP) LogStatus() {
	log.WithFields(log.Fields{
		"smsp_id":           s.ID,
		"total_insts_count": s.instsCount,
	}).Info("SMSP status")
}
