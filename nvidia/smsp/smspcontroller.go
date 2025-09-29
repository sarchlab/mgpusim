package smsp

import (
	// "fmt"

	"encoding/binary"
	"math/rand/v2"

	log "github.com/sirupsen/logrus"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
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
	// ToGPUControllerMem       sim.Port
	// ToGPUControllerMemRemote sim.Port
	// ToMem            sim.Port
	ToVectorMemRemote sim.Port
	// waitingForMemRsp  bool
	waitingCycle uint64

	PendingSMSPtoMemReadReq  map[string]*mem.ReadReq
	PendingSMSPtoMemWriteReq map[string]*mem.WriteReq
	PendingSMSPMemMsgID2Warp map[string]*SMSPWarpUnit

	// unfinishedInstsCount uint64
	scheduler SMSPSWarpScheduler

	finishedWarpsCount uint64
	finishedWarpsList  []*trace.WarpTrace
	// currentWarp        trace.WarpTrace

	ToVectorMem sim.Port

	SMSPReceiveSMLatency          uint64
	SMSPReceiveSMLatencyRemaining uint64

	VisTracing bool
}

func (s *SMSPController) SetSMRemotePort(remote sim.Port) {
	s.toSMRemote = remote
}

func (s *SMSPController) SetVectorMemRemote(remote sim.Port) {
	s.ToVectorMemRemote = remote
	// fmt.Printf("SMSPController %s set GPUControllerMemRemote to %s\n", s.ID, remote.Name())
}

// func (s *SMSPController) SetSMMemRemotePort(remote sim.Port) {
// 	s.toSMMemRemote = remote
// }

func (s *SMSPController) Tick() bool {
	madeProgress := false
	madeProgress = s.reportFinishedWarps() || madeProgress
	// madeProgress = s.run() || madeProgress
	madeProgress = s.processSMInput() || madeProgress
	madeProgress = s.run() || madeProgress // avoid huge cost from warp setup
	madeProgress = s.processMemRsp() || madeProgress
	// warps can be switched, but ignore now

	return madeProgress
}

// func (s *SMSPController) processSMSPsRequestMem() bool {
// 	msg := s.ToSMSPsMem.PeekIncoming()
// 	if msg == nil {
// 		return false
// 	}

// 	switch msg := msg.(type) {
// 	case *message.SMSPToGPUControllerMemReadMsg:
// 		g.processSMSPToGPUControllerMemReadMsg(msg)
// 	case *message.SMSPToGPUControllerMemWriteMsg:
// 		g.processSMSPToGPUControllerMemWriteMsg(msg)
// 	default:
// 		log.WithField("function", "processSMSPsRequestMem").Panic("Unhandled message type")
// 	}

// 	return true
// }

// func (s *SMSPController) processMemRsp() bool {
// 	msg := s.ToMem.PeekIncoming()
// 	if msg == nil {
// 		return false
// 	}
// 	// fmt.Printf("%T\n", msg)
// 	switch msg := msg.(type) {
// 	case *mem.WriteDoneRsp:
// 		write := s.PendingSMSPtoMemWriteReq[msg.RespondTo]
// 		// fmt.Printf("%.10f, GPUController received msg from Caches, original ID = %s write complete\n",
// 		// 	g.CurrentTime(), write.OriginalSMSPtoGPUControllerID)

// 		s.processMemWriteRspMsg(msg)
// 		// delete(g.PendingCacheWriteReq, msg.RespondTo)
// 		s.ToMem.RetrieveIncoming()

// 		return true
// 	case *mem.DataReadyRsp:
// 		read := s.PendingSMSPtoMemReadReq[msg.RespondTo]
// 		// delete(g.PendingReadReq, msg.RespondTo)

// 		// fmt.Printf("%.10f, GPUController received msg from Caches, original ID = %s read complete, %v\n",
// 		// 	g.CurrentTime(), read.OriginalSMSPtoGPUControllerID, msg.Data)

// 		s.processMemReadRspMsg(msg)
// 		// delete(g.PendingCacheReadReq, msg.RespondTo)
// 		s.ToMem.RetrieveIncoming()

