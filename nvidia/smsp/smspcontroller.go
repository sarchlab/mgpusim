package smsp

import (
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

	toSM       sim.Port
	toSMRemote sim.Port

	ToVectorMemRemote sim.Port

	PendingSMSPtoMemReadReq      map[string]*mem.ReadReq
	PendingSMSPtoMemWriteReq     map[string]*mem.WriteReq
	PendingSMSPMemMsgID2Pipeline map[string]*PipelineInstance

	scheduler *SMSPSWarpScheduler

	finishedWarpsCount uint64
	finishedWarpsList  []*trace.WarpTrace

	ToVectorMem sim.Port

	SMSPReceiveSMLatency          uint64
	SMSPReceiveSMLatencyRemaining uint64

	ResourcePool           *ResourcePool
	MemResponseHandleWidth uint64

	VisTracing bool
}

func (s *SMSPController) SetSMRemotePort(remote sim.Port) {
	s.toSMRemote = remote
}

func (s *SMSPController) SetVectorMemRemote(remote sim.Port) {
	s.ToVectorMemRemote = remote
}

func (s *SMSPController) Tick() bool {
	madeProgress := false

	s.ResourcePool.Reset()
	madeProgress = s.reportFinishedWarps() || madeProgress
	madeProgress = s.processSMInput() || madeProgress
	madeProgress = s.run() || madeProgress
	madeProgress = s.processMemRsp() || madeProgress

	return madeProgress
}

