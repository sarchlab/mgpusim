package smsp

import (
	// "fmt"

	"encoding/binary"

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
	// waitingCycle uint64

	PendingSMSPtoMemReadReq  map[string]*mem.ReadReq
	PendingSMSPtoMemWriteReq map[string]*mem.WriteReq
	PendingSMSPMemMsgID2Warp map[string]*SMSPWarpUnit

	// unfinishedInstsCount uint64
	scheduler *SMSPSWarpScheduler

	finishedWarpsCount uint64
	finishedWarpsList  []*trace.WarpTrace
	// currentWarp        trace.WarpTrace

	ToVectorMem sim.Port

	SMSPReceiveSMLatency          uint64
	SMSPReceiveSMLatencyRemaining uint64

	ResourcePool *ResourcePool

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

	s.ResourcePool = NewH100SMSPResourcePool()
	// fmt.Printf("SMSPController %s Tick\n", s.ID)
	madeProgress = s.reportFinishedWarps() || madeProgress
	// madeProgress = s.run() || madeProgress
	madeProgress = s.processSMInput() || madeProgress
	madeProgress = s.run() || madeProgress // avoid huge cost from warp setup
	madeProgress = s.processMemRsp() || madeProgress
	// warps can be switched, but ignore now
	// fmt.Printf("SMSPController %s Tick end, madeProgress: %v\n", s.ID, madeProgress)

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
	if s.scheduler.isEmpty() {
		return false
	}

	madeProgress := true

	// 1) Scheduler issues up to SMSPSchedulerIssueSpeed warps (or as configured)
	issued := s.scheduler.issueWarps(s.ResourcePool)
	if len(issued) == 0 {
		madeProgress = false
	}
	// fmt.Printf("SMSPController %s issued %d warps\n", s.Name(), len(issued))

	// 2) For each newly issued warp, either start memory request (special) or
	//    create/attach a pipeline and add it to runningWarps.
	for _, wu := range issued {
		// stageNameString := fmt.Sprintf("%s(%s)", wu.Pipeline.Stages[wu.Pipeline.PC].Def.Name, wu.Pipeline.Stages[wu.Pipeline.PC].Def.Unit.String())
		// fmt.Printf("one warpunit: warp id = %d, unfinishedInstsCount = %d, status = %d. This warp's pipeline is at stage %d(name: %s), left cycles: %d\n", wu.warp.ID, wu.unfinishedInstsCount, wu.status, wu.Pipeline.PC, stageNameString, wu.Pipeline.Stages[wu.Pipeline.PC].Left)
		// If warp is waiting for mem somehow, skip (shouldn't normally happen)
		if wu.status == WarpStatusWaiting {
			log.Panic("warp in waiting status should not be issued")
		}

		// Determine next instruction index
		if wu.unfinishedInstsCount == 0 {
			// nothing to do
			log.Panic("issued warp has no unfinished instructions")
		}

		progressed := wu.Pipeline.Tick()

		if !progressed {
			log.Panic("issued warp's pipeline should not be stalled")
		}

		if wu.Pipeline.Done {
			wu.Pipeline = nil
			wu.unfinishedInstsCount--

			if wu.unfinishedInstsCount == 0 {
				// Warp completely finished
				s.scheduler.removeFinishedWarps(wu)
				s.finishedWarpsCount++
				s.finishedWarpsList = append(s.finishedWarpsList, wu.warp)
				continue
			}

			// Prepare next instruction immediately
			nextIdx := wu.warp.InstructionsCount() - wu.unfinishedInstsCount
			nextInst := wu.warp.Instructions[nextIdx]

			// switch nextInst.OpCode.OpcodeType() {
			// case trace.OpCodeMemRead:
			// 	wu.status = WarpStatusWaiting
			// 	s.doRead(wu, nextInst.InstructionsFullID(), nextInst.MemAddress, uint64(nextInst.MemAddressSuffix1))
			// 	// remove from running list temporarily
			// 	s.runningWarps = append(s.runningWarps[:i], s.runningWarps[i+1:]...)
			// 	continue
			// case trace.OpCodeMemWrite:
			// 	wu.status = WarpStatusWaiting
			// 	data := uint32(0)
			// 	s.doWrite(wu, nextInst.InstructionsFullID(), nextInst.MemAddress, &data)
			// 	s.runningWarps = append(s.runningWarps[:i], s.runningWarps[i+1:]...)
			// 	continue
			// default:
			// 	// Non-memory instruction: start the next pipeline immediately
			// 	wu.Pipeline = NewPipelineInstance(nextInst, wu)
			// 	wu.status = WarpStatusRunning
			// }

			wu.Pipeline = NewPipelineInstance(nextInst, wu)
			wu.status = WarpStatusReady

			// stay in runningWarps for next tick
			continue
		}

		// instIdx := wu.warp.InstructionsCount() - wu.unfinishedInstsCount
		// inst := wu.warp.Instructions[instIdx]

		// Memory ops are special: we send mem request and mark waiting (do not create pipeline)
		// switch inst.OpCode.OpcodeType() {
		// case trace.OpCodeMemRead:
		// 	wu.status = WarpStatusWaiting
		// 	s.doRead(wu, inst.InstructionsFullID(), inst.MemAddress, uint64(inst.MemAddressSuffix1))
		// 	madeProgress = true
		// 	continue
		// case trace.OpCodeMemWrite:
		// 	wu.status = WarpStatusWaiting
		// 	data := uint32(0) // or rand.Uint32() if you want nondeterministic
		// 	s.doWrite(wu, inst.InstructionsFullID(), inst.MemAddress, &data)
		// 	madeProgress = true
		// 	continue
		// }
	}

	// // 3) Tick all running pipelines once (use index loop because we will remove completed)
	// i := 0
	// // numTicked := 0
	// // numTickedAll := len(s.runningWarps)
	// for i < len(s.runningWarps) {
	// 	wu := s.runningWarps[i]
	// 	// fmt.Printf("One warpunit in runningWarps: warp id = %d, unfinishedInstsCount = %d, status = %d. This warp's pipeline is at stage %d(name: %s), left cycles: %d\n", wu.warp.ID, wu.unfinishedInstsCount, wu.status, wu.Pipeline.PC, wu.Pipeline.Stages[wu.Pipeline.PC].Def.Name, wu.Pipeline.Stages[wu.Pipeline.PC].Left)
	// 	// Safety: skip waiting warps (they should not be in runningWarps normally)
	// 	if wu == nil || wu.Pipeline == nil {
	// 		// remove from running list if no pipeline (defensive)
	// 		s.runningWarps = append(s.runningWarps[:i], s.runningWarps[i+1:]...)
	// 		continue
	// 	}

	// 	// Tick the pipeline. Tick returns true if progressed; false if stalled due to resources.
	// 	progressed := wu.Pipeline.Tick(s.ResourcePool)
	// 	// if progressed {
	// 	// 	numTicked++
	// 	// }
	// 	if progressed {
	// 		madeProgress = true
	// 	}

	// 	if wu.Pipeline.Done {
	// 		wu.Pipeline = nil
	// 		wu.unfinishedInstsCount--

	// 		if wu.unfinishedInstsCount == 0 {
	// 			// Warp completely finished
	// 			s.scheduler.removeFinishedWarps(wu)
	// 			s.finishedWarpsCount++
	// 			s.finishedWarpsList = append(s.finishedWarpsList, wu.warp)
	// 			// Remove from running list
	// 			s.runningWarps = append(s.runningWarps[:i], s.runningWarps[i+1:]...)
	// 			continue
	// 		}

	// 		// Prepare next instruction immediately
	// 		nextIdx := wu.warp.InstructionsCount() - wu.unfinishedInstsCount
	// 		nextInst := wu.warp.Instructions[nextIdx]

	// 		switch nextInst.OpCode.OpcodeType() {
	// 		case trace.OpCodeMemRead:
	// 			wu.status = WarpStatusWaiting
	// 			s.doRead(wu, nextInst.InstructionsFullID(), nextInst.MemAddress, uint64(nextInst.MemAddressSuffix1))
	// 			// remove from running list temporarily
	// 			s.runningWarps = append(s.runningWarps[:i], s.runningWarps[i+1:]...)
	// 			continue
	// 		case trace.OpCodeMemWrite:
	// 			wu.status = WarpStatusWaiting
	// 			data := uint32(0)
	// 			s.doWrite(wu, nextInst.InstructionsFullID(), nextInst.MemAddress, &data)
	// 			s.runningWarps = append(s.runningWarps[:i], s.runningWarps[i+1:]...)
	// 			continue
	// 		default:
	// 			// Non-memory instruction: start the next pipeline immediately
	// 			wu.Pipeline = NewPipelineInstance(nextInst, wu)
	// 			wu.status = WarpStatusRunning
	// 		}

	// 		// stay in runningWarps for next tick
	// 		i++
	// 		continue
	// 	}

	// 	// not finished -> advance index
	// 	i++
	// }
	// fmt.Printf("SMSPController %s ticked %d/%d pipelines this cycle, runningWarps len = %d\n", s.ID, numTicked, numTickedAll, len(s.runningWarps))

	return madeProgress
}