// 		// g.toDRAM.RetrieveIncoming()

// 		return true
// 	default:
// 		log.Panicf("cannot process message of type %s", reflect.TypeOf(msg))
// 	}

// 	return false
// }

func (s *SMSPController) processSMInput() bool {
	// fmt.Println("Called processSMInput")
	// if s.SMSPReceiveSMLatencyRemaining > 0 {
	// 	s.SMSPReceiveSMLatencyRemaining--
	// 	// fmt.Printf("s.SMSPReceiveSMLatencyRemaining: %d->%d\n", s.SMSPReceiveSMLatencyRemaining+1, s.SMSPReceiveSMLatencyRemaining)
	// 	return true
	// }
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
	// s.SMSPReceiveSMLatencyRemaining = s.SMSPReceiveSMLatency

	return true
}

func (s *SMSPController) processMemRsp() bool {
	// fmt.Printf("[waiting] SMSPController %s is waiting in processMemRsp\n", s.ID)
	msg := s.ToVectorMem.PeekIncoming()
	if msg == nil {
		// fmt.Printf("[waiting] nothing is received in processMemRsp\n")
		return false
	}

	switch msg := msg.(type) { // switch msg := msg.(type) {
	case *mem.DataReadyRsp:
		originalReqMsg := s.PendingSMSPtoMemReadReq[msg.RespondTo]
		// fmt.Printf("[received] DataReadyRsp (%s): %v\n", originalReqMsg.ID, originalReqMsg)
		if s.VisTracing {
			tracing.TraceReqFinalize(originalReqMsg, s)
		}
		// fmt.Printf("%.10f, %s, SMSPController %s received data ready response (read), msg ID = %s\n", s.Engine.CurrentTime(), s.Name(), s.ID, msg.Meta().ID)
		// s.waitingForMemRsp = false
		warpUnit := s.PendingSMSPMemMsgID2Warp[originalReqMsg.ID]
		if warpUnit == nil {
			log.Panic("In processing read req, warpUnit not found")
		}
		if warpUnit.status != WarpStatusWaiting {
			// fmt.Printf("warpUnit.status = %d\n", warpUnit.status)
			log.Panic("In processing read req, the corresponding warpUnit status is not waiting")
		}
		delete(s.PendingSMSPtoMemReadReq, originalReqMsg.ID)
		if warpUnit.unfinishedInstsCount == 1 {
			s.scheduler.removeFinishedWarps(warpUnit)
			s.finishedWarpsCount++
			s.finishedWarpsList = append(s.finishedWarpsList, warpUnit.warp)
			// fmt.Printf("SMSPController %s finished a warp %d, finishedWarpsCount = %d\n", s.ID, warpUnit.warp.ID, s.finishedWarpsCount)
		} else {
			warpUnit.status = WarpStatusRunning
			warpUnit.unfinishedInstsCount--
		}

	case *mem.WriteDoneRsp:
		originalReqMsg := s.PendingSMSPtoMemWriteReq[msg.RespondTo]
		// fmt.Printf("[received] WriteDoneRsp (%s): %v\n", originalReqMsg.ID, originalReqMsg)
		if s.VisTracing {
			tracing.TraceReqFinalize(originalReqMsg, s)
		}
		// fmt.Printf("%.10f, %s, SMSPController %s received write done response (write), msg ID = %s\n", s.Engine.CurrentTime(), s.Name(), s.ID, msg.Meta().ID)
		// s.waitingForMemRsp = false
		warpUnit := s.PendingSMSPMemMsgID2Warp[originalReqMsg.ID]
		if warpUnit == nil {
			log.Panic("In processing write req, warpUnit not found")
		}
		if warpUnit.status != WarpStatusWaiting {
			log.Panic("In processing write req, the corresponding warpUnit status is not waiting")
		}
		delete(s.PendingSMSPtoMemWriteReq, originalReqMsg.ID)
		if warpUnit.unfinishedInstsCount == 1 {
			s.scheduler.removeFinishedWarps(warpUnit)
			s.finishedWarpsCount++
			s.finishedWarpsList = append(s.finishedWarpsList, warpUnit.warp)
			// fmt.Printf("SMSPController %s finished a warp %d, finishedWarpsCount = %d\n", s.ID, warpUnit.warp.ID, s.finishedWarpsCount)
		} else {
			warpUnit.status = WarpStatusRunning
			warpUnit.unfinishedInstsCount--
		}
	default:
		log.WithField("function", "processSMInput").Panic("Unhandled message type")
		s.ToVectorMem.RetrieveIncoming()
		return false
	}
	s.ToVectorMem.RetrieveIncoming()
	return true
}

