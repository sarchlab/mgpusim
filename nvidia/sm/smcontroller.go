package sm

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/message"
	"github.com/sarchlab/mgpusim/v4/nvidia/smsp"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type SMController struct {
	*sim.TickingComponent

	ID         string
	warpsCount uint64
	// instsCount uint64

	// meta
	toGPU       sim.Port
	toGPURemote sim.Port

	toSMSPs sim.Port

	// cache updates
	// toGPUMem        sim.Port
	// toGPUMemRemote  sim.Port
	// toSMSPMem       sim.Port
	// toSMSPMemRemote sim.Port

	// PendingReadReq  map[string]*message.SMSPToSMMemReadMsg
	// PendingWriteReq map[string]*message.SMSPToSMMemWriteMsg

	SMSPs     map[string]*smsp.SMSPController
	freeSMSPs []*smsp.SMSPController

	undispatchedWarps    []*trace.WarpTrace
	unfinishedWarpsCount uint64

	finishedThreadblocksCount uint64
	// l1Caches                  []*writearound.Comp
}

func (s *SMController) SetGPURemotePort(remote sim.Port) {
	s.toGPURemote = remote
}

// func (s *SMController) SetGPUMemRemotePort(remote sim.Port) {
// 	s.toGPUMemRemote = remote
// }

func (s *SMController) Tick() bool {
	madeProgress := false
	madeProgress = s.reportFinishedKernels() || madeProgress
	madeProgress = s.dispatchThreadblocksToSMSPs() || madeProgress
	madeProgress = s.processGPUInput() || madeProgress
	madeProgress = s.processSMSPsInput() || madeProgress
	// madeProgress = s.processSMSPsInputMem() || madeProgress
	// madeProgress = s.processGPUMemRsp() || madeProgress

	return madeProgress
}

func (s *SMController) processGPUInput() bool {
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

func (s *SMController) processSMSPsInput() bool {
	msg := s.toSMSPs.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *message.SMSPToSMMsg:
		s.processSMSPSMsg(msg)
	default:
		log.WithField("function", "processSMSPsInput").Panic("Unhandled message type")
	}

	return true
}

// func (s *SMController) processSMSPsInputMem() bool {
// 	msg := s.toSMSPMem.PeekIncoming()
// 	if msg == nil {
// 		return false
// 	}

// 	switch msg := msg.(type) {
// 	case *message.SMSPToSMMemReadMsg:
// 		s.processSMSPSMemReadMsg(msg)
// 	case *message.SMSPToSMMemWriteMsg:
// 		s.processSMSPSMemWriteMsg(msg)
// 	default:
// 		log.WithField("function", "processSMSPsInputMem").Panic("Unhandled message type")
// 	}

// 	return true
// }

// func (s *SMController) processGPUMemRsp() bool {
// 	msg := s.toGPUMem.PeekIncoming()
// 	if msg == nil {
// 		return false
// 	}

// 	switch msg := msg.(type) {
// 	case *message.GPUtoSMMemReadMsg:
// 		s.processGPUMemReadMsg(msg)
// 	case *message.GPUtoSMMemWriteMsg:
// 		s.processGPUMemWriteMsg(msg)
// 	default:
// 		log.WithField("function", "processGPUMemRsp").Panic("Unhandled message type")
// 	}

// 	return true
// }

// func (s *SMController) processGPUMemReadMsg(rspMsg *message.GPUtoSMMemReadMsg) bool {
// 	fmt.Printf("%.10f, %s, GPU, read from address = %d, data = %d\n", s.Engine.CurrentTime(), s.Name(), rspMsg.Address, rspMsg.Rsp.Data)
// 	// msg := &message.GPUtoSMMemReadMsg{
// 	// 	Address: s.PendingReadReq[rspMsg.OriginalSMtoGPUID].Address,
// 	// 	Rsp:     rspMsg.Rsp,
// 	// }
// 	// msg.Src = g.toSMMem.AsRemote()
// 	// msg.Dst = g.PendingReadReq[rspMsg.RespondTo].Src
// 	// err := g.toSMMem.Send(msg)
// 	// if err != nil {
// 	// 	fmt.Printf("GPU failed to send read rsp to SMController: %v\n", err)
// 	// 	g.toDRAM.RetrieveIncoming()
// 	// 	return false
// 	// }
// 	s.toGPUMem.RetrieveIncoming()
// 	return true
// }