func (s *SMSPController) processSMInput() bool {
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

func (s *SMSPController) releasePipelineSrcRegs(pipe *PipelineInstance) bool {
	if pipe == nil || pipe.SrcRegsReleased {
		return false
	}
	pipe.Warp.Scoreboard.MarkSrcRegsConsumed(pipe.SrcRegs)
	pipe.MarkSrcRegsReleased()
	return true
}

func (s *SMSPController) releasePipelineDstRegs(pipe *PipelineInstance) bool {
	if pipe == nil || pipe.DstRegsReleased {
		return false
	}
	pipe.Warp.Scoreboard.MarkDstRegsCompleted(pipe.DstRegs)
	pipe.MarkDstRegsReleased()
	return true
}

func (s *SMSPController) processMemRsp() bool {
	if s.MemResponseHandleWidth == 0 {
		s.MemResponseHandleWidth = 1
	}

	madeProgress := false
	for i := uint64(0); i < s.MemResponseHandleWidth; i++ {
		msg := s.ToVectorMem.PeekIncoming()
		if msg == nil {
			break
		}

		switch msg := msg.(type) {
		case *mem.DataReadyRsp:
			originalReqMsg := s.PendingSMSPtoMemReadReq[msg.RespondTo]
			if originalReqMsg == nil {
				log.Panic("read response has no matching pending request")
			}
			if s.VisTracing {
				tracing.TraceReqFinalize(originalReqMsg, s)
			}

			pipeline := s.PendingSMSPMemMsgID2Pipeline[originalReqMsg.ID]
			if pipeline == nil {
				log.Panic("pipeline not found for read response")
			}
			delete(s.PendingSMSPtoMemReadReq, originalReqMsg.ID)
			delete(s.PendingSMSPMemMsgID2Pipeline, originalReqMsg.ID)

			s.releasePipelineSrcRegs(pipeline)
			s.releasePipelineDstRegs(pipeline)
			pipeline.MarkMemoryResponseReady()
			pipeline.Warp.updateStatus()

		case *mem.WriteDoneRsp:
			originalReqMsg := s.PendingSMSPtoMemWriteReq[msg.RespondTo]
			if originalReqMsg == nil {
				log.Panic("write response has no matching pending request")
			}
			if s.VisTracing {
				tracing.TraceReqFinalize(originalReqMsg, s)
			}

			pipeline := s.PendingSMSPMemMsgID2Pipeline[originalReqMsg.ID]
			if pipeline == nil {
				log.Panic("pipeline not found for write response")
			}
			delete(s.PendingSMSPtoMemWriteReq, originalReqMsg.ID)
			delete(s.PendingSMSPMemMsgID2Pipeline, originalReqMsg.ID)

			s.releasePipelineSrcRegs(pipeline)
			s.releasePipelineDstRegs(pipeline)
			pipeline.MarkMemoryResponseReady()
			pipeline.Warp.updateStatus()

		default:
			log.WithField("function", "processSMInput").Panic("Unhandled message type")
			s.ToVectorMem.RetrieveIncoming()
			return madeProgress
		}
		s.ToVectorMem.RetrieveIncoming()
		madeProgress = true
	}

	return madeProgress
}

func (s *SMSPController) processSMMsg(msg *message.SMToSMSPMsg) {
	s.scheduler.insertWarps(msg.WarpList)
	s.toSM.RetrieveIncoming()
}

func (s *SMSPController) skipMaskedControlOps() bool {
	madeProgress := false

	for {
		removedAnyWarp := false
		for i, wu := range s.scheduler.warpUnitList {
			for wu.HasMoreToIssue() {
				inst := wu.NextInstruction()
				if inst.Mask != 0 {
					break
				}
				opcodeStr := inst.OpCode.String()
				if opcodeStr != "BRA" && opcodeStr != "EXIT" {
					break
				}
				wu.nextIssueInstIndex++
				wu.unfinishedInstsCount--
				madeProgress = true
			}

			if wu.unfinishedInstsCount == 0 && len(wu.InFlightPipelines) == 0 {
				s.finishedWarpsCount++
				s.finishedWarpsList = append(s.finishedWarpsList, wu.warp)
				s.scheduler.warpUnitList = append(s.scheduler.warpUnitList[:i], s.scheduler.warpUnitList[i+1:]...)
				removedAnyWarp = true
				break
			}

			wu.updateStatus()
		}

		if !removedAnyWarp {
			break
		}
	}

	return madeProgress
}

func (s *SMSPController) launchIssuedPipelines(issued []*IssueDecision) bool {
	madeProgress := false

	for _, decision := range issued {
		pipe := decision.Pipeline
		stage := pipe.CurrentStage()
		if stage == nil {
			continue
		}

		if isMemoryPipeStage(stage.Def.Name) {
			pipe.MarkMemoryRequestSent()
			// fmt.Printf("pipe.Inst.MemAddress = %x\n", pipe.Inst.MemAddress)
			switch stage.Def.Name {
			case "MemoryPipeRead":
				if !s.doRead(pipe, pipe.Inst.InstructionsFullID(), pipe.Inst.MemAddress, uint64(pipe.Inst.MemAddressSuffix1)) {
					log.Panic("failed to send memory read request")
				}
			case "MemoryPipeWrite":
				data := uint32(0)
				if !s.doWrite(pipe, pipe.Inst.InstructionsFullID(), pipe.Inst.MemAddress, &data) {
					log.Panic("failed to send memory write request")
				}
			default:
				log.Panicf("unknown memory pipe stage name: %s", stage.Def.Name)
			}
			s.releasePipelineSrcRegs(pipe)
		}

		decision.WarpUnit.updateStatus()
		madeProgress = true
	}

	return madeProgress
}

func (s *SMSPController) tickInFlightPipelines() bool {
	madeProgress := false

	for _, wu := range s.scheduler.warpUnitList {
		remaining := make([]*PipelineInstance, 0, len(wu.InFlightPipelines))
		for _, pipe := range wu.InFlightPipelines {
			if pipe.Done {
				if s.releasePipelineSrcRegs(pipe) {
					madeProgress = true
				}
				if s.releasePipelineDstRegs(pipe) {
					madeProgress = true
				}
				wu.unfinishedInstsCount--
				madeProgress = true
				continue
			}

			if pipe.IsMemoryPipeline() {
				remaining = append(remaining, pipe)
				continue
			}

			if pipe.Tick() {
				madeProgress = true
			}

			if pipe.ReadyToReleaseSrcRegs() {
				if s.releasePipelineSrcRegs(pipe) {
					madeProgress = true
				}
			}

			if pipe.Done {
				if s.releasePipelineDstRegs(pipe) {
					madeProgress = true
				}
				wu.unfinishedInstsCount--
				continue
			}

			remaining = append(remaining, pipe)
		}
		wu.InFlightPipelines = remaining
		wu.updateStatus()
	}

	return madeProgress
}

func (s *SMSPController) collectFinishedWarps() bool {
	madeProgress := false

	for {
		removedAnyWarp := false
		for i, wu := range s.scheduler.warpUnitList {
			if wu.unfinishedInstsCount == 0 && len(wu.InFlightPipelines) == 0 {
				s.finishedWarpsCount++
				s.finishedWarpsList = append(s.finishedWarpsList, wu.warp)
				s.scheduler.warpUnitList = append(s.scheduler.warpUnitList[:i], s.scheduler.warpUnitList[i+1:]...)
				madeProgress = true
				removedAnyWarp = true
				break
			}
		}

		if !removedAnyWarp {
			break
		}
	}

	return madeProgress
}

func (s *SMSPController) run() bool {
	if s.scheduler.isEmpty() {
		return false
	}

	madeProgress := false
	madeProgress = s.skipMaskedControlOps() || madeProgress

	issued := s.scheduler.issueWarps(s.ResourcePool)
	// if strings.Contains(s.Name(), "GPU[0].SM[0].SMSP[0]") {
	// 	s.scheduler.logWarpUnitList(s.Name(), s.Engine.CurrentTime())
	// }
	// s.scheduler.logWarpUnitList(s.Name(), s.Engine.CurrentTime())
	if len(issued) > 0 {
		madeProgress = true
	}

	madeProgress = s.launchIssuedPipelines(issued) || madeProgress
	madeProgress = s.tickInFlightPipelines() || madeProgress
	madeProgress = s.collectFinishedWarps() || madeProgress

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

func (s *SMSPController) doRead(pipeline *PipelineInstance, reqParentID string, addr uint64, byteSize uint64) bool {
	const cacheBlockSize = 512
	blockOffset := addr % cacheBlockSize

	if blockOffset+byteSize > cacheBlockSize {
		byteSize = cacheBlockSize - blockOffset
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
	s.PendingSMSPMemMsgID2Pipeline[msg.ID] = pipeline
	err := s.ToVectorMem.Send(msg)
	if err != nil {
		return false
	}

	return true
}

func (s *SMSPController) doWrite(pipeline *PipelineInstance, reqParentID string, addr uint64, d *uint32) bool {
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
	s.PendingSMSPMemMsgID2Pipeline[msg.ID] = pipeline
	err := s.ToVectorMem.Send(msg)
	if err != nil {
		return false
	}
	return true
}

func (s *SMSPController) GetTotalInstsCount() uint64 {
	return s.instsCount
}

func (s *SMSPController) LogStatus() {
}

func uint32ToBytes(data uint32) []byte {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, data)

	return bytes
}