// func (s *SMSPController) processSMSPToGPUControllerMemReadMsg(msg *message.SMSPToGPUControllerMemReadMsg) bool {
// 	// fmt.Printf("%.10f, %s, GPUController receives SMSPToGPUControllerMemReadMsg, read from address = %d\n", g.Engine.CurrentTime(), g.Name(), msg.Address)
// 	readReq := mem.ReadReqBuilder{}.
// 		WithSrc(g.ToCaches.AsRemote()).
// 		WithDst(g.ToDRAM.AsRemote()).
// 		WithAddress(msg.Address).
// 		WithPID(1).
// 		Build()
// 	err := g.ToCaches.Send(readReq)
// 	if err != nil {
// 		fmt.Printf("GPUController failed to send read mem request: %v\n", err)
// 		g.ToSMSPsMem.RetrieveIncoming()
// 		return false
// 	}
// 	// fmt.Printf("%.10f, GPUController, read request sent to DRAM, address = %d, ID = %s\n",
// 	// 	g.Engine.CurrentTime(), msg.Address, readReq.ID)
// 	g.PendingSMSPtoGPUControllerMemReadReq[msg.ID] = msg
// 	g.PendingCacheReadReq[readReq.ID] = &message.GPUControllerToCachesMemReadMsg{
// 		OriginalSMSPtoGPUControllerID: msg.ID,
// 		Msg:                           *readReq,
// 	}
// 	// fmt.Printf("%.10f, GPUController, read request sent to DRAM, address = %d, ID = %s\n", g.CurrentTime(), msg.Address, readReq.ID)
// 	g.ToSMSPsMem.RetrieveIncoming()
// 	return true
// }

// func (s *SMSPController) processSMSPToGPUControllerMemWriteMsg(msg *message.SMSPToGPUControllerMemWriteMsg) bool {
// 	// fmt.Printf("%.10f, %s, GPUController receives SMSPToGPUControllerMemWriteMsg, write to address = %d, data = %d\n", g.Engine.CurrentTime(), g.Name(), msg.Address, msg.Data)
// 	writeReq := mem.WriteReqBuilder{}.
// 		WithSrc(g.ToCaches.AsRemote()).
// 		WithDst(g.ToDRAM.AsRemote()).
// 		WithAddress(msg.Address).
// 		WithPID(1).
// 		WithData(uint32ToBytes(msg.Data)).
// 		Build()

// 	err := g.ToCaches.Send(writeReq)
// 	if err != nil {
// 		fmt.Printf("GPUController failed to send write mem request: %v\n", err)
// 		g.ToSMSPsMem.RetrieveIncoming()
// 		return false
// 	}
// 	g.PendingSMSPtoGPUControllerMemWriteReq[msg.ID] = msg
// 	g.PendingCacheWriteReq[writeReq.ID] = &message.GPUControllerToCachesMemWriteMsg{
// 		OriginalSMSPtoGPUControllerID: msg.ID,
// 		Msg:                           *writeReq,
// 	}

// 	// fmt.Printf("%.10f, GPUController, write request sent to DRAM, address = %d, ID = %s\n", g.CurrentTime(), msg.Address, writeReq.ID)
// 	g.ToSMSPsMem.RetrieveIncoming()
// 	return true
// }

