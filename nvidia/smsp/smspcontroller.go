package smsp

import (
	// "fmt"

	"encoding/binary"
	"fmt"
	"strings"

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

	PendingSMSPtoMemReadReq     map[string]*mem.ReadReq
	PendingSMSPtoMemWriteReq    map[string]*mem.WriteReq
	PendingSMSPMemMsgID2Warp    map[string]*SMSPWarpUnit
	PendingSMSPMemMsgID2DstRegs map[string][]string
	PendingSMSPMemMsgID2SrcRegs map[string][]string

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
		srcRegs := s.PendingSMSPMemMsgID2SrcRegs[originalReqMsg.ID]
		dstRegs := s.PendingSMSPMemMsgID2DstRegs[originalReqMsg.ID]
		if warpUnit == nil {
			log.Panic("In processing read req, warpUnit not found")
		}
		if warpUnit.status != WarpStatusWaiting {
			fmt.Printf("[*mem.DataReadyRsp] SMSP: %v, warpUnit.status != WarpStatusWaiting, warpUnit.status = %d\n", s.Name(), warpUnit.status)
			s.scheduler.logWarpUnitList(s.Name(), s.Engine.CurrentTime())
			// log.Panic("In processing read req, the corresponding warpUnit status is not waiting")
			// May be empty dst & src memorypipe insts
			s.ToVectorMem.RetrieveIncoming()
			return true
		}
		delete(s.PendingSMSPtoMemReadReq, originalReqMsg.ID)
		// if warpUnit.unfinishedInstsCount == 1 {
		// 	s.scheduler.removeFinishedWarps(warpUnit)
		// 	s.finishedWarpsCount++
		// 	s.finishedWarpsList = append(s.finishedWarpsList, warpUnit.warp)
		// 	// fmt.Printf("SMSPController %s finished a warp %d, finishedWarpsCount = %d\n", s.ID, warpUnit.warp.ID, s.finishedWarpsCount)
		// } else {
		// 	warpUnit.status = WarpStatusRunning
		// 	warpUnit.unfinishedInstsCount--
		// }

		warpUnit.Scoreboard.MarkCompleted(srcRegs, dstRegs)
		warpUnit.Pipeline.Stages[warpUnit.Pipeline.PC].Left = 0

		if !warpUnit.Scoreboard.HasAnyBusy() {
			warpUnit.status = WarpStatusRunning
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
		srcRegs := s.PendingSMSPMemMsgID2SrcRegs[originalReqMsg.ID]
		dstRegs := s.PendingSMSPMemMsgID2DstRegs[originalReqMsg.ID]
		if warpUnit == nil {
			log.Panic("In processing write req, warpUnit not found")
		}
		if warpUnit.status != WarpStatusWaiting {
			fmt.Printf("[*mem.WriteDoneRsp] SMSP: %v, warpUnit.status != WarpStatusWaiting, warpUnit.status = %d\n", s.Name(), warpUnit.status)
			s.scheduler.logWarpUnitList(s.Name(), s.Engine.CurrentTime())
			// log.Panic("In processing write req, the corresponding warpUnit status is not waiting")
			// May be empty dst & src memorypipe insts
			s.ToVectorMem.RetrieveIncoming()
			return true
		}
		delete(s.PendingSMSPtoMemWriteReq, originalReqMsg.ID)
		// if warpUnit.unfinishedInstsCount == 1 {
		// 	s.scheduler.removeFinishedWarps(warpUnit)
		// 	s.finishedWarpsCount++
		// 	s.finishedWarpsList = append(s.finishedWarpsList, warpUnit.warp)
		// 	// fmt.Printf("SMSPController %s finished a warp %d, finishedWarpsCount = %d\n", s.ID, warpUnit.warp.ID, s.finishedWarpsCount)
		// } else {
		// 	warpUnit.status = WarpStatusRunning
		// 	warpUnit.unfinishedInstsCount--
		// }
		warpUnit.Scoreboard.MarkCompleted(srcRegs, dstRegs)
		warpUnit.Pipeline.Stages[warpUnit.Pipeline.PC].Left = 0

		if !warpUnit.Scoreboard.HasAnyBusy() {
			warpUnit.status = WarpStatusRunning
		}

	default:
		log.WithField("function", "processSMInput").Panic("Unhandled message type")
		s.ToVectorMem.RetrieveIncoming()
		return false
	}
	s.ToVectorMem.RetrieveIncoming()
	return true
}

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
	// if strings.Contains(s.Name(), "GPU[0].SM[0].SMSP[0]") {
	// 	fmt.Printf("%.10f, %s's Scheduler is running\n", s.Engine.CurrentTime(), s.Name())
	// }
	if s.scheduler.isEmpty() {
		return false
	}

	madeProgress := true

	// s.scheduler.logWarpUnitList(s.Name(), s.Engine.CurrentTime())
	issued := s.scheduler.issueWarps(s.ResourcePool) //  s.Name()
	// 1) Scheduler issues up to SMSPSchedulerIssueSpeed warps (or as configured)
	if strings.Contains(s.Name(), "GPU[0].SM[0].SMSP[0]") {
		s.scheduler.logWarpUnitList(s.Name(), s.Engine.CurrentTime())
		// if len(issued) == 0 {
		// 	fmt.Printf("%.10f, %s's Scheduler has nothing to issue, warpUnitList length = %d\n", s.Engine.CurrentTime(), s.Name(), len(s.scheduler.warpUnitList))
		// } else {
		// 	fmt.Printf("%.10f, %s's Scheduler issued %d warps:\n", s.Engine.CurrentTime(), s.Name(), len(issued))
		// }
	}

	if len(issued) == 0 {
		madeProgress = false
	}
	// fmt.Printf("%.10f, SMSPController %s issued %d warps:\n", s.Engine.CurrentTime(), s.Name(), len(issued))

	// 2) For each newly issued warp, either start memory request (special) or
	//    create/attach a pipeline and add it to runningWarps.
	for _, wu := range issued {
		// stageNameString := fmt.Sprintf("%s(%s)", wu.Pipeline.Stages[wu.Pipeline.PC].Def.Name, wu.Pipeline.Stages[wu.Pipeline.PC].Def.Unit.String())
		// currentInst := wu.warp.Instructions[wu.warp.InstructionsCount()-wu.unfinishedInstsCount].OpCode.String()
		// fmt.Printf("one warpunit: warp id = %d, unfinishedInstsCount = %d, status = %d. This warp's current instruction: %s, its pipeline is at stage %d(name: %s), left cycles: %d\n", wu.warp.ID, wu.unfinishedInstsCount, wu.status, currentInst, wu.Pipeline.PC, stageNameString, wu.Pipeline.Stages[wu.Pipeline.PC].Left)
		// If warp is waiting for mem somehow, skip (shouldn't normally happen)
		if wu.status == WarpStatusWaiting {
			log.Panic("warp in waiting status should not be issued")
		}

		// Determine next instruction index
		if wu.unfinishedInstsCount == 0 {
			// nothing to do
			log.Panic("issued warp has no unfinished instructions")
		}

		progressed := false
		stageName := wu.Pipeline.Stages[wu.Pipeline.PC].Def.Name
		if isMemoryPipeStage(stageName) {
			// fmt.Printf("SMSPController %s processing memory pipe stage %s for warp %d\n", s.ID, stageName, wu.warp.ID)
			if wu.Pipeline.Stages[wu.Pipeline.PC].Left == 2 { // Left=2: not yet launched; Left=1: launched but not yet completed; Left=0: completed
				// send mem request
				// fmt.Printf("WarpStatusReady status for warp %d\n", wu.warp.ID)
				issuedMemoryPipeNum := uint64(0)
				reachMaxNumMemoryPipeInst := false
				lastAvailableMemoryPipeInstIdx := uint64(0)
				previousPipelinePC := wu.Pipeline.PC
				for !reachMaxNumMemoryPipeInst && wu.warp.InstructionsCount()-wu.unfinishedInstsCount+issuedMemoryPipeNum < wu.warp.InstructionsCount() {
					inst := wu.warp.Instructions[wu.warp.InstructionsCount()-wu.unfinishedInstsCount+issuedMemoryPipeNum]
					if !isMemoryPipeStage(getPipelineStages(inst.OpCode.String()).Stages[previousPipelinePC].Name) {
						break
					}

					srcRegs := inst.SrcRegs
					destRegs := inst.DestRegs

					srcStringRegs := make([]string, len(srcRegs))
					destStringRegs := make([]string, len(destRegs))
					for i, r := range destRegs {
						destStringRegs[i] = r.Name
					}
					for i, r := range srcRegs {
						srcStringRegs[i] = r.Name
					}

					switch stageName {
					case "MemoryPipeRead":
						conflict := wu.Scoreboard.HasReadConflict(srcStringRegs, destStringRegs)
						if conflict {
							reachMaxNumMemoryPipeInst = true // stall due to register conflict
						} else {
							wu.status = WarpStatusWaiting
							lastAvailableMemoryPipeInstIdx = wu.warp.InstructionsCount() - wu.unfinishedInstsCount + issuedMemoryPipeNum
							issuedMemoryPipeNum++
							wu.Scoreboard.MarkIssued(srcStringRegs, destStringRegs)
							s.doRead(wu, inst.InstructionsFullID(), srcStringRegs, destStringRegs, inst.MemAddress, uint64(inst.MemAddressSuffix1))
						}
					case "MemoryPipeWrite":
						conflict := wu.Scoreboard.HasWriteConflict(srcStringRegs, destStringRegs)
						if conflict {
							reachMaxNumMemoryPipeInst = true // stall due to register conflict
						} else {
							wu.status = WarpStatusWaiting
							lastAvailableMemoryPipeInstIdx = wu.warp.InstructionsCount() - wu.unfinishedInstsCount + issuedMemoryPipeNum
							issuedMemoryPipeNum++
							wu.Scoreboard.MarkIssued(srcStringRegs, destStringRegs)
							data := uint32(0)
							s.doWrite(wu, inst.InstructionsFullID(), srcStringRegs, destStringRegs, inst.MemAddress, &data)
						}
					default:
						log.Panicf("Unknown memory pipe stage name: %s", stageName)
					}
				}

				currentInst := wu.warp.Instructions[lastAvailableMemoryPipeInstIdx]
				wu.Pipeline = NewPipelineInstance(currentInst, wu)
				wu.Pipeline.PC = previousPipelinePC
				wu.Pipeline.Stages[wu.Pipeline.PC].Left = 1
				// wu.Pipeline.Stages[wu.Pipeline.PC].Left = 1
				if strings.Contains(s.Name(), "GPU[0].SM[0].SMSP[0]") {
					fmt.Printf("memLoop's issuedMemoryPipeNum = %d for warp %d\n", issuedMemoryPipeNum, wu.warp.ID)
				}

			} else if wu.Pipeline.Stages[wu.Pipeline.PC].Left == 1 {
				log.Panic("MemoryPipe stage should not be in 'Waiting' (Left = 1) status here")
			} else if wu.Pipeline.Stages[wu.Pipeline.PC].Left == 0 {
				// proceed
				// fmt.Printf("WarpStatusRunning status for warp %d\n", wu.warp.ID)
				// wu.Pipeline.Stages[wu.Pipeline.PC].Left = 0
				wu.Pipeline.PC++
				if wu.Pipeline.PC >= len(wu.Pipeline.Stages) {
					wu.Pipeline.Done = true
				}
			}
			progressed = true
		} else {
			progressed = wu.Pipeline.Tick()
		}

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

			// Skip masked branches/exits (BRA or EXIT) immediately if Mask == 0.
			// Repeat until we reach a non-skipped instruction or the warp finishes.
			for {
				if nextInst == nil {
					log.Panic("nextInst is nil unexpectedly")
					break
				}

				// If the mask is zero and the opcode is BRA or EXIT, treat it as executed
				// and move to the next instruction.
				// fmt.Printf("nextInst.Mask: %08b, %v\n", nextInst.Mask, nextInst.Mask == 0)
				if nextInst.Mask == 0 {
					opcodeStr := nextInst.OpCode.String()
					// fmt.Printf("opcodeStr: %s\n", opcodeStr)
					if opcodeStr == "BRA" || opcodeStr == "EXIT" {
						// consume this instruction
						wu.unfinishedInstsCount--
						// fmt.Printf("SMSPController %s: warp %d finished while skipping BRA/EXIT\n", s.ID, wu.warp.ID)
						if wu.unfinishedInstsCount == 0 {
							// Warp completely finished
							s.scheduler.removeFinishedWarps(wu)
							s.finishedWarpsCount++
							s.finishedWarpsList = append(s.finishedWarpsList, wu.warp)
							nextInst = nil
							break
						}
						// advance to next instruction
						nextIdx = wu.warp.InstructionsCount() - wu.unfinishedInstsCount
						nextInst = wu.warp.Instructions[nextIdx]
						// repeat the check for the new instruction
						continue
					}
				}
				// not skippable -> stop skipping loop
				break
			}

			// If warp finished while skipping, continue outer loop.
			if nextInst == nil {
				log.Panic("nextInst is nil unexpectedly")
				continue
			}

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
	}

	return madeProgress
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

