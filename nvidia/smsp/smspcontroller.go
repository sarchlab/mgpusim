package smsp

import (
	// "fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type SMSPController struct {
	*sim.TickingComponent

	ID         string
	instsCount uint64

	// meta
	toSM       sim.Port
	toSMRemote sim.Port

	// cache updates
	// toSMMem       sim.Port
	// toSMMemRemote sim.Port

	unfinishedInstsCount uint64

	finishedWarpsCount uint64
	currentWarp        trace.WarpTrace
}

func (s *SMSPController) SetSMRemotePort(remote sim.Port) {
	s.toSMRemote = remote
}

// func (s *SMSPController) SetSMMemRemotePort(remote sim.Port) {
// 	s.toSMMemRemote = remote
// }

func (s *SMSPController) Tick() bool {
	madeProgress := false
	madeProgress = s.reportFinishedWarps() || madeProgress
	madeProgress = s.run() || madeProgress
	madeProgress = s.processSMInput() || madeProgress
	// warps can be switched, but ignore now

	return madeProgress
}

func (s *SMSPController) processSMInput() bool {
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

func (s *SMSPController) processSMMsg(msg *message.SMToSMSPMsg) {
	// fmt.Println("Called processSMMsg")
	s.unfinishedInstsCount = msg.Warp.InstructionsCount()
	s.currentWarp = msg.Warp
	s.instsCount += msg.Warp.InstructionsCount()
	// log.WithFields(log.Fields{
	// 	"msg.Warp id":     msg.Warp.ID,
	// 	"unit instsCount": msg.Warp.InstructionsCount()}).Info("SMSPController received warp")
	s.toSM.RetrieveIncoming()
}

func (s *SMSPController) run() bool {
	if s.unfinishedInstsCount == 0 {
		return false
	}

	s.unfinishedInstsCount--
	if s.unfinishedInstsCount == 0 {
		s.finishedWarpsCount++
	}
	currentInstruction := s.currentWarp.Instructions[s.currentWarp.InstructionsCount()-s.unfinishedInstsCount-1]
	currentInstructionType := currentInstruction.OpCode.OpcodeType()
	switch currentInstructionType {
	case trace.OpCodeMemRead:
		// address := rand.Uint64() % (1048576 / 4) * 4
		// s.doRead(&address)
	case trace.OpCodeMemWrite:
		// address := rand.Uint64() % (1048576 / 4) * 4
		// data := rand.Uint32()
		// s.doWrite(&address, &data)
	}
	// if currentInstruction.OpCode.OpcodeType() == trace.OpCodeMemory {
	// 	// fmt.Printf("%.10f, %s, SMSPController, insts id = %d, %s, %v\n",
	// 	// 	s.Engine.CurrentTime(), s.Name(),
	// 	// 	s.currentWarp.InstructionsCount()-s.unfinishedInstsCount-1,
	// 	// 	currentInstruction.OpCode,
	// 	// 	currentInstruction)
	// 	// address := rand.Uint64() % (1048576 / 4) * 4
	// 	// s.doRead(&address)
	// }
	return true
}

func (s *SMSPController) reportFinishedWarps() bool {
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

// func (s *SMSPController) doRead(addr *uint64) bool {

// 	msg := &message.SMSPToSMMemReadMsg{
// 		Address: *addr,
// 	}
// 	msg.Src = s.toSMMem.AsRemote()
// 	msg.Dst = s.toSMMemRemote.AsRemote()

// 	err := s.toSMMem.Send(msg)
// 	if err != nil {
// 		return false
// 	}

// 	return true
// }

// func (s *SMSPController) doWrite(addr *uint64, d *uint32) bool {

// 	msg := &message.SMSPToSMMemWriteMsg{
// 		Address: *addr,
// 		Data:    *d,
// 	}
// 	msg.Src = s.toSMMem.AsRemote()
// 	msg.Dst = s.toSMMemRemote.AsRemote()

// 	err := s.toSMMem.Send(msg)
// 	if err != nil {
// 		return false
// 	}

// 	return true
// }

func (s *SMSPController) GetTotalInstsCount() uint64 {
	return s.instsCount
}

func (s *SMSPController) LogStatus() {
	// log.WithFields(log.Fields{
	// 	"smsp_id":           s.ID,
	// 	"total_insts_count": s.instsCount,
	// }).Info("SMSPController status")
}