// func (s *SMSPController) processMemReadRspMsg(rspMsg *mem.DataReadyRsp) bool {
// 	// fmt.Printf("%.10f, %s, GPUController is sending read rsp back to SMSP\n", g.Engine.CurrentTime(), g.Name())
// 	originalSMSPToGPUControllerReq := g.PendingSMSPtoGPUControllerMemReadReq[originalID]
// 	msg := &message.CachesToSMSPMemReadRspMsg{
// 		OriginalSMSPtoGPUControllerID: originalID,
// 		Msg:                           *rspMsg,
// 	}
// 	msg.Src = g.ToSMSPsMem.AsRemote()
// 	msg.Dst = originalSMSPToGPUControllerReq.Src
// 	msg.ID = sim.GetIDGenerator().Generate()
// 	err := g.ToSMSPsMem.Send(msg)
// 	if err != nil {
// 		fmt.Printf("GPUController failed to send read rsp back to SMSP: %v\n", err)
// 		return false
// 	}
// 	return true
// }

// func (s *SMSPController) processMemWriteRspMsg(rspMsg *mem.WriteDoneRsp) bool {
// 	// fmt.Printf("%.10f, %s, GPUController is sending write rsp back to SMSP\n", g.Engine.CurrentTime(), g.Name())
// 	originalSMSPToGPUControllerReq := g.PendingSMSPtoGPUControllerMemWriteReq[originalID]
// 	msg := &message.CachesToSMSPMemWriteRspMsg{
// 		OriginalSMSPtoGPUControllerID: originalID,
// 		Msg:                           *rspMsg,
// 	}
// 	msg.Src = g.ToSMSPsMem.AsRemote()
// 	msg.Dst = originalSMSPToGPUControllerReq.Src
// 	msg.ID = sim.GetIDGenerator().Generate()
// 	err := g.ToSMSPsMem.Send(msg)
// 	if err != nil {
// 		fmt.Printf("GPUController failed to send write rsp back to SMSP: %v\n", err)
// 		return false
// 	}
// 	return true
// }

func (s *SMSPController) processSMMsg(msg *message.SMToSMSPMsg) {
	// s.unfinishedInstsCount = msg.Warp.InstructionsCount()
	// s.currentWarp = msg.Warp
	// s.instsCount += msg.Warp.InstructionsCount()

	s.scheduler.insertWarps(msg.WarpList)

	// log.WithFields(log.Fields{
	// 	"msg.Warp id":     msg.Warp.ID,
	// 	"unit instsCount": msg.Warp.InstructionsCount()}).Info("SMSPController received warp")
	s.toSM.RetrieveIncoming()
}

func (s *SMSPController) run() bool {
	// if s.unfinishedInstsCount == 0 || s.waitingForMemRsp {
	// 	return false
	// }
	if s.scheduler.isEmpty() {
		return false
	}
	if s.waitingCycle > 0 {
		s.waitingCycle--
		return true
	}
	//  || s.waitingForMemRsp
	// currentWarpUnitIndex, currentWarpUnit := s.scheduler.issueWarp()

	currentWarpUnitList := s.scheduler.issueWarps()

	runProgress := false
	for _, warpUnit := range currentWarpUnitList {
		runProgress = s.runWarp(warpUnit) || runProgress
	}
	return runProgress
}