// func (s *SMController) processGPUMemWriteMsg(rspMsg *message.GPUtoSMMemWriteMsg) bool {
// 	fmt.Printf("%.10f, %s, GPU, write to address = %d\n", s.Engine.CurrentTime(), s.Name(), rspMsg.Address)
// 	// fmt.Printf("%.10f, %s, GPU\n", g.Engine.CurrentTime(), g.Name())
// 	// msg := &message.GPUtoSMMemWriteMsg{
// 	// 	Address: g.PendingReadReq[rspMsg.RespondTo].Address,
// 	// 	Rsp:     *rspMsg,
// 	// }
// 	// msg.Src = g.toSMMem.AsRemote()
// 	// msg.Dst = g.PendingReadReq[rspMsg.RespondTo].Src
// 	// err := g.toSMMem.Send(msg)
// 	// if err != nil {
// 	// 	fmt.Printf("GPU failed to send write rsp to SMController: %v\n", err)
// 	// 	g.toDRAM.RetrieveIncoming()
// 	// 	return false
// 	// }
// 	s.toGPUMem.RetrieveIncoming()
// 	return true
// }

func (s *SMController) processSMMsg(msg *message.DeviceToSMMsg) {
	for i := range msg.Threadblock.Warps {
		s.undispatchedWarps = append(s.undispatchedWarps, msg.Threadblock.Warps[i])
		s.unfinishedWarpsCount++
		s.warpsCount++
	}
	s.toGPU.RetrieveIncoming()
}

func (s *SMController) processSMSPSMsg(msg *message.SMSPToSMMsg) {
	if msg.WarpFinished {
		s.freeSMSPs = append(s.freeSMSPs, s.SMSPs[msg.SMSPID])
		s.unfinishedWarpsCount--
		if s.unfinishedWarpsCount == 0 {
			s.finishedThreadblocksCount++
		}
	}
	s.toSMSPs.RetrieveIncoming()
}

// func (s *SMController) processSMSPSMemReadMsg(msg *message.SMSPToSMMemReadMsg) bool {
// 	// fmt.Printf("%.10f, %s, SMSPController, read from address = %d\n", s.Engine.CurrentTime(), s.Name(), msg.Address)
// 	memMsg := &message.SMToGPUMemReadMsg{
// 		Address: msg.Address,
// 	}
// 	memMsg.Src = s.toGPUMem.AsRemote()
// 	memMsg.Dst = s.toGPUMemRemote.AsRemote()

// 	err := s.toGPUMem.Send(memMsg)
// 	if err != nil {
// 		return false
// 	}
// 	s.PendingReadReq[msg.ID] = msg
// 	s.toSMSPMem.RetrieveIncoming()
// 	return true
// }

// func (s *SMController) processSMSPSMemWriteMsg(msg *message.SMSPToSMMemWriteMsg) bool {
// 	// fmt.Printf("%.10f, %s, SMSPController, write to address = %d, data = %d\n", s.Engine.CurrentTime(), s.Name(), msg.Address, msg.Data)
// 	memMsg := &message.SMToGPUMemWriteMsg{
// 		Address: msg.Address,
// 		Data:    msg.Data,
// 	}
// 	memMsg.Src = s.toGPUMem.AsRemote()
// 	memMsg.Dst = s.toGPUMemRemote.AsRemote()

// 	err := s.toGPUMem.Send(memMsg)
// 	if err != nil {
// 		return false
// 	}
// 	s.PendingWriteReq[msg.ID] = msg
// 	s.toSMSPMem.RetrieveIncoming()
// 	return true
// }

func (s *SMController) reportFinishedKernels() bool {
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

func (s *SMController) dispatchThreadblocksToSMSPs() bool {
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

func (s *SMController) GetTotalWarpsCount() uint64 {
	return s.warpsCount
}

// func (s *SMController) GetTotalInstsCount() uint64 {
// 	return s.instsCount
// }

func (s *SMController) LogStatus() {
	// log.WithFields(log.Fields{
	// 	"sm_id":             s.ID,
	// 	"total_warps_count": s.warpsCount,
	// }).Info("SMController status")
}
