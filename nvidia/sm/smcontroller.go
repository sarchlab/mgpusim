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
	toGPUControllerCaches sim.Port

	// PendingReadReq  map[string]*message.SMSPToSMMemReadMsg
	// PendingWriteReq map[string]*message.SMSPToSMMemWriteMsg

	SMSPs    map[string]*smsp.SMSPController
	SMSPsIDs []string
	// freeSMSPs []*smsp.SMSPController
	SMSPList []*smsp.SMSPController

	undispatchedWarps []*trace.WarpTrace
	// unfinishedWarpsCount uint64

	// finishedThreadblocksCount uint64
	// l1Caches                  []*writearound.Comp

	smspsCount     uint64
	smspIssueIndex uint64

	SM2SMSPWarpIssueLatency          uint64
	SM2SMSPWarpIssueLatencyRemaining uint64

	threadblockWarpCountTable       map[trace.Dim3]uint64
	threadblockWarpCountTableOrigin map[trace.Dim3]uint64
}

func (s *SMController) SetGPURemotePort(remote sim.Port) {
	s.toGPURemote = remote
}

// func (s *SMController) SetGPUControllerCachesPort(remote sim.Port) {
// 	s.toGPUControllerCaches = remote
// 	// for i := range len(s.SMSPs) {
// 	// 	smsp := s.SMSPs[i]

// 	// 	sm.freeSMSPs = append(sm.freeSMSPs, smsp)
// 	// 	sm.SMSPs[smsp.ID] = smsp
// 	// }
// }

// func (s *SMController) SetGPUMemRemotePort(remote sim.Port) {
// 	s.toGPUMemRemote = remote
// }

func (s *SMController) Tick() bool {
	madeProgress := false
	madeProgress = s.reportFinishedKernels() || madeProgress
	madeProgress = s.dispatchThreadblocksToSMSPs() || madeProgress
	madeProgress = s.processGPUInput() || madeProgress
	madeProgress = s.processSMSPsInput() || madeProgress
	madeProgress = s.checkAnyWarpNotFinished() || madeProgress
	// madeProgress = s.processSMSPsInputMem() || madeProgress
	// madeProgress = s.processGPUMemRsp() || madeProgress

	return madeProgress
}

func (s *SMController) checkAnyWarpNotFinished() bool {
	if len(s.undispatchedWarps) > 0 {
		return true
	}
	for _, count := range s.threadblockWarpCountTable {
		if count > 0 {
			return true
		}
	}
	return false
	// return s.unfinishedWarpsCount > 0 || len(s.undispatchedWarps) > 0
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
	s.threadblockWarpCountTable[msg.Threadblock.ID] = msg.Threadblock.WarpsCount()
	s.threadblockWarpCountTableOrigin[msg.Threadblock.ID] = msg.Threadblock.WarpsCount()
	for i := range msg.Threadblock.Warps {
		s.undispatchedWarps = append(s.undispatchedWarps, msg.Threadblock.Warps[i])
		// s.unfinishedWarpsCount++
		s.warpsCount++
	}
	s.toGPU.RetrieveIncoming()
}