func (s *SMSPController) runWarp(warpUnit *SMSPWarpUnit) bool {
	if warpUnit == nil {
		// fmt.Printf("No warp is issued in SMSPController %s\n", s.ID)
		return false
	}

	// fmt.Printf("SMSPController %s issued warp unit index = %d, warp id = %d\n", s.ID, currentWarpUnitIndex, currentWarpUnit.warp.ID)

	// s.unfinishedInstsCount--
	// if s.unfinishedInstsCount == 0 {
	// 	s.finishedWarpsCount++
	// }

	lastInstructionFlag := false

	if warpUnit.unfinishedInstsCount == 1 {
		lastInstructionFlag = true
	}

	// currentInstruction := s.currentWarp.Instructions[s.currentWarp.InstructionsCount()-s.unfinishedInstsCount-1]
	currentInstruction := warpUnit.warp.Instructions[warpUnit.warp.InstructionsCount()-warpUnit.unfinishedInstsCount]

	currentInstructionType := currentInstruction.OpCode.OpcodeType()
	reqParentID := currentInstruction.InstructionsFullID()
	// fmt.Printf("%v\n", currentInstructionType == trace.OpCodeMemRead)
	switch currentInstructionType {
	case trace.OpCodeMemRead:
		// address := rand.Uint64() % (1048576 / 4) * 4
		// fmt.Printf("In branch trace.OpCodeMemRead\n")
		warpUnit.status = WarpStatusWaiting
		address := currentInstruction.MemAddress
		// address = 0
		s.doRead(warpUnit, reqParentID, address, uint64(currentInstruction.MemAddressSuffix1))
	case trace.OpCodeMemWrite:
		// address := rand.Uint64() % (1048576 / 4) * 4
		// fmt.Printf("In branch trace.OpCodeMemWrite\n")
		warpUnit.status = WarpStatusWaiting
		address := currentInstruction.MemAddress
		// address = 0
		data := rand.Uint32()
		s.doWrite(warpUnit, reqParentID, address, &data)
	case trace.OpCodeExit:
		// fmt.Printf("In branch trace.OpCodeExit\n")
		if currentInstruction.Mask != 0 {
			s.scheduler.removeFinishedWarps(warpUnit)
			// s.unfinishedInstsCount = 0
			s.finishedWarpsCount++
			s.finishedWarpsList = append(s.finishedWarpsList, warpUnit.warp)
			// fmt.Printf("SMSPController %s finished a warp %d, finishedWarpsCount = %d\n", s.ID, currentWarpUnit.warp.ID, s.finishedWarpsCount)
		} else {
			// if lastInstructionFlag {
			// 	s.scheduler.removeFinishedWarps(warpUnit)
			// 	s.finishedWarpsCount++
			// 	s.finishedWarpsList = append(s.finishedWarpsList, warpUnit.warp)
			// } else {
			// 	warpUnit.status = WarpStatusRunning
			// 	warpUnit.unfinishedInstsCount--
			// }
			s.handleNormalInstruction(lastInstructionFlag, warpUnit, currentInstruction)
		}
	// case trace.OpCode4:
	// 	s.waitingCycle = 3
	// case trace.OpCode6:
	// 	s.waitingCycle = 5
	// case trace.OpCode8:
	// 	s.waitingCycle = 7
	// case trace.OpCode10:
	// 	s.waitingCycle = 9
	default:
		// fmt.Printf("In branch default\n")
		// if lastInstructionFlag {
		// 	s.scheduler.removeFinishedWarps(warpUnit)
		// 	s.finishedWarpsCount++
		// 	s.finishedWarpsList = append(s.finishedWarpsList, warpUnit.warp)
		// } else {
		// 	warpUnit.status = WarpStatusRunning
		// 	warpUnit.unfinishedInstsCount--
		// }
		s.handleNormalInstruction(lastInstructionFlag, warpUnit, currentInstruction)
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

func (s *SMSPController) handleNormalInstruction(lastInstructionFlag bool, warpUnit *SMSPWarpUnit, currentInstruction *trace.InstructionTrace) {
	if warpUnit.currentInstructionRemainingCycles > 0 {
		// fmt.Printf("warpUnit.currentInstructionRemainingCycles: %d->%d\n", warpUnit.currentInstructionRemainingCycles, warpUnit.currentInstructionRemainingCycles-1)
		warpUnit.currentInstructionRemainingCycles--
		if warpUnit.currentInstructionRemainingCycles == 0 {
			s.handleNormalInstructionEnd(lastInstructionFlag, warpUnit)
		}
		return
	}

	// First time loading this instruction
	warpUnit.currentInstructionRemainingCycles = currentInstruction.OpCode.GetInstructionCycles() - 1 + 50
	if warpUnit.currentInstructionRemainingCycles == 0 {
		s.handleNormalInstructionEnd(lastInstructionFlag, warpUnit)
	}
	return
}

func (s *SMSPController) handleNormalInstructionEnd(lastInstructionFlag bool, warpUnit *SMSPWarpUnit) {
	if lastInstructionFlag {
		s.scheduler.removeFinishedWarps(warpUnit)
		s.finishedWarpsCount++
		s.finishedWarpsList = append(s.finishedWarpsList, warpUnit.warp)
	} else {
		warpUnit.status = WarpStatusRunning
		warpUnit.unfinishedInstsCount--
	}
}

func (s *SMSPController) reportFinishedWarps() bool {
	if s.finishedWarpsCount == 0 {
		return false
	}

	msg := &message.SMSPToSMMsg{
		WarpFinished: true,
		SMSPID:       s.ID,
		Warp:         s.finishedWarpsList[0],
	}
	msg.Src = s.toSM.AsRemote()
	msg.Dst = s.toSMRemote.AsRemote()

	err := s.toSM.Send(msg)
	if err != nil {
		return false
	}

	s.finishedWarpsCount--
	s.finishedWarpsList = s.finishedWarpsList[1:]

	return true
}

func (s *SMSPController) doRead(warpUnit *SMSPWarpUnit, reqParentID string, addr uint64, byteSize uint64) bool {
	// fmt.Printf("[start] SMSPController %s doRead from address %x with byteSize %d\n", s.ID, addr, byteSize)
	msg := mem.ReadReqBuilder{}.
		WithSrc(s.ToVectorMem.AsRemote()).
		WithDst(s.ToVectorMemRemote.AsRemote()).
		WithAddress(addr).
		WithByteSize(byteSize).
		WithPID(1).
		Build()
	if s.ToVectorMem == nil {
		log.Panic("s.ToVectorMem is nil")
	}
	if s.ToVectorMemRemote == nil {
		log.Panic("s.ToVectorMemRemote is nil")
	}
	msg.Src = s.ToVectorMem.AsRemote()
	msg.Dst = s.ToVectorMemRemote.AsRemote()
	msg.ID = sim.GetIDGenerator().Generate()
	if s.VisTracing {
		tracing.TraceReqInitiate(msg, s, reqParentID)
	}
	s.PendingSMSPtoMemReadReq[msg.ID] = msg
	s.PendingSMSPMemMsgID2Warp[msg.ID] = warpUnit
	err := s.ToVectorMem.Send(msg)
	if err != nil {
		return false
	}
	// s.waitingForMemRsp = true
	// fmt.Printf("[finished] SMSPController %s doRead from address %x with byteSize %d\n", s.ID, addr, byteSize)

	return true
}

func (s *SMSPController) doWrite(warpUnit *SMSPWarpUnit, reqParentID string, addr uint64, d *uint32) bool {
	// fmt.Printf("[start] SMSPController %s doRead from address %x\n", s.ID, addr)
	msg := mem.WriteReqBuilder{}.
		WithSrc(s.ToVectorMem.AsRemote()).
		WithDst(s.ToVectorMemRemote.AsRemote()).
		WithAddress(addr).
		WithPID(1).
		WithData(uint32ToBytes(*d)).
		Build()
	msg.Src = s.ToVectorMem.AsRemote()
	msg.Dst = s.ToVectorMemRemote.AsRemote()
	msg.ID = sim.GetIDGenerator().Generate()
	if s.VisTracing {
		tracing.TraceReqInitiate(msg, s, reqParentID)
	}
	s.PendingSMSPtoMemWriteReq[msg.ID] = msg
	s.PendingSMSPMemMsgID2Warp[msg.ID] = warpUnit
	// fmt.Printf("%.10f, %s, SMSPController %s sent write req to Mem, Address = %d, msg ID = %s\n", s.Engine.CurrentTime(), s.Name(), s.ID, *addr, msg.ID)
	err := s.ToVectorMem.Send(msg)
	if err != nil {
		return false
	}
	// s.waitingForMemRsp = true
	// fmt.Printf("[finished] SMSPController %s doRead from address %x\n", s.ID, addr)
	return true
}

func (s *SMSPController) GetTotalInstsCount() uint64 {
	return s.instsCount
}

func (s *SMSPController) LogStatus() {
	// log.WithFields(log.Fields{
	// 	"smsp_id":           s.ID,
	// 	"total_insts_count": s.instsCount,
	// }).Info("SMSPController status")
}

func uint32ToBytes(data uint32) []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, data)

	return bytes
}