// func (s *SMSPController) handleNormalInstruction(lastInstructionFlag bool, warpUnit *SMSPWarpUnit, currentInstruction *trace.InstructionTrace) {
// 	if warpUnit.currentInstructionRemainingCycles > 0 {
// 		// fmt.Printf("warpUnit.currentInstructionRemainingCycles: %d->%d\n", warpUnit.currentInstructionRemainingCycles, warpUnit.currentInstructionRemainingCycles-1)
// 		warpUnit.currentInstructionRemainingCycles--
// 		if warpUnit.currentInstructionRemainingCycles == 0 {
// 			s.handleNormalInstructionEnd(lastInstructionFlag, warpUnit)
// 		}
// 		return
// 	}

// 	// First time loading this instruction
// 	warpUnit.currentInstructionRemainingCycles = currentInstruction.OpCode.GetInstructionCycles() - 1 + 0
// 	if warpUnit.currentInstructionRemainingCycles == 0 {
// 		s.handleNormalInstructionEnd(lastInstructionFlag, warpUnit)
// 	}
// 	return
// }

// func (s *SMSPController) handleNormalInstructionEnd(lastInstructionFlag bool, warpUnit *SMSPWarpUnit) {
// 	if lastInstructionFlag {
// 		s.scheduler.removeFinishedWarps(warpUnit)
// 		s.finishedWarpsCount++
// 		s.finishedWarpsList = append(s.finishedWarpsList, warpUnit.warp)
// 	} else {
// 		warpUnit.status = WarpStatusRunning
// 		warpUnit.unfinishedInstsCount--
// 	}
// }

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
	const cacheBlockSize = 512
	blockOffset := addr % cacheBlockSize

	if blockOffset+byteSize > cacheBlockSize {
		byteSize = cacheBlockSize - blockOffset
		// Log warning for debugging
	}
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

// // add this helper to check existence before append
// func (s *SMSPController) appendToRunningIfNotPresent(wu *SMSPWarpUnit) {
// 	for _, w := range s.runningWarps {
// 		if w == wu {
// 			return
// 		}
// 	}
// 	s.runningWarps = append(s.runningWarps, wu)
// }