func (s *SMController) processSMSPSMsg(msg *message.SMSPToSMMsg) {
	warpFatherThreadID := msg.Warp.FatherThreadID
	if msg.WarpFinished {
		// s.freeSMSPs = append(s.freeSMSPs, s.SMSPs[msg.SMSPID])
		// s.unfinishedWarpsCount--
		s.threadblockWarpCountTable[warpFatherThreadID]--
		if s.threadblockWarpCountTable[warpFatherThreadID] < 0 {
			log.Panic("In processing SMSP message, threadblock warp count is negative")
		}
		// fmt.Printf("%.10f, %s, SMController, received a msg from smsp for a warp finished, unfinished warps count = %d->%d\n", s.Engine.CurrentTime(), s.Name(), s.unfinishedWarpsCount+1, s.unfinishedWarpsCount)
		// if s.unfinishedWarpsCount == 0 {
		// 	s.finishedThreadblocksCount++
		// }
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

func (s *SMController) findFirstFinishedThreadblock() trace.Dim3 {
	for threadblockID, warpsCount := range s.threadblockWarpCountTable {
		if warpsCount == 0 {
			delete(s.threadblockWarpCountTable, threadblockID)
			return threadblockID

			// fmt.Printf("find a finished threadblock %v\n", threadblockID)
		}
	}
	return trace.Dim3{-1, -1, -1}
}

func (s *SMController) reportFinishedKernels() bool {
	// if s.finishedThreadblocksCount == 0 {
	// 	return false
	// }
	firstFinishedThreadblock := s.findFirstFinishedThreadblock()
	if firstFinishedThreadblock == (trace.Dim3{-1, -1, -1}) {
		return false
	}
	numThreadFinished := s.threadblockWarpCountTableOrigin[firstFinishedThreadblock] * 32

	msg := &message.SMToDeviceMsg{
		NumThreadFinished: numThreadFinished,
		SMID:              s.ID,
	}
	msg.Src = s.toGPU.AsRemote()
	msg.Dst = s.toGPURemote.AsRemote()

	err := s.toGPU.Send(msg)
	if err != nil {
		return false
	}

	// s.finishedThreadblocksCount--

	return true
}

func (s *SMController) dispatchThreadblocksToSMSPs() bool {
	if len(s.SMSPList) == 0 || len(s.undispatchedWarps) == 0 {
		return false
	}

	if s.SM2SMSPWarpIssueLatencyRemaining > 0 {
		s.SM2SMSPWarpIssueLatencyRemaining--
		return true
	}

	if s.smspsCount == 0 {
		log.Panic("SMSP count is 0")
	}

	nUndispatchedWarps := len(s.undispatchedWarps)
	msgList := make([]message.SMToSMSPMsg, s.smspsCount)
	for i, smsp := range s.SMSPList {
		// fmt.Printf("i: %d, smsp id: %s, s.toSMSPs.AsRemote(): %s\n", i, smsp.ID, s.toSMSPs.AsRemote())
		msgList[i].Src = s.toSMSPs.AsRemote()
		msgList[i].Dst = smsp.GetPortByName(fmt.Sprintf("%s.ToSM", smsp.Name())).AsRemote()
		// msgList[i].Warp = nil
		msgList[i].WarpList = []*trace.WarpTrace{}
	}

	for i := 0; i < nUndispatchedWarps; i++ {
		// smsp := s.SMSPList[s.smspIssueIndex]
		warp := s.undispatchedWarps[0]

		msgList[s.smspIssueIndex].WarpList = append(msgList[s.smspIssueIndex].WarpList, warp)

		// msg := &message.SMToSMSPMsg{
		// 	Warp: warp,
		// }
		// msg.Src = s.toSMSPs.AsRemote()
		// msg.Dst = smsp.GetPortByName(fmt.Sprintf("%s.ToSM", smsp.Name())).AsRemote()

		// err := s.toSMSPs.Send(msg)
		// if err != nil {
		// 	return false
		// }
		s.smspIssueIndex = (s.smspIssueIndex + 1) % s.smspsCount
		s.undispatchedWarps = s.undispatchedWarps[1:]
	}

	for i := range msgList {
		if len(msgList[i].WarpList) > 0 {
			err := s.toSMSPs.Send(&msgList[i])
			if err != nil {
				return false
			}
		}
	}

	s.SM2SMSPWarpIssueLatencyRemaining = s.SM2SMSPWarpIssueLatency

	// smsp := s.SMSPList[s.smspIssueIndex]
	// warp := s.undispatchedWarps[0]

	// msg := &message.SMToSMSPMsg{
	// 	Warp: warp,
	// }
	// msg.Src = s.toSMSPs.AsRemote()
	// msg.Dst = smsp.GetPortByName(fmt.Sprintf("%s.ToSM", smsp.Name())).AsRemote()

	// err := s.toSMSPs.Send(msg)
	// if err != nil {
	// 	return false
	// }

	// s.smspIssueIndex = (s.smspIssueIndex + 1) % s.smspsCount
	// s.undispatchedWarps = s.undispatchedWarps[1:]

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