func (s *SMSPController) doRead(warpUnit *SMSPWarpUnit, reqParentID string, srcRegs []string, dstRegs []string, addr uint64, byteSize uint64) bool {
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
	if s.PendingSMSPMemMsgID2DstRegs == nil {
		s.PendingSMSPMemMsgID2DstRegs = make(map[string][]string)
	}
	if s.PendingSMSPMemMsgID2SrcRegs == nil {
		s.PendingSMSPMemMsgID2SrcRegs = make(map[string][]string)
	}
	s.PendingSMSPtoMemReadReq[msg.ID] = msg
	s.PendingSMSPMemMsgID2Warp[msg.ID] = warpUnit
	s.PendingSMSPMemMsgID2DstRegs[msg.ID] = dstRegs
	s.PendingSMSPMemMsgID2SrcRegs[msg.ID] = srcRegs
	err := s.ToVectorMem.Send(msg)
	if err != nil {
		return false
	}
	// s.waitingForMemRsp = true
	// fmt.Printf("[finished] SMSPController %s doRead from address %x with byteSize %d\n", s.ID, addr, byteSize)

	return true
}

func (s *SMSPController) doWrite(warpUnit *SMSPWarpUnit, reqParentID string, srcRegs []string, dstRegs []string, addr uint64, d *uint32) bool {
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
	if s.PendingSMSPMemMsgID2DstRegs == nil {
		s.PendingSMSPMemMsgID2DstRegs = make(map[string][]string)
	}
	if s.PendingSMSPMemMsgID2SrcRegs == nil {
		s.PendingSMSPMemMsgID2SrcRegs = make(map[string][]string)
	}
	s.PendingSMSPtoMemWriteReq[msg.ID] = msg
	s.PendingSMSPMemMsgID2Warp[msg.ID] = warpUnit
	s.PendingSMSPMemMsgID2DstRegs[msg.ID] = dstRegs
	s.PendingSMSPMemMsgID2SrcRegs[msg.ID] = srcRegs
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
